package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/compdani/list_pocket/internal/migrations"
	"github.com/compdani/list_pocket/models"
	"github.com/dop251/goja"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

type fakeHost struct {
	mu       sync.Mutex
	settings map[string]map[string]any
	stripped []string
	invalid  []string
	events   []EventInput
}

func (h *fakeHost) LoadPluginSettings(id string) (map[string]any, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[string]any{}
	for k, v := range h.settings[id] {
		out[k] = v
	}
	return out, nil
}

func (h *fakeHost) SavePluginSettings(id string, values map[string]any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.settings == nil {
		h.settings = map[string]map[string]any{}
	}
	h.settings[id] = values
	return nil
}

func (h *fakeHost) DeletePluginSettings(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.settings, id)
	return nil
}

func (h *fakeHost) StripRolePermissions(perms []string) error {
	h.stripped = append(h.stripped, perms...)
	return nil
}

func (h *fakeHost) FindContact(string, string) (map[string]any, bool, error) { return nil, false, nil }
func (h *fakeHost) GetContact(string) (map[string]any, bool, error)          { return nil, false, nil }
func (h *fakeHost) UpsertContact(ContactInput) (map[string]any, error)       { return nil, nil }
func (h *fakeHost) SetConsent(string, string, string, string, string) error  { return nil }
func (h *fakeHost) ImportSMSConsent(string, bool, bool) (map[string]any, error) {
	return map[string]any{"explicit": 1}, nil
}
func (h *fakeHost) ListEvents(string, string, int) ([]map[string]any, error) { return nil, nil }
func (h *fakeHost) CountEvents(string) (map[string]int, error) {
	return map[string]int{"read": 1}, nil
}
func (h *fakeHost) MarkAddressInvalid(id, channel, address string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.invalid = append(h.invalid, address)
	return nil
}
func (h *fakeHost) RecordEvent(e EventInput) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, e)
	return true, nil
}
func (h *fakeHost) UpdateEventStatus(string, string, string, bool) (bool, error) { return false, nil }
func (h *fakeHost) RecordBounce(BounceInput) error                               { return nil }
func (h *fakeHost) OptOut(string, string) (int64, error)                         { return 0, nil }
func (h *fakeHost) SendMessage(SendInput) error                                  { return nil }

func testApp(t *testing.T) core.App {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	if err := app.RunAppMigrations(); err != nil {
		t.Fatal(err)
	}
	return app
}

func testManager(t *testing.T, app core.App, host HostServices, safeMode bool) *Manager {
	t.Helper()
	return New(app, Options{
		Dir:      filepath.Join(app.DataDir(), "plugins"),
		SafeMode: safeMode,
		Log:      log.New(io.Discard, "", 0),
		Host:     host,
	})
}

func zipBytes(t *testing.T, files map[string]string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buf.Bytes())
}

const testManifest = `{
  "schemaVersion": 1, "id": "sample", "name": "Sample", "version": "1.0.0",
  "server": {"concurrency": 2, "timeoutMs": 300},
  "webhooks": [
    {"path": "signed", "methods": ["GET", "POST"], "maxBodyBytes": 64},
    {"path": "open", "verify": "none"},
    {"path": "spin", "verify": "none"},
    {"path": "boom", "verify": "none"},
    {"path": "once", "verify": "none"}
  ],
  "adminRoutes": [{"path": "stats", "permission": "plugin_sample:manage"}],
  "settings": [
    {"key": "secret", "type": "secret", "label": "Secret", "required": true},
    {"key": "mode", "type": "select", "label": "Mode", "options": ["a", "b"], "default": "a"}
  ],
  "permissions": [{"group": "plugin_sample", "permissions": ["plugin_sample:manage"]}]
}`

