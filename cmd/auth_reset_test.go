package main

import (
	"encoding/json"
	"html"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase"
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/auth"
	appcore "github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/notifs"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	null "gopkg.in/volatiletech/null.v6"
)

type resetMailCapture struct{ messages []models.Message }

func (m *resetMailCapture) Name() string { return "email" }
func (m *resetMailCapture) Push(msg models.Message) error {
	m.messages = append(m.messages, msg)
	return nil
}
func (m *resetMailCapture) Flush() error { return nil }
func (m *resetMailCapture) Close() error { return nil }

func TestResetUsesNativeTokenAndRequiresLogin(t *testing.T) {
	a := newSetupTestApp(t)
	pb := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := pb.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pb.ResetBootstrapState() })
	logger := log.New(io.Discard, "", 0)
	au, err := auth.New(auth.Config{}, nil, pb, &auth.Callbacks{}, logger)
	if err != nil {
		t.Fatal(err)
	}
	db, err := pbdb.NewFromPocketBase(pb)
	if err != nil {
		t.Fatal(err)
	}
	a.core = appcore.New(&appcore.Opt{DB: db, I18n: a.i18n, Log: logger}, &appcore.Hooks{})
	u := auth.User{Username: "alice", Email: null.StringFrom("alice@example.com"), Type: auth.UserTypeUser, Status: auth.UserStatusEnabled, PasswordLogin: true, Password: null.StringFrom("old-password"), TwofaType: "totp", TwofaKey: null.StringFrom("secret")}
	rec, err := au.UpsertUserAuthRecord(u, "")
	if err != nil {
		t.Fatal(err)
	}
	a.auth, err = auth.New(auth.Config{}, nil, pb, &auth.Callbacks{GetUserByUsername: func(username string) (auth.User, error) { return a.core.GetUser("", username, "") }}, logger)
	if err != nil {
		t.Fatal(err)
	}
	a.urlCfg = &UrlConfig{RootURL: "https://example.com"}
	a.cfg = &Config{FromEmail: "admin@example.com"}
	capture := &resetMailCapture{}
	a.emailMsgr = capture
	oldTemplates := notifs.Tpls
	t.Cleanup(func() { notifs.Tpls = oldTemplates })
	notifs.Tpls, err = parseTemplatesFS(initTplFuncs(a.i18n, a.urlCfg), os.DirFS(".."), "static/email-templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	forgot := func() error {
		e := &pbcore.RequestEvent{App: pb}
		e.Request = httptest.NewRequest("POST", "/mailapi/auth/forgot", strings.NewReader(`{"email":"alice@example.com"}`))
		e.Request.Header.Set("Content-Type", "application/json")
		e.Response = httptest.NewRecorder()
		return a.AuthForgotPassword(e)
	}
	if err := forgot(); err != nil {
		t.Fatal(err)
	}
	if len(capture.messages) != 1 {
		t.Fatalf("reset emails=%d", len(capture.messages))
	}
	link := regexp.MustCompile(`href="([^"]+/admin/reset[^"]+)"`).FindSubmatch(capture.messages[0].Body)
	if len(link) != 2 {
		t.Fatalf("reset URL missing from mail: %s", capture.messages[0].Body)
	}
	resetURL, err := url.Parse(html.UnescapeString(string(link[1])))
	if err != nil {
		t.Fatal(err)
	}
	token := resetURL.Query().Get("token")
	oldSession, err := rec.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	reset := func(token string) (*httptest.ResponseRecorder, error) {
		body, _ := json.Marshal(map[string]string{"email": u.Email.String, "token": token, "password": "new-password", "password2": "new-password"})
		e := &pbcore.RequestEvent{App: pb}
		e.Request = httptest.NewRequest("POST", "/mailapi/auth/reset", strings.NewReader(string(body)))
		e.Request.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		e.Response = out
		return out, a.AuthResetPassword(e)
	}
	if _, err := reset(token + "invalid"); err == nil {
		t.Fatal("invalid token accepted")
	}
	// Explicit password policy must survive both Core conversion passes.
	rec.Set("password_login", false)
	if err := pb.Save(rec); err != nil {
		t.Fatal(err)
	}
	lookedUp, err := a.core.GetUser(rec.Id, "", "")
	if err != nil || lookedUp.PasswordLogin {
		t.Fatalf("password policy overridden: %+v %v", lookedUp, err)
	}
	if _, err := a.auth.LoginUser("alice", "old-password"); err == nil {
		t.Fatal("disabled password login accepted")
	}
	if err := forgot(); err != nil {
		t.Fatal(err)
	}
	if len(capture.messages) != 1 {
		t.Fatal("password-disabled user received reset mail")
	}
	if _, err := reset(token); err == nil {
		t.Fatal("password-disabled reset accepted")
	}
	persisted, err := pb.FindRecordById("users", rec.Id)
	if err != nil || persisted.GetBool("password_login") {
		t.Fatal("stored policy changed")
	}
	rec.Set("password_login", true)
	if err := pb.Save(rec); err != nil {
		t.Fatal(err)
	}
	// Tokens survive a process/bootstrap lifecycle, unlike the old temporary map.
	if err := pb.ResetBootstrapState(); err != nil {
		t.Fatal(err)
	}
	if err := pb.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	db, err = pbdb.NewFromPocketBase(pb)
	if err != nil {
		t.Fatal(err)
	}
	a.core = appcore.New(&appcore.Opt{DB: db, I18n: a.i18n, Log: logger}, &appcore.Hooks{})
	out, err := reset(token)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.Body.String(), `"token"`) || !strings.Contains(out.Body.String(), `/admin/login`) {
		t.Fatalf("reset issued auth or missed login: %s", out.Body)
	}
	if _, err := reset(token); err == nil {
		t.Fatal("used reset token replayed")
	}
	if _, err := pb.FindAuthRecordByToken(oldSession, pbcore.TokenTypeAuth); err == nil {
		t.Fatal("old session survived password reset")
	}
	eLogin := &pbcore.RequestEvent{App: pb}
	eLogin.Request = httptest.NewRequest("POST", "/mailapi/auth/login", strings.NewReader(`{"username":"alice","password":"new-password"}`))
	eLogin.Request.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	eLogin.Response = loginResponse
	if err := a.AuthLogin(eLogin); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loginResponse.Body.String(), `"twofa_required"`) {
		t.Fatalf("reset bypassed TOTP: %s", loginResponse.Body)
	}
	// The same persisted eligibility check applies to existing tokens on mailapi.
	disabled, err := pb.FindRecordById("users", rec.Id)
	if err != nil {
		t.Fatal(err)
	}
	disabled.Set("status", auth.UserStatusDisabled)
	if err := pb.Save(disabled); err != nil {
		t.Fatal(err)
	}
	e := &pbcore.RequestEvent{App: pb, Auth: disabled}
	e.Request = httptest.NewRequest("GET", "/mailapi/health", nil)
	response := httptest.NewRecorder()
	e.Response = response
	e.Set(appContextKey, a)
	if err := hydrateAuthUser(e); err != nil {
		t.Fatal(err)
	}
	if response.Code != 403 {
		t.Fatalf("disabled existing token status=%d", response.Code)
	}

	updated, err := pb.FindRecordById("users", rec.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ValidatePassword("old-password") {
		t.Fatal("old password accepted")
	}
	if !updated.ValidatePassword("new-password") {
		t.Fatal("password not updated")
	}
}
