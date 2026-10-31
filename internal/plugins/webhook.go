package plugins

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/pocketbase/pocketbase/core"
)

const maxResponseBytes = 1 << 20

func contextWithTimeout(ctx context.Context, d time.Duration) (context.Context, func()) {
	return context.WithTimeout(ctx, d)
}

// ServeWebhook handles /webhooks/plugins/{plugin}/{path...}. Unknown and disabled
// plugins and unknown paths all get the same 404.
func (m *Manager) ServeWebhook(e *core.RequestEvent) error {
	p := m.Loaded(e.Request.PathValue("plugin"))
	if p == nil {
		return writeStatus(e, http.StatusNotFound, "not found")
	}
	path := strings.Trim(e.Request.PathValue("path"), "/")
	idx := slices.IndexFunc(p.Manifest.Webhooks, func(w WebhookManifest) bool { return w.Path == path })
	if idx < 0 {
		return writeStatus(e, http.StatusNotFound, "not found")
	}
	spec := p.Manifest.Webhooks[idx]
	if !slices.Contains(spec.Methods, e.Request.Method) {
		e.Response.Header().Set("Allow", strings.Join(spec.Methods, ", "))
		return writeStatus(e, http.StatusMethodNotAllowed, "method not allowed")
	}

	body, err := readLimited(e, spec.MaxBodyBytes)
	if err != nil {
		return writeStatus(e, http.StatusRequestEntityTooLarge, "payload too large")
	}

	ctx, cancel := context.WithTimeout(e.Request.Context(), acquireTimeout+p.timeout)
	defer cancel()

	var rejected bool
	res, err := p.call(ctx, func(v *vm) (goja.Value, error) {
		handler := v.webhooks[path]
		req := v.requestObject(e, body)
		if spec.Verify != verifyNone {
			verify, _ := goja.AssertFunction(handler.Get("verify"))
			ok, err := verify(handler, req)
			if err != nil {
				return nil, err
			}
			if ok == nil || !ok.StrictEquals(v.rt.ToValue(true)) {
				rejected = true
				return goja.Undefined(), nil
			}
		}
		handle, _ := goja.AssertFunction(handler.Get("handle"))
		return handle(handler, req)
	})
	if rejected {
		return writeStatus(e, http.StatusUnauthorized, "unauthorized")
	}
	return m.writeResult(e, p, res, err)
}

// ServeAdminRoute runs an admin route. The caller has already checked auth and
// the route's permission and existence.
func (m *Manager) ServeAdminRoute(e *core.RequestEvent, p *Plugin, path string) error {
	body, err := readLimited(e, maxWebhookBytes)
	if err != nil {
		return writeStatus(e, http.StatusRequestEntityTooLarge, "payload too large")
	}
	res, err := p.call(e.Request.Context(), func(v *vm) (goja.Value, error) {
		return v.admin[path](goja.Undefined(), v.requestObject(e, body))
	})
	return m.writeResult(e, p, res, err)
}

func readLimited(e *core.RequestEvent, limit int64) ([]byte, error) {
	if e.Request.Body == nil {
		return nil, nil
	}
	return io.ReadAll(http.MaxBytesReader(e.Response, e.Request.Body, limit))
}

// requestObject builds the `req` a handler receives. Signature checks run in Go
// over the raw bytes, because JS strings are UTF-16 and can't round-trip them.
func (v *vm) requestObject(e *core.RequestEvent, body []byte) *goja.Object {
	r := e.Request
	query := map[string]any{}
	for k, vals := range r.URL.Query() {
		if len(vals) > 0 {
			query[k] = vals[0]
		}
	}
	headers := map[string]any{}
	for k, vals := range r.Header {
		headers[strings.ToLower(k)] = strings.Join(vals, ", ")
	}

	req := v.rt.NewObject()
	_ = req.Set("method", r.Method)
	_ = req.Set("path", r.PathValue("path"))
	_ = req.Set("query", query)
	_ = req.Set("headers", headers)
	_ = req.Set("body", string(body))
	_ = req.Set("bodyBase64", base64.StdEncoding.EncodeToString(body))
	_ = req.Set("json", v.jsonParser(body))
	_ = req.Set("verifyHmacSha256", func(secret, signature string, opts map[string]any) bool {
		return verifySignature(body, secret, signature, opts)
	})
	_ = req.Set("once", func(key string, ttlSeconds int64) bool {
		first, err := v.plugin.mgr.store.kvOnce(v.plugin.Manifest.ID, "once:"+key, ttlSeconds)
		if err != nil {
			v.throw(err)
		}
		return first
	})
	return req
}