const testServer = `
plugin.webhook('signed', {
  verify(req) {
    if (req.method === 'GET') return req.query.token === 'tok';
    return req.verifyHmacSha256(host.settings.get().secret, req.headers['x-sig'], { prefix: 'sha256=' });
  },
  handle(req) {
    if (req.method === 'GET') return { status: 200, body: req.query.challenge };
    return { status: 201, json: { got: req.json().value }, headers: { 'Set-Cookie': 'a=b', 'X-Test': 'yes' } };
  },
});
plugin.webhook('open', { handle(req) { host.kv.set('last', { body: req.body }); } });
plugin.webhook('spin', { handle() { while (true) {} } });
plugin.webhook('boom', { handle() { throw new Error('nope'); } });
plugin.webhook('once', { handle(req) { return { json: { first: req.once(req.query.id, 60) } }; } });
plugin.adminRoute('stats', () => ({ json: host.kv.get('last') || null }));
`

func installSample(t *testing.T, m *Manager) {
	t.Helper()
	r := zipBytes(t, map[string]string{"plugin.json": testManifest, "server.js": testServer})
	if _, err := m.Install(r, r.Size()); err != nil {
		t.Fatalf("install: %v", err)
	}
}

func bootedSample(t *testing.T) (*Manager, *fakeHost) {
	t.Helper()
	app := testApp(t)
	host := &fakeHost{}
	m := testManager(t, app, host, false)
	if err := m.Boot(); err != nil {
		t.Fatal(err)
	}
	installSample(t, m)
	if _, err := m.SetEnabled("sample", true); err != nil {
		t.Fatal(err)
	}
	if err := host.SavePluginSettings("sample", map[string]any{"secret": "s3cret"}); err != nil {
		t.Fatal(err)
	}
	// A new Manager on the same data stands in for the restart.
	m = testManager(t, app, host, false)
	if err := m.Boot(); err != nil {
		t.Fatal(err)
	}
	if m.Loaded("sample") == nil {
		rows, _ := m.List()
		t.Fatalf("sample did not load: %+v", rows)
	}
	return m, host
}

func webhookRequest(t *testing.T, m *Manager, method, plugin, path, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.SetPathValue("plugin", plugin)
	req.SetPathValue("path", path)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e := &core.RequestEvent{}
	e.Request = req
	e.Response = rec
	if err := m.ServeWebhook(e); err != nil {
		t.Fatalf("ServeWebhook: %v", err)
	}
	return rec
}

