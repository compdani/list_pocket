package auth

import (
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/golang-jwt/jwt/v5"
	null "gopkg.in/volatiletech/null.v6"
)

func TestExchangeJWTValidation(t *testing.T) {
	old := sharedSecret
	t.Cleanup(func() { sharedSecret = old })
	for _, tc := range []struct {
		name, secret, email string
		exp                 *jwt.NumericDate
		valid               bool
	}{
		{"empty key", "", "a@example.com", jwt.NewNumericDate(time.Now().Add(time.Hour)), false},
		{"no expiration", "secret", "a@example.com", nil, false},
		{"expired", "secret", "a@example.com", jwt.NewNumericDate(time.Now().Add(-time.Hour)), false},
		{"no email", "secret", "", jwt.NewNumericDate(time.Now().Add(time.Hour)), false},
		{"valid", "secret", "a@example.com", jwt.NewNumericDate(time.Now().Add(time.Hour)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sharedSecret = []byte(tc.secret)
			signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, UserClaims{Email: tc.email, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: tc.exp}}).SignedString(sharedSecret)
			if err != nil {
				t.Fatal(err)
			}
			_, err = validateUserJWT(signed)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func TestNativeAuthPolicyAndSecretExport(t *testing.T) {
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	a, err := New(Config{AuthCollection: "users"}, nil, app, &Callbacks{}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	u := User{Username: "admin", Email: null.StringFrom("admin@example.com"), Type: UserTypeUser, Status: UserStatusEnabled, PasswordLogin: true, TwofaType: "totp", TwofaKey: null.StringFrom("secret")}
	if err := a.SyncUser(u); err != nil {
		t.Fatal(err)
	}
	rec, err := a.pbAuth.FindByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rec.PublicExport()["twofa_key"]; ok {
		t.Fatal("TOTP secret is public")
	}
	// Repair existing schemas as well as new fields.
	col := rec.Collection()
	col.Fields.GetByName("twofa_key").SetHidden(false)
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	if err := a.ensureAuthCollection(); err != nil {
		t.Fatal(err)
	}
	rec, err = a.pbAuth.FindByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rec.PublicExport()["twofa_key"]; ok {
		t.Fatal("schema repair exposed secret")
	}
	// Exercise the native HTTP pipeline, not just the auth response helper.
	t.Setenv("shared_secret", "exchange-test-key")
	routes, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	RegisterExchangeRoutes(routes)
	mux, err := routes.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, status, twofa, kind string
		password, allowed         bool
	}{
		{"enabled", UserStatusEnabled, "", UserTypeUser, true, true},
		{"disabled", UserStatusDisabled, "", UserTypeUser, true, false},
		{"password disabled", UserStatusEnabled, "", UserTypeUser, false, false},
		{"totp", UserStatusEnabled, "totp", UserTypeUser, true, false},
		{"api", UserStatusEnabled, "", UserTypeAPI, true, false},
	} {
		t.Run("http "+tc.name, func(t *testing.T) {
			rec.Set("status", tc.status)
			rec.Set("twofa_type", tc.twofa)
			rec.Set("user_type", tc.kind)
			rec.Set("password_login", tc.password)
			rec.SetPassword("test-password")
			if err := app.Save(rec); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("POST", "/api/collections/users/auth-with-password", strings.NewReader(`{"identity":"admin@example.com","password":"test-password"}`))
			request.Header.Set("Content-Type", "application/json")
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, request)
			if (out.Code == 200) != tc.allowed {
				t.Fatalf("password status=%d body=%s", out.Code, out.Body)
			}
			session, err := rec.NewAuthToken()
			if err != nil {
				t.Fatal(err)
			}
			request = httptest.NewRequest("POST", "/api/collections/users/auth-refresh", nil)
			request.Header.Set("Authorization", session)
			out = httptest.NewRecorder()
			mux.ServeHTTP(out, request)
			if (out.Code == 200) != (tc.status == UserStatusEnabled && tc.kind == UserTypeUser) {
				t.Fatalf("refresh status=%d body=%s", out.Code, out.Body)
			}
			resetToken, err := rec.NewPasswordResetToken()
			if err != nil {
				t.Fatal(err)
			}
			for _, resetRoute := range []struct{ path, body string }{
				{"request-password-reset", `{"email":"admin@example.com"}`},
				{"confirm-password-reset", fmt.Sprintf(`{"token":%q,"password":"replacement-password","passwordConfirm":"replacement-password"}`, resetToken)},
			} {
				request = httptest.NewRequest("POST", "/api/collections/users/"+resetRoute.path, strings.NewReader(resetRoute.body))
				request.Header.Set("Content-Type", "application/json")
				out = httptest.NewRecorder()
				mux.ServeHTTP(out, request)
				if out.Code != 403 {
					t.Fatalf("native reset bypassed application flow: %s %d %s", resetRoute.path, out.Code, out.Body)
				}
			}

			claims := UserClaims{Email: rec.Email(), RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
			exchange, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("exchange-test-key"))
			if err != nil {
				t.Fatal(err)
			}
			request = httptest.NewRequest("GET", "/user_token", nil)
			request.Header.Set("ExchangeToken", exchange)
			out = httptest.NewRecorder()
			mux.ServeHTTP(out, request)
			wantExchange := tc.status == UserStatusEnabled && tc.kind == UserTypeUser && tc.twofa == ""
			if (out.Code == 200) != wantExchange {
				t.Fatal(fmt.Sprintf("exchange status=%d body=%s", out.Code, out.Body))
			}
			if strings.Contains(out.Body.String(), `"twofa_key"`) {
				t.Fatal("exchange exported secret")
			}
		})
	}
	rec.Set("user_type", UserTypeUser)

	for _, tc := range []struct {
		name, status, method, twofa string
		password, allowed           bool
	}{
		{"disabled refresh", UserStatusDisabled, "", "", true, false},
		{"TOTP native password", UserStatusEnabled, pbcore.MFAMethodPassword, "totp", true, false},
		{"TOTP exchange", UserStatusEnabled, "listpocket-token-exchange", "totp", true, false},
		{"password disabled", UserStatusEnabled, pbcore.MFAMethodPassword, "", false, false},
		{"enabled refresh", UserStatusEnabled, "", "totp", true, true},
		{"enabled native password", UserStatusEnabled, pbcore.MFAMethodPassword, "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec.Set("status", tc.status)
			rec.Set("twofa_type", tc.twofa)
			rec.Set("password_login", tc.password)
			e := &pbcore.RequestEvent{App: app}
			e.Request = httptest.NewRequest("POST", "/", nil)
			e.Response = httptest.NewRecorder()
			err := apis.RecordAuthResponse(e, rec, tc.method, nil)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v err=%v", tc.allowed, err)
			}
		})
	}
}
