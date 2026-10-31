package plugins

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
	"github.com/dop251/goja"
)

const (
	maxFetchBytes = 10 << 20
	redacted      = "[redacted]"
)

var fetchClient = &http.Client{}

func (v *vm) throw(err error) {
	panic(v.rt.NewGoError(err))
}

// installGlobals defines the `plugin` and `host` objects the script sees.
func (v *vm) installGlobals() error {
	rt := v.rt

	plugin := rt.NewObject()
	_ = plugin.Set("channel", func(id string, handler *goja.Object) {
		if handler == nil {
			v.throw(errors.New("plugin.channel needs a handler object"))
		}
		if _, ok := goja.AssertFunction(handler.Get("send")); !ok {
			v.throw(fmt.Errorf("channel %q needs a send function", id))
		}
		if !v.declaresChannel(id) {
			v.throw(fmt.Errorf("channel %q isn't declared in %s", id, manifestFile))
		}
		v.channels[id] = handler
	})
	_ = plugin.Set("webhook", func(path string, handler *goja.Object) {
		if handler == nil {
			v.throw(errors.New("plugin.webhook needs a handler object"))
		}
		if !v.declaresWebhook(path) {
			v.throw(fmt.Errorf("webhook %q isn't declared in %s", path, manifestFile))
		}
		v.webhooks[path] = handler
	})
	_ = plugin.Set("adminRoute", func(path string, handler goja.Callable) {
		if handler == nil {
			v.throw(errors.New("plugin.adminRoute needs a function"))
		}
		if !v.declaresAdminRoute(path) {
			v.throw(fmt.Errorf("admin route %q isn't declared in %s", path, manifestFile))
		}
		v.admin[path] = handler
	})
	_ = plugin.Set("tool", func(name string, handler goja.Callable) {
		if handler == nil {
			v.throw(errors.New("plugin.tool needs a function"))
		}
		if !v.declaresTool(name) {
			v.throw(fmt.Errorf("tool %q isn't declared in %s", name, manifestFile))
		}
		v.tools[name] = handler
	})
	if err := rt.Set("plugin", plugin); err != nil {
		return err
	}

	host := rt.NewObject()
	sections := map[string]*goja.Object{
		"settings":  v.hostSettings(),
		"kv":        v.hostKV(),
		"log":       v.hostLog(),
		"crypto":    v.hostCrypto(),
		"errors":    v.hostErrors(),
		"contacts":  v.hostContacts(),
		"events":    v.hostEvents(),
		"messages":  v.hostMessages(),
		"workflows": v.hostWorkflows(),
	}
	for name, obj := range sections {
		_ = host.Set(name, obj)
	}
	_ = host.Set("fetch", v.hostFetch)
	return rt.Set("host", host)
}

func (v *vm) declaresChannel(id string) bool {
	for _, c := range v.plugin.Manifest.Channels {
		if c.ID == id {
			return true
		}
	}
	return false
}

func (v *vm) declaresWebhook(path string) bool {
	for _, w := range v.plugin.Manifest.Webhooks {
		if w.Path == path {
			return true
		}
	}
	return false
}

func (v *vm) declaresAdminRoute(path string) bool {
	for _, r := range v.plugin.Manifest.AdminRoutes {
		if r.Path == path {
			return true
		}
	}
	return false
}

func (v *vm) hostSettings() *goja.Object {
	obj := v.rt.NewObject()
	_ = obj.Set("get", func() map[string]any {
		values, err := v.plugin.settings()
		if err != nil {
			v.throw(err)
		}
		return values
	})
	return obj
}

func (v *vm) hostKV() *goja.Object {
	obj := v.rt.NewObject()
	id := v.plugin.Manifest.ID
	store := v.plugin.mgr.store

	_ = obj.Set("get", func(key string) goja.Value {
		raw, ok, err := store.kvGet(id, key)
		if err != nil {
			v.throw(err)
		}
		if !ok {
			return goja.Undefined()
		}
		var out any
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			v.throw(err)
		}
		return v.rt.ToValue(out)
	})
	_ = obj.Set("set", func(key string, value goja.Value, opts map[string]any) {
		raw, err := json.Marshal(value.Export())
		if err != nil {
			v.throw(err)
		}
		if err := store.kvSet(id, key, string(raw), intOpt(opts, "ttlSeconds")); err != nil {
			v.throw(err)
		}
	})
	_ = obj.Set("delete", func(key string) {
		if err := store.kvDelete(id, key); err != nil {
			v.throw(err)
		}
	})
	return obj
}