// verifySignature compares in constant time. A blank secret never verifies, so an
// unconfigured plugin can't be satisfied by a signature made with an empty key.
func verifySignature(body []byte, secret, signature string, opts map[string]any) bool {
	if secret == "" || signature == "" {
		return false
	}
	if prefix, _ := opts["prefix"].(string); prefix != "" {
		if !strings.HasPrefix(signature, prefix) {
			return false
		}
		signature = strings.TrimPrefix(signature, prefix)
	}

	var (
		got []byte
		err error
	)
	if encoding, _ := opts["encoding"].(string); encoding == "base64" {
		got, err = base64.StdEncoding.DecodeString(signature)
	} else {
		got, err = hex.DecodeString(signature)
	}
	if err != nil {
		return false
	}
	return hmac.Equal(got, hmacSHA256([]byte(secret), body))
}

func (m *Manager) writeResult(e *core.RequestEvent, p *Plugin, res any, err error) error {
	switch {
	case errors.Is(err, ErrBusy):
		e.Response.Header().Set("Retry-After", "5")
		return writeStatus(e, http.StatusServiceUnavailable, "plugin busy")
	case errors.Is(err, ErrTimeout):
		m.log.Printf("plugin %s: request to %s timed out", p.Manifest.ID, e.Request.URL.Path)
		return writeStatus(e, http.StatusGatewayTimeout, "plugin timed out")
	case err != nil:
		// Never log the request body: it can hold personal data.
		m.log.Printf("plugin %s: request to %s failed: %s", p.Manifest.ID, e.Request.URL.Path, p.redact(err.Error()))
		return writeStatus(e, http.StatusInternalServerError, "plugin error")
	}

	if res == nil {
		return e.JSON(http.StatusOK, map[string]any{"ok": true})
	}
	out, ok := res.(map[string]any)
	if !ok {
		m.log.Printf("plugin %s: handler for %s returned %T, not a response object", p.Manifest.ID, e.Request.URL.Path, res)
		return writeStatus(e, http.StatusInternalServerError, "plugin error")
	}

	status := http.StatusOK
	if n := intOpt(out, "status"); n >= 100 && n <= 599 {
		status = int(n)
	}

	var payload []byte
	contentType := "text/plain; charset=utf-8"
	if j, ok := out["json"]; ok {
		b, err := json.Marshal(j)
		if err != nil {
			return writeStatus(e, http.StatusInternalServerError, "plugin error")
		}
		payload = b
		contentType = "application/json"
	} else if b, ok := out["body"]; ok {
		payload = []byte(fmt.Sprint(b))
	}
	if len(payload) > maxResponseBytes {
		m.log.Printf("plugin %s: response for %s is over %d bytes", p.Manifest.ID, e.Request.URL.Path, maxResponseBytes)
		return writeStatus(e, http.StatusInternalServerError, "plugin error")
	}

	h := e.Response.Header()
	h.Set("Content-Type", contentType)
	if headers, ok := out["headers"].(map[string]any); ok {
		for k, val := range headers {
			if strings.EqualFold(k, "Set-Cookie") {
				continue
			}
			h.Set(k, fmt.Sprint(val))
		}
	}
	h.Set("X-Content-Type-Options", "nosniff")
	e.Response.WriteHeader(status)
	_, err = e.Response.Write(payload)
	return err
}

func writeStatus(e *core.RequestEvent, status int, message string) error {
	return e.JSON(status, map[string]any{"message": message})
}

// ServeAsset handles /admin/plugin-assets/{plugin}/{version}/{path...}. It serves
// only the ui/ folder of a running plugin, so server.js is never exposed. Module
// imports can't send a bearer token, so this is unauthenticated, and UI bundles
// must not contain secrets.
func (m *Manager) ServeAsset(e *core.RequestEvent) error {
	p := m.Loaded(e.Request.PathValue("plugin"))
	rel := e.Request.PathValue("path")
	if p == nil || p.Manifest.UI == nil || e.Request.PathValue("version") != p.Manifest.Version ||
		!strings.HasPrefix(rel, "ui/") || !isLocalPath(rel) {
		return writeStatus(e, http.StatusNotFound, "not found")
	}
	if _, err := fs.Stat(os.DirFS(p.Dir), rel); err != nil {
		return writeStatus(e, http.StatusNotFound, "not found")
	}
	h := e.Response.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	// The URL carries the version, so a file at a URL never changes.
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFileFS(e.Response, e.Request, os.DirFS(p.Dir), rel)
	return nil
}