func sign(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestParseManifestRejects(t *testing.T) {
	base := func(extra string) string {
		return `{"schemaVersion":1,"id":"sample","name":"S","version":"1.0.0"` + extra + `}`
	}
	cases := map[string]string{
		"bad schema":          `{"schemaVersion":2,"id":"sample","name":"S","version":"1"}`,
		"reserved id":         `{"schemaVersion":1,"id":"whatsapp","name":"S","version":"1"}`,
		"bad campaign body":   base(`,"channels":[{"id":"ch:sample","label":"T","campaignBody":"sometimes"}]`),
		"bad id":              `{"schemaVersion":1,"id":"../evil","name":"S","version":"1"}`,
		"traversal version":   `{"schemaVersion":1,"id":"sample","name":"S","version":"1..2"}`,
		"slash version":       `{"schemaVersion":1,"id":"sample","name":"S","version":"1/2"}`,
		"unprefixed channel":  base(`,"channels":[{"id":"telegram","label":"T"}]`),
		"other plugin prefix": base(`,"channels":[{"id":"ch:other","label":"T"}]`),
		"html channel":        base(`,"channels":[{"id":"ch:sample","label":"T","contentTypes":["html"]}]`),
		"bad address source":  base(`,"channels":[{"id":"ch:sample","label":"T","addressSource":"email"}]`),
		"webhook traversal":   base(`,"webhooks":[{"path":"../x"}]`),
		"huge webhook":        base(`,"webhooks":[{"path":"x","maxBodyBytes":99999999}]`),
		"undeclared perm":     base(`,"adminRoutes":[{"path":"x","permission":"plugin_sample:nope"}]`),
		"foreign perm":        base(`,"permissions":[{"group":"campaigns","permissions":["campaigns:get"]}]`),
		"ui outside ui dir":   base(`,"ui":{"entry":"server.js"}`),
		"ui traversal":        base(`,"ui":{"entry":"ui/../../x.js"}`),
		"select no options":   base(`,"settings":[{"key":"a","type":"select","label":"A"}]`),
	}
	for name, raw := range cases {
		if _, err := ParseManifest([]byte(raw)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestInstallRejectsUnsafeArchives(t *testing.T) {
	m := testManager(t, testApp(t), &fakeHost{}, false)
	if err := m.Boot(); err != nil {
		t.Fatal(err)
	}

	cases := map[string]map[string]string{
		"zip slip":      {"plugin.json": testManifest, "server.js": testServer, "../evil.txt": "x"},
		"absolute path": {"plugin.json": testManifest, "server.js": testServer, "/evil.txt": "x"},
		"backslash":     {"plugin.json": testManifest, "server.js": testServer, `..\evil.txt`: "x"},
		"no manifest":   {"server.js": testServer},
		"no server":     {"plugin.json": testManifest},
		"syntax error":  {"plugin.json": testManifest, "server.js": "function ("},
	}
	for name, files := range cases {
		r := zipBytes(t, files)
		if _, err := m.Install(r, r.Size()); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(m.dir), "evil.txt")); err == nil {
		t.Fatal("zip slip wrote outside the plugin directory")
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	link := &zip.FileHeader{Name: "link"}
	link.SetMode(os.ModeSymlink | 0o777)
	w, _ := zw.CreateHeader(link)
	_, _ = w.Write([]byte("/etc/passwd"))
	mf, _ := zw.Create("plugin.json")
	_, _ = mf.Write([]byte(testManifest))
	_ = zw.Close()
	if _, err := m.Install(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err == nil {
		t.Error("symlink: expected an error")
	}
}

func TestInstallIsDisabledUntilEnabled(t *testing.T) {
	app := testApp(t)
	m := testManager(t, app, &fakeHost{}, false)
	if err := m.Boot(); err != nil {
		t.Fatal(err)
	}
	installSample(t, m)
	if m.Loaded("sample") != nil {
		t.Fatal("install must not start the plugin")
	}
	m2 := testManager(t, app, &fakeHost{}, false)
	if err := m2.Boot(); err != nil {
		t.Fatal(err)
	}
	if m2.Loaded("sample") != nil {
		t.Fatal("a disabled plugin must not load at boot")
	}
}

func TestDroppedDirectoryStaysDisabled(t *testing.T) {
	app := testApp(t)
	m := testManager(t, app, &fakeHost{}, false)
	dir := m.versionDir("sample", "1.0.0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(testManifest), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "server.js"), []byte(testServer), 0o644)
	if err := m.Boot(); err != nil {
		t.Fatal(err)
	}
	rows, _ := m.List()
	if len(rows) != 1 || rows[0].Enabled || rows[0].Status != StatusDisabled {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	if m.Loaded("sample") != nil {
		t.Fatal("dropped directory must not auto-enable")
	}
}

func TestFailedLoadDoesNotStopBoot(t *testing.T) {
	app := testApp(t)
	host := &fakeHost{}
	m := testManager(t, app, host, false)
	_ = m.Boot()
	r := zipBytes(t, map[string]string{"plugin.json": testManifest, "server.js": "throw new Error('bad top level');"})
	if _, err := m.Install(r, r.Size()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetEnabled("sample", true); err != nil {
		t.Fatal(err)
	}
	m2 := testManager(t, app, host, false)
	if err := m2.Boot(); err != nil {
		t.Fatalf("boot must survive a bad plugin: %v", err)
	}
	rows, _ := m2.List()
	if rows[0].Status != StatusFailed || !strings.Contains(rows[0].LastError, "bad top level") {
		t.Fatalf("unexpected status: %+v", rows[0])
	}
}

func TestMissingHandlerFailsLoad(t *testing.T) {
	app := testApp(t)
	host := &fakeHost{}
	m := testManager(t, app, host, false)
	_ = m.Boot()
	r := zipBytes(t, map[string]string{"plugin.json": testManifest, "server.js": "plugin.webhook('open', { handle() {} });"})
	if _, err := m.Install(r, r.Size()); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SetEnabled("sample", true)
	m2 := testManager(t, app, host, false)
	_ = m2.Boot()
	if m2.Loaded("sample") != nil {
		t.Fatal("a declared webhook with no handler must fail the load")
	}
}

func TestSafeModeLoadsNothing(t *testing.T) {
	m, host := bootedSample(t)
	m2 := testManager(t, m.store.app, host, true)
	if err := m2.Boot(); err != nil {
		t.Fatal(err)
	}
	if m2.Loaded("sample") != nil {
		t.Fatal("safe mode must not load plugins")
	}
	rows, _ := m2.List()
	if rows[0].Status != StatusSafeMode || !rows[0].Enabled {
		t.Fatalf("safe mode must keep the enabled flag: %+v", rows[0])
	}
}

func TestWebhookStatusMatrix(t *testing.T) {
	m, _ := bootedSample(t)
	body := `{"value":"héllo"}`

	cases := []struct {
		name         string
		method       string
		plugin, path string
		target       string
		body         string
		headers      map[string]string
		want         int
	}{
		{"unknown plugin", "POST", "nope", "signed", "/x", "", nil, 404},
		{"unknown path", "POST", "sample", "nope", "/x", "", nil, 404},
		{"wrong method", "DELETE", "sample", "signed", "/x", "", nil, 405},
		{"too large", "POST", "sample", "signed", "/x", strings.Repeat("a", 65), nil, 413},
		{"missing signature", "POST", "sample", "signed", "/x", body, nil, 401},
		{"bad signature", "POST", "sample", "signed", "/x", body, map[string]string{"X-Sig": sign("wrong", body)}, 401},
		{"no prefix", "POST", "sample", "signed", "/x", body, map[string]string{"X-Sig": strings.TrimPrefix(sign("s3cret", body), "sha256=")}, 401},
		{"good signature", "POST", "sample", "signed", "/x", body, map[string]string{"X-Sig": sign("s3cret", body)}, 201},
		{"handshake rejected", "GET", "sample", "signed", "/x?token=no&challenge=abc", "", nil, 401},
		{"handshake ok", "GET", "sample", "signed", "/x?token=tok&challenge=abc", "", nil, 200},
		{"no return value", "POST", "sample", "open", "/x", "hi", nil, 200},
		{"thrown error", "POST", "sample", "boom", "/x", "", nil, 500},
		{"timeout", "POST", "sample", "spin", "/x", "", nil, 504},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := webhookRequest(t, m, tc.method, tc.plugin, tc.path, tc.target, tc.body, tc.headers)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == 500 && strings.Contains(rec.Body.String(), "nope") {
				t.Fatalf("error details leaked: %s", rec.Body.String())
			}
		})
	}

	rec := webhookRequest(t, m, "GET", "sample", "signed", "/x?token=tok&challenge=abc", "", nil)
	if rec.Body.String() != "abc" {
		t.Fatalf("handshake body: %q", rec.Body.String())
	}
	rec = webhookRequest(t, m, "POST", "sample", "signed", "/x", body, map[string]string{"X-Sig": sign("s3cret", body)})
	if rec.Header().Get("Set-Cookie") != "" || rec.Header().Get("X-Test") != "yes" {
		t.Fatalf("unexpected headers: %v", rec.Header())
	}
	var parsed map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil || parsed["got"] != "héllo" {
		t.Fatalf("body: %s (%v)", rec.Body.String(), err)
	}
}

func TestSignatureUsesRawBytes(t *testing.T) {
	m, _ := bootedSample(t)
	raw := string([]byte{'{', 0xff, 0xfe, '}'})
	rec := webhookRequest(t, m, "POST", "sample", "signed", "/x", raw, map[string]string{"X-Sig": sign("s3cret", raw)})
	// The signature passes, and only then does the handler fail to parse the JSON.
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("non-UTF-8 body failed signature verification")
	}
}

func TestBlankSecretNeverVerifies(t *testing.T) {
	if verifySignature([]byte("x"), "", sign("", "x"), map[string]any{"prefix": "sha256="}) {
		t.Fatal("a blank secret must not verify")
	}
}

func TestWebhookOnce(t *testing.T) {
	m, _ := bootedSample(t)
	first := webhookRequest(t, m, "POST", "sample", "once", "/x?id=abc", "", nil)
	second := webhookRequest(t, m, "POST", "sample", "once", "/x?id=abc", "", nil)
	other := webhookRequest(t, m, "POST", "sample", "once", "/x?id=def", "", nil)
	if !strings.Contains(first.Body.String(), "true") || !strings.Contains(second.Body.String(), "false") || !strings.Contains(other.Body.String(), "true") {
		t.Fatalf("once results: %s %s %s", first.Body, second.Body, other.Body)
	}
}

func TestTimeoutReplacesVM(t *testing.T) {
	m, _ := bootedSample(t)
	for range 3 {
		if rec := webhookRequest(t, m, "POST", "sample", "spin", "/x", "", nil); rec.Code != http.StatusGatewayTimeout {
			t.Fatalf("got %d", rec.Code)
		}
	}
	if rec := webhookRequest(t, m, "POST", "sample", "open", "/x", "ok", nil); rec.Code != http.StatusOK {
		t.Fatalf("plugin should recover after timeouts, got %d", rec.Code)
	}
}

func TestPanicIsContained(t *testing.T) {
	m, _ := bootedSample(t)
	p := m.Loaded("sample")
	_, err := p.call(context.Background(), func(*vm) (goja.Value, error) { panic("host bug") })
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("expected a contained panic, got %v", err)
	}
	if _, err := p.call(context.Background(), func(v *vm) (goja.Value, error) { return v.rt.ToValue(1), nil }); err != nil {
		t.Fatalf("slot should be usable after a panic: %v", err)
	}
}

func TestSaturationReturnsBusy(t *testing.T) {
	m, _ := bootedSample(t)
	p := m.Loaded("sample")
	var held []*vm
	for range p.Manifest.Server.Concurrency {
		v, err := p.acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.call(ctx, func(v *vm) (goja.Value, error) { return nil, nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
	for _, v := range held {
		p.release(v, false)
	}
}

func TestConcurrentCalls(t *testing.T) {
	m, _ := bootedSample(t)
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			target := "/x?id=" + strings.Repeat("a", i+1)
			if rec := webhookRequest(t, m, "POST", "sample", "once", target, "", nil); rec.Code != http.StatusOK {
				t.Errorf("got %d", rec.Code)
			}
		}()
	}
	wg.Wait()
}

func TestAsyncHandlersAndUnsendableErrors(t *testing.T) {
	app := testApp(t)
	host := &fakeHost{}
	m := testManager(t, app, host, false)
	_ = m.Boot()
	man := `{"schemaVersion":1,"id":"sample","name":"S","version":"1.0.0",
	  "webhooks":[{"path":"a","verify":"none"},{"path":"b","verify":"none"},{"path":"c","verify":"none"}]}`
	r := zipBytes(t, map[string]string{"plugin.json": man, "server.js": `
	  plugin.webhook('a', { async handle() { return { json: { ok: 1 } }; } });
	  plugin.webhook('b', { handle() { return new Promise(() => {}); } });
	  plugin.webhook('c', { async handle() { throw host.errors.unsendable('gone'); } });`})
	if _, err := m.Install(r, r.Size()); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SetEnabled("sample", true)
	m = testManager(t, app, host, false)
	_ = m.Boot()

	if rec := webhookRequest(t, m, "POST", "sample", "a", "/x", "", nil); rec.Code != 200 {
		t.Fatalf("async: %d", rec.Code)
	}
	if rec := webhookRequest(t, m, "POST", "sample", "b", "/x", "", nil); rec.Code != 500 {
		t.Fatalf("pending promise: %d", rec.Code)
	}
	p := m.Loaded("sample")
	_, err := p.call(context.Background(), func(v *vm) (goja.Value, error) {
		h := v.webhooks["c"]
		fn, _ := goja.AssertFunction(h.Get("handle"))
		return fn(h)
	})
	if !errors.Is(err, models.ErrUnsendableDestination) {
		t.Fatalf("expected the unsendable error through a rejection, got %v", err)
	}
}

func TestHostFetchAndLogRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]string{"auth": r.Header.Get("Authorization"), "body": string(b), "method": r.Method})
	}))
	defer srv.Close()

	app := testApp(t)
	host := &fakeHost{}
	var logs bytes.Buffer
	m := New(app, Options{Dir: filepath.Join(app.DataDir(), "plugins"), Log: log.New(&logs, "", 0), Host: host})
	_ = m.Boot()
	man := `{"schemaVersion":1,"id":"sample","name":"S","version":"1.0.0",
	  "settings":[{"key":"token","type":"secret","label":"T"}],
	  "webhooks":[{"path":"a","verify":"none"}]}`
	r := zipBytes(t, map[string]string{"plugin.json": man, "server.js": `
	  plugin.webhook('a', { handle(req) {
	    var s = host.settings.get();
	    host.log.info('using', s.token);
	    var res = host.fetch(req.query.url, { method: 'POST', headers: { Authorization: 'Bearer ' + s.token }, body: 'hi' });
	    return { json: res.json() };
	  } });`})
	if _, err := m.Install(r, r.Size()); err != nil {
		t.Fatal(err)
	}
	_ = host.SavePluginSettings("sample", map[string]any{"token": "topsecret"})
	_, _ = m.SetEnabled("sample", true)
	m = New(app, Options{Dir: filepath.Join(app.DataDir(), "plugins"), Log: log.New(&logs, "", 0), Host: host})
	_ = m.Boot()

	rec := webhookRequest(t, m, "POST", "sample", "a", "/x?url="+srv.URL, "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Bearer topsecret") || !strings.Contains(rec.Body.String(), `"method":"POST"`) {
		t.Fatalf("fetch: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(logs.String(), "topsecret") || !strings.Contains(logs.String(), redacted) {
		t.Fatalf("secret not redacted: %s", logs.String())
	}
}

