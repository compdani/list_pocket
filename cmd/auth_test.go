package main

import (
	"html"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/compdani/list_pocket/internal/i18n"
	pbcore "github.com/pocketbase/pocketbase/core"
)

// newSetupTestApp returns an App awaiting first-time setup that renders the real
// public templates and language file, so a template that doesn't match its data fails here.
func newSetupTestApp(t *testing.T) *App {
	t.Helper()

	b, err := os.ReadFile("../i18n/en.json")
	if err != nil {
		t.Fatal(err)
	}
	lang, err := i18n.New(b)
	if err != nil {
		t.Fatal(err)
	}

	urlCfg := &UrlConfig{}
	tpl, err := parseTemplatesFS(initTplFuncs(lang, urlCfg), os.DirFS("../static"), "public/templates/*.html")
	if err != nil {
		t.Fatal(err)
	}

	return &App{
		i18n:           lang,
		log:            log.New(io.Discard, "", 0),
		renderer:       newTplRenderer(tpl, &Config{}, urlCfg, lang),
		needsUserSetup: true,
	}
}

func serveLoginSetup(t *testing.T, a *App, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	re := &pbcore.RequestEvent{}
	re.Request = r
	re.Response = rec
	re.Set(appContextKey, a)
	if err := a.LoginSetupPage(re); err != nil {
		t.Fatalf("LoginSetupPage: %v", err)
	}
	return rec
}

func TestLoginSetupPageRenders(t *testing.T) {
	a := newSetupTestApp(t)

	rec := serveLoginSetup(t, a, httptest.NewRequest(http.MethodGet, "/admin/setup?next=/admin/lists", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`name="email"`, `name="username"`, `name="password"`, `name="password2"`,
		`name="next" value="/admin/lists"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("setup page is missing %q", want)
		}
	}
	if strings.Contains(body, `class="error"`) {
		t.Error("setup page shows an error before anything was submitted")
	}
}

func TestLoginSetupPageShowsErrorOnFailedPost(t *testing.T) {
	a := newSetupTestApp(t)

	form := url.Values{
		"email":     {"admin@example.com"},
		"username":  {"admin"},
		"password":  {"password-one"},
		"password2": {"password-two"},
	}
	r := httptest.NewRequest(http.MethodPost, "/admin/setup", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := serveLoginSetup(t, a, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if want := html.EscapeString(a.i18n.T("users.passwordMismatch")); !strings.Contains(rec.Body.String(), want) {
		t.Errorf("failed POST does not show %q", want)
	}
	if !a.isSetupRequired() {
		t.Error("a failed POST must leave setup required")
	}
}
