package chatgpt_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/compdani/list_pocket/internal/migrations"

	"github.com/compdani/list_pocket/internal/flow/chatgpt"
	"github.com/compdani/list_pocket/internal/flow/llm"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// mockAuth is a stand-in ChatGPT authorization server.
type mockAuth struct {
	*httptest.Server
	key    *rsa.PrivateKey
	scope  string
	nonce  string
	mu     sync.Mutex
	forms  []url.Values
	issued int
}

func newMockAuth(t *testing.T) *mockAuth {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := &mockAuth{key: key, scope: chatgpt.Scopes}
	mux := http.NewServeMux()
	m.Server = httptest.NewServer(mux)
	t.Cleanup(m.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": m.URL, "jwks_uri": m.URL + "/jwks", "revocation_endpoint": m.URL + "/revoke"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		m.record(r.PostForm)
	})
	mux.HandleFunc("/api/accounts/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		m.record(r.PostForm)
		m.mu.Lock()
		m.issued++
		n := m.issued
		m.mu.Unlock()
		f := r.PostForm
		switch f.Get("grant_type") {
		case "authorization_code":
			if f.Get("code") != "good-code" || f.Get("code_verifier") == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
		case "refresh_token":
			if f.Get("refresh_token") != "rt-"+itoa(n-1) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "refresh token reused"})
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-" + itoa(n), "refresh_token": "rt-" + itoa(n), "expires_in": 3600,
			"scope": m.scope, "id_token": m.idToken(t, f.Get("client_id")),
		})
	})
	return m
}

func itoa(n int) string { return big.NewInt(int64(n)).String() }

func (m *mockAuth) record(f url.Values) {
	m.mu.Lock()
	m.forms = append(m.forms, f)
	m.mu.Unlock()
}