func TestSettingsMasksAndKeepsSecrets(t *testing.T) {
	m, host := bootedSample(t)
	values, secrets, err := m.SettingsView("sample")
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := values["secret"]; leaked || !secrets["secret"] {
		t.Fatalf("secret leaked or not flagged: %v %v", values, secrets)
	}
	if values["mode"] != "a" {
		t.Fatalf("default not applied: %v", values)
	}

	if err := m.SaveSettings("sample", map[string]any{"secret": "", "mode": "b", "extra": "dropped"}); err != nil {
		t.Fatal(err)
	}
	stored, _ := host.LoadPluginSettings("sample")
	if stored["secret"] != "s3cret" || stored["mode"] != "b" || stored["extra"] != nil {
		t.Fatalf("unexpected stored settings: %v", stored)
	}
	if err := m.SaveSettings("sample", map[string]any{"mode": "zzz"}); err == nil {
		t.Fatal("invalid select value must fail")
	}
	_ = host.DeletePluginSettings("sample")
	if err := m.SaveSettings("sample", map[string]any{"mode": "a"}); err == nil {
		t.Fatal("a missing required secret must fail")
	}
}

func TestUninstall(t *testing.T) {
	m, host := bootedSample(t)
	if err := m.Uninstall("sample", true); err == nil {
		t.Fatal("uninstalling an enabled plugin must fail")
	}
	if _, err := m.SetEnabled("sample", false); err != nil {
		t.Fatal(err)
	}
	if rec := webhookRequest(t, m, "POST", "sample", "open", "/x", "", nil); rec.Code != 404 {
		t.Fatalf("disabled plugin must stop serving, got %d", rec.Code)
	}
	if err := m.Uninstall("sample", true); err != nil {
		t.Fatal(err)
	}
	if len(host.stripped) != 1 || host.stripped[0] != "plugin_sample:manage" {
		t.Fatalf("permissions not stripped: %v", host.stripped)
	}
	if _, err := os.Stat(filepath.Join(m.dir, "sample")); !os.IsNotExist(err) {
		t.Fatal("plugin files should be gone")
	}
	if stored, _ := host.LoadPluginSettings("sample"); len(stored) != 0 {
		t.Fatalf("purge should drop settings: %v", stored)
	}
}