func (v *vm) hostLog() *goja.Object {
	obj := v.rt.NewObject()
	for _, level := range []string{"info", "warn", "error"} {
		_ = obj.Set(level, func(call goja.FunctionCall) goja.Value {
			parts := make([]string, 0, len(call.Arguments))
			for _, arg := range call.Arguments {
				parts = append(parts, arg.String())
			}
			v.plugin.mgr.log.Printf("plugin %s [%s]: %s", v.plugin.Manifest.ID, level, v.plugin.redact(strings.Join(parts, " ")))
			return goja.Undefined()
		})
	}
	return obj
}

func (v *vm) hostCrypto() *goja.Object {
	obj := v.rt.NewObject()
	_ = obj.Set("hmacSha256Hex", func(secret, data string) string {
		return hex.EncodeToString(hmacSHA256([]byte(secret), []byte(data)))
	})
	_ = obj.Set("sha256Hex", func(data string) string {
		sum := sha256.Sum256([]byte(data))
		return hex.EncodeToString(sum[:])
	})
	_ = obj.Set("timingSafeEqual", func(a, b string) bool {
		return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
	})
	return obj
}

func (v *vm) hostErrors() *goja.Object {
	obj := v.rt.NewObject()
	_ = obj.Set("unsendable", func(reason string) goja.Value {
		return v.rt.NewGoError(fmt.Errorf("%w: %s", models.ErrUnsendableDestination, reason))
	})
	return obj
}

// hostFetch is synchronous and bound to the current call's deadline.
func (v *vm) hostFetch(url string, opts map[string]any) goja.Value {
	method, _ := opts["method"].(string)
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if s, ok := opts["body"].(string); ok {
		body = strings.NewReader(s)
	}

	ctx := v.ctx
	if ms := intOpt(opts, "timeoutMs"); ms > 0 {
		var cancel func()
		ctx, cancel = contextWithTimeout(ctx, time.Duration(ms)*time.Millisecond)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), url, body)
	if err != nil {
		v.throw(err)
	}
	if headers, ok := opts["headers"].(map[string]any); ok {
		for k, val := range headers {
			req.Header.Set(k, fmt.Sprint(val))
		}
	}

	resp, err := fetchClient.Do(req)
	if err != nil {
		v.throw(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
	if err != nil {
		v.throw(err)
	}
	if len(data) > maxFetchBytes {
		v.throw(fmt.Errorf("response from %s is larger than %d bytes", req.URL.Host, maxFetchBytes))
	}

	headers := make(map[string]any, len(resp.Header))
	for k := range resp.Header {
		headers[strings.ToLower(k)] = resp.Header.Get(k)
	}

	out := v.rt.NewObject()
	_ = out.Set("status", resp.StatusCode)
	_ = out.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)
	_ = out.Set("headers", headers)
	_ = out.Set("body", string(data))
	_ = out.Set("bodyBase64", base64.StdEncoding.EncodeToString(data))
	_ = out.Set("json", v.jsonParser(data))
	return out
}

func (v *vm) jsonParser(data []byte) func() goja.Value {
	return func() goja.Value {
		var parsed any
		dec := json.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&parsed); err != nil {
			v.throw(fmt.Errorf("invalid JSON: %w", err))
		}
		return v.rt.ToValue(parsed)
	}
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func intOpt(opts map[string]any, key string) int64 {
	switch n := opts[key].(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

// settings returns the plugin's stored settings, secrets included, for server code.
func (p *Plugin) settings() (map[string]any, error) {
	values, err := p.mgr.host.LoadPluginSettings(p.Manifest.ID)
	if err != nil {
		return nil, err
	}
	for _, f := range p.Manifest.Settings {
		if _, ok := values[f.Key]; !ok && f.Default != nil {
			values[f.Key] = f.Default
		}
	}
	return values, nil
}

// redact removes configured secret values from text bound for the logs.
func (p *Plugin) redact(s string) string {
	values, err := p.mgr.host.LoadPluginSettings(p.Manifest.ID)
	if err != nil {
		return s
	}
	for _, f := range p.Manifest.Settings {
		if f.Type != "secret" {
			continue
		}
		if secret, ok := values[f.Key].(string); ok && secret != "" {
			s = strings.ReplaceAll(s, secret, redacted)
		}
	}
	return s
}