func (m *mockAuth) idToken(t *testing.T, aud string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": m.URL, "aud": aud, "sub": "user-1", "email": "ann@example.com", "nonce": m.nonce,
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	})
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(m.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// signIn runs Begin + Complete against the mock with an issued client ID.
func signIn(t *testing.T, m *mockAuth, o *chatgpt.OAuth) (*chatgpt.Credentials, error) {
	p := o.Begin("host-1", "http://127.0.0.1:1455/auth/callback", nil)
	m.nonce = p.Nonce
	return o.Complete(context.Background(), p, url.Values{"state": {p.State}, "code": {"good-code"}, "client_id": {"oaiapp_123"}})
}

func TestBeginRegistersDynamicClientThenReusesIssued(t *testing.T) {
	o := &chatgpt.OAuth{AuthBase: "https://auth.example"}
	first, _ := url.Parse(o.Begin("host-1", "http://127.0.0.1:1455/auth/callback", nil).AuthURL)
	q := first.Query()
	if first.Path != "/api/accounts/authorize" || q.Get("client_id") != chatgpt.DynamicClient || q.Get("agent_name_hint") != chatgpt.AgentName {
		t.Errorf("first registration URL: %s", first)
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("resource") != chatgpt.Resource || q.Get("ext_agent_host_id") != "host-1" || !strings.Contains(q.Get("scope"), chatgpt.PlanScope) {
		t.Errorf("authorize params: %v", q)
	}
	again, _ := url.Parse(o.Begin("host-1", "http://x/cb", &chatgpt.Credentials{ClientID: "oaiapp_123", IDToken: "idt"}).AuthURL)
	if again.Query().Get("client_id") != "oaiapp_123" || again.Query().Get("id_token_hint") != "idt" || again.Query().Get("agent_name_hint") != "" {
		t.Errorf("returning sign-in URL: %s", again)
	}
}

func TestCompleteValidatesAndRequiresPlan(t *testing.T) {
	m := newMockAuth(t)
	o := &chatgpt.OAuth{AuthBase: m.URL}
	cred, err := signIn(t, m, o)
	if err != nil {
		t.Fatal(err)
	}
	if cred.ClientID != "oaiapp_123" || cred.Sub != "user-1" || cred.Email != "ann@example.com" || cred.RefreshToken != "rt-1" || !cred.HasPlan() {
		t.Errorf("credentials: %+v", cred)
	}
	if f := m.forms[0]; f.Get("client_id") != "oaiapp_123" || f.Get("resource") != chatgpt.Resource || f.Get("redirect_uri") == "" {
		t.Errorf("token exchange form: %v", f)
	}

	// Wrong state, declined consent, missing issued client, bad nonce, no plan scope.
	p := o.Begin("host-1", "http://127.0.0.1:1455/auth/callback", nil)
	for name, cb := range map[string]url.Values{
		"state":    {"state": {"nope"}, "code": {"good-code"}, "client_id": {"oaiapp_123"}},
		"declined": {"state": {p.State}, "error": {"access_denied"}},
		"client":   {"state": {p.State}, "code": {"good-code"}},
	} {
		if _, err := o.Complete(context.Background(), p, cb); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	m.nonce = "other"
	if _, err := o.Complete(context.Background(), p, url.Values{"state": {p.State}, "code": {"good-code"}, "client_id": {"oaiapp_123"}}); err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Errorf("nonce mismatch should fail: %v", err)
	}
	m.scope = "openid profile email offline_access"
	if _, err := signIn(t, m, o); !errors.Is(err, chatgpt.ErrPlanNotGranted) {
		t.Errorf("missing plan scope should fail with ErrPlanNotGranted: %v", err)
	}
}

func TestStoreRefreshesAndRotates(t *testing.T) {
	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{DataDir: t.TempDir(), EncryptionEnv: "pb_test_env"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	if err := app.RunAllMigrations(); err != nil {
		t.Fatal(err)
	}
	m := newMockAuth(t)
	now := time.Now()
	o := &chatgpt.OAuth{AuthBase: m.URL, Now: func() time.Time { return now }}
	store := &chatgpt.Store{App: app, OAuth: o}

	if _, err := store.Token(context.Background()); !errors.Is(err, chatgpt.ErrNotSignedIn) {
		t.Fatalf("expected ErrNotSignedIn, got %v", err)
	}
	cred, err := signIn(t, m, o)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(cred); err != nil {
		t.Fatal(err)
	}
	host := store.HostID()
	if !regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(host) || store.HostID() != host {
		t.Errorf("host ID should be a stable urn:uuid v4, got %q", host)
	}
	// A pre-urn value from older builds is replaced once.
	if rec, err := app.FindFirstRecordByData("ai_settings", "key", "chatgpt_host_id"); err == nil {
		rec.Set("value", "harness-0123abcd")
		_ = app.Save(rec)
	}
	if host = store.HostID(); !strings.HasPrefix(host, "urn:uuid:") || store.HostID() != host {
		t.Errorf("legacy host ID not replaced: %q", host)
	}

	tok, err := store.Token(context.Background())
	if err != nil || tok != "at-1" {
		t.Fatalf("fresh token should be reused: %q %v", tok, err)
	}

	// Within 10 minutes of expiry: refresh, rotate, persist.
	now = now.Add(55 * time.Minute)
	if tok, err = store.Token(context.Background()); err != nil || tok != "at-2" {
		t.Fatalf("refresh: %q %v", tok, err)
	}
	saved, _ := store.Load()
	if saved.RefreshToken != "rt-2" || saved.ClientID != "oaiapp_123" {
		t.Errorf("rotated refresh token not saved: %+v", saved)
	}
	now = now.Add(55 * time.Minute)
	if tok, err = store.Token(context.Background()); err != nil || tok != "at-3" {
		t.Fatalf("second refresh must use the rotated token: %q %v", tok, err)
	}

	st := store.Status()
	if !st.SignedIn || !st.Plan || st.Email != "ann@example.com" {
		t.Errorf("status: %+v", st)
	}
	raw, _ := json.Marshal(st)
	if strings.Contains(string(raw), "rt-") || strings.Contains(string(raw), "at-") {
		t.Errorf("status must not leak tokens: %s", raw)
	}

	// The resolver uses the store for the plan provider.
	r := &llm.Resolver{Getenv: func(string) string { return "" }, Tokens: store.Token}
	ep, err := r.Endpoint(llm.ProviderChatGPT)
	if err != nil || ep.Kind != llm.KindResponses || ep.APIKey != "at-3" || ep.BaseURL != chatgpt.Resource {
		t.Errorf("endpoint: %+v %v", ep, err)
	}

	// Import rejects unusable files and keeps this install's host ID.
	if err := store.Import(&chatgpt.Credentials{ClientID: chatgpt.DynamicClient, RefreshToken: "x", Scopes: []string{chatgpt.PlanScope}}); err == nil {
		t.Error("import should reject an unissued client ID")
	}
	if err := store.Import(&chatgpt.Credentials{ClientID: "oaiapp_9", RefreshToken: "rt-3", Scopes: []string{chatgpt.PlanScope}, HostID: "elsewhere"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Load(); got.HostID != host {
		t.Errorf("import should keep host ID %s, got %s", host, got.HostID)
	}

	if err := o.Revoke(context.Background(), saved); err != nil {
		t.Errorf("revoke: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if store.Status().SignedIn {
		t.Error("cleared store should be signed out")
	}
}

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"slug":"gpt-a","display_name":"GPT A","visibility":"list"},{"slug":"hidden","visibility":"hide"},{"id":"gpt-b"}]}`))
	}))
	defer srv.Close()
	models, err := chatgpt.ListModels(context.Background(), nil, srv.URL, "tok")
	if err != nil || len(models) != 2 || models[0].Slug != "gpt-a" || models[1].DisplayName != "gpt-b" {
		t.Errorf("models: %+v %v", models, err)
	}
	if _, err := chatgpt.ListModels(context.Background(), nil, srv.URL, "bad"); err == nil {
		t.Error("expected 401 error")
	}
}

func TestLoopbackReceivesCallback(t *testing.T) {
	lb, err := chatgpt.Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		resp, err := http.Get(lb.RedirectURI + "?state=s&code=c")
		if err == nil {
			resp.Body.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q, err := lb.Wait(ctx)
	if err != nil || q.Get("code") != "c" {
		t.Fatalf("callback: %v %v", q, err)
	}
	if !strings.HasPrefix(lb.RedirectURI, "http://127.0.0.1:") {
		t.Errorf("redirect must be loopback: %s", lb.RedirectURI)
	}
}