func TestUpgradeEnabledPlugin(t *testing.T) {
	m, _ := bootedSample(t)
	same := zipBytes(t, map[string]string{"plugin.json": testManifest, "server.js": testServer})
	if _, err := m.Install(same, same.Size()); err == nil {
		t.Fatal("reinstalling the running version must fail")
	}
	next := zipBytes(t, map[string]string{"plugin.json": strings.Replace(testManifest, "1.0.0", "1.1.0", 1), "server.js": testServer})
	res, err := m.Install(next, next.Size())
	if err != nil {
		t.Fatal(err)
	}
	if !res.NeedsRestart {
		t.Fatal("upgrading an enabled plugin needs a restart")
	}
	if m.Loaded("sample").Manifest.Version != "1.0.0" {
		t.Fatal("running code must stay on the old version until restart")
	}
}

func TestMessengerSend(t *testing.T) {
	app := testApp(t)
	host := &fakeHost{}
	m := testManager(t, app, host, false)
	_ = m.Boot()
	man := `{"schemaVersion":1,"id":"sample","name":"S","version":"1.0.0","channels":[{"id":"ch:sample","label":"S"}]}`
	r := zipBytes(t, map[string]string{"plugin.json": man, "server.js": `
	  plugin.channel('ch:sample', { send(msg) {
	    if (msg.body === 'hard') throw host.errors.unsendable('gone');
	    if (msg.body === 'soft') throw new Error('try later');
	    if (msg.body === 'someBad') return { invalidAddresses: [msg.to[0]], providerMsgId: 'p1' };
	    if (msg.body === 'allBad') return { invalidAddresses: msg.to };
	    if (msg.fields.mode !== 'x' || msg.subscriber.email !== '') throw new Error('bad view');
	    return { providerMsgId: 'p2' };
	  } });`})
	if _, err := m.Install(r, r.Size()); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SetEnabled("sample", true)
	m = testManager(t, app, host, false)
	_ = m.Boot()

	msgrs, specs := m.Messengers()
	if len(msgrs) != 1 || specs[0].Consent != models.ConsentSourceChannel || !specs[0].TextPipeline || specs[0].EmailHeaders {
		t.Fatalf("unexpected spec: %+v", specs)
	}
	msgr := msgrs[0]
	camp := &models.Campaign{Attribs: models.JSON{CampaignFieldsAttrib: map[string]any{"mode": "x"}}}
	send := func(body string, to ...string) error {
		return msgr.Push(models.Message{To: to, Body: []byte(body), Campaign: camp,
			Subscriber: models.Subscriber{Email: models.NewPlaceholderEmail()}})
	}

	if err := send("ok", "a"); err != nil {
		t.Fatalf("ok send: %v", err)
	}
	if len(host.events) != 1 || host.events[0].ProviderMsgID != "p2" || host.events[0].Direction != "outbound" {
		t.Fatalf("send not recorded: %+v", host.events)
	}
	if err := send("ok"); !errors.Is(err, models.ErrUnsendableDestination) {
		t.Fatalf("no addresses should be unsendable, got %v", err)
	}
	if err := send("hard", "a", "b"); !errors.Is(err, models.ErrUnsendableDestination) || len(host.invalid) != 2 {
		t.Fatalf("hard failure: %v %v", err, host.invalid)
	}
	host.invalid = nil
	if err := send("soft", "a"); err == nil || errors.Is(err, models.ErrUnsendableDestination) {
		t.Fatalf("a soft failure must stay retryable, got %v", err)
	}
	if err := send("someBad", "a", "b"); err != nil || len(host.invalid) != 1 || host.invalid[0] != "a" {
		t.Fatalf("partial failure: %v %v", err, host.invalid)
	}
	host.invalid = nil
	if err := send("allBad", "a"); !errors.Is(err, models.ErrUnsendableDestination) {
		t.Fatalf("all addresses rejected should be unsendable, got %v", err)
	}
}

func TestPhoneChannelAndNewHostCalls(t *testing.T) {
	app := testApp(t)
	host := &fakeHost{}
	m := testManager(t, app, host, false)
	_ = m.Boot()
	man := `{"schemaVersion":1,"id":"sample","name":"S","version":"1.0.0",
	  "channels":[{"id":"ch:sample","label":"S","addressSource":"phone","campaignBody":"none"}],
	  "webhooks":[{"path":"a","verify":"none"},{"path":"foreign","verify":"none"}]}`
	r := zipBytes(t, map[string]string{"plugin.json": man, "server.js": `
	  plugin.channel('ch:sample', { send(msg) {
	    if (msg.subscriber.first_name !== 'Ada' || msg.fields.tpl !== 'hello') throw new Error('bad view');
	    return {};
	  } });
	  plugin.webhook('a', { handle() {
	    return { json: {
	      imported: host.contacts.importSMSConsent('ch:sample', { dryRun: true }).explicit,
	      counts: host.events.counts('c1').read,
	      listed: host.events.list({ subscriberId: 's', channel: 'ch:sample' }) === null ? 0 : 0,
	    } };
	  } });
	  plugin.webhook('foreign', { handle() { return { json: host.events.list({ subscriberId: 's', channel: 'ch:other' }) }; } });`})
	if _, err := m.Install(r, r.Size()); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SetEnabled("sample", true)
	m = testManager(t, app, host, false)
	_ = m.Boot()

	msgrs, specs := m.Messengers()
	if specs[0].Address != models.AddressSourcePhone || specs[0].CampaignBody != models.CampaignBodyNone {
		t.Fatalf("unexpected spec: %+v", specs[0])
	}
	err := msgrs[0].Push(models.Message{
		To:         []string{"+15550100"},
		Subscriber: models.Subscriber{FirstName: "Ada"},
		TxMessage:  &models.TransactionalMessage{Data: models.JSON{"tpl": "hello"}},
	})
	if err != nil {
		t.Fatalf("transactional data should reach msg.fields: %v", err)
	}

	rec := webhookRequest(t, m, "POST", "sample", "a", "/x", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"imported":1`) || !strings.Contains(rec.Body.String(), `"counts":1`) {
		t.Fatalf("host calls: %d %s", rec.Code, rec.Body.String())
	}
	if rec := webhookRequest(t, m, "POST", "sample", "foreign", "/x", "", nil); rec.Code != 500 {
		t.Fatalf("reading another channel must be refused, got %d", rec.Code)
	}
}
