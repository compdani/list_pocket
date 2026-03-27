package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"image/png"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/notifs"
	"github.com/compdani/list_pocket/internal/tmptokens"
	"github.com/compdani/list_pocket/internal/utils"
	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
	"gopkg.in/volatiletech/null.v6"
)

const (
	passwordResetTTL = 30 * time.Minute
	twofaTokenTTL    = 5 * time.Minute

	// Length of reset and 2FA auth tokens.
	tmpAuthTokenLen = 64
)

type loginTpl struct {
	Title       string
	Description string

	NextURI         string
	PasswordEnabled bool
	Error           string
}

type forgotPasswordTpl struct {
	Title       string
	Description string
	Error       string
}

type resetPasswordTpl struct {
	Title       string
	Description string
	Token       string
	Email       string
	Error       string
}

type twofaTpl struct {
	Title       string
	Description string
	Token       string
	NextURI     string
	Error       string
}

type authBridgeTpl struct {
	Title       string
	Description string
	NextURI     string
	PayloadJSON template.JS
}

func getRequestedNextURI(c echo.Context) string {
	if c.Request().Method == http.MethodGet {
		return utils.SanitizeURI(c.QueryParam("next"))
	}

	return utils.SanitizeURI(c.FormValue("next"))
}

func adminRedirectPath(next string) string {
	next = utils.SanitizeURI(next)
	if next == "" || next == "/" {
		return uriAdmin
	}
	if next == uriAdmin || strings.HasPrefix(next, uriAdmin+"/") {
		return next
	}
	return path.Join(uriAdmin, next)
}

// LoginPage renders the login page and handles the login form.
func (a *App) LoginPage(c echo.Context) error {
	// Has the user been setup?
	a.Lock()
	needsUserSetup := a.needsUserSetup
	a.Unlock()

	if needsUserSetup {
		return a.LoginSetupPage(c)
	}

	// Process POST login request.
	var loginErr error
	if c.Request().Method == http.MethodPost {
		loginErr = a.doLogin(c)
		if loginErr == nil {
			return c.Redirect(http.StatusFound, utils.SanitizeURI(c.FormValue("next")))
		}
	}

	// Render the page, with or without POST.
	return a.renderLoginPage(c, loginErr)
}

// LoginSetupPage renders the first time user login page and handles the login form.
func (a *App) LoginSetupPage(c echo.Context) error {
	// Process POST login request.
	var loginErr error
	if c.Request().Method == http.MethodPost {
		loginErr = a.doFirstTimeSetup(c)
		if loginErr == nil {
			a.Lock()
			a.needsUserSetup = false
			a.Unlock()
			return c.Redirect(http.StatusFound, utils.SanitizeURI(c.FormValue("next")))
		}
	}

	// Render the page, with or without POST.
	return a.renderLoginSetupPage(c, loginErr)
}

// TwofaPage renders the 2FA verification page and handles the 2FA form submission.
func (a *App) TwofaPage(c echo.Context) error {
	var token, next string

	if c.Request().Method == http.MethodPost {
		token = strings.TrimSpace(c.FormValue("token"))
		next = utils.SanitizeURI(c.FormValue("next"))
	} else {
		token = strings.TrimSpace(c.QueryParam("token"))
		next = utils.SanitizeURI(c.QueryParam("next"))
	}

	// If there's no token, redirect.
	if len(token) < tmpAuthTokenLen {
		return c.Redirect(http.StatusFound, uriAdmin)
	}

	if next == "" || next == "/" {
		next = uriAdmin
	}

	// Validate the 2FA temp token.
	data, err := tmptokens.Check(token)
	if err != nil {
		return c.Redirect(http.StatusFound, uriAdmin)
	}

	userRecordID, ok := data.(string)
	if !ok {
		return a.renderTwofaPage(c, token, next, a.i18n.T("users.invalidRequest"))
	}

	// Process the 2FA verification POST request.
	if c.Request().Method == http.MethodPost {
		return a.doTwofaVerify(c, token, userRecordID, next)
	}

	// Render the 2FA verification page.
	return a.renderTwofaPage(c, token, next, "")
}

// Logout logs a user out.
func (a *App) Logout(c echo.Context) error {
	// API auth is token-based via PocketBase. Logout is handled by clearing
	// the token on the client and does not depend on server-side sessions.
	return c.JSON(http.StatusOK, okResp{true})
}

// ForgotPage renders the forgot password page and handles the forgot password form.
func (a *App) ForgotPage(c echo.Context) error {
	// Process the forgot password request.
	if c.Request().Method == http.MethodPost {
		return a.doForgotPassword(c)
	}

	// Render the forgot page.
	out := forgotPasswordTpl{Title: a.i18n.T("users.forgotPassword")}
	return c.Render(http.StatusOK, "admin-forgot-password", out)
}

// ResetPage renders the reset password page and handles the reset password form.
func (a *App) ResetPage(c echo.Context) error {
	var (
		token = strings.TrimSpace(c.QueryParam("token"))
		email = strings.ToLower(strings.TrimSpace(c.QueryParam("email")))
	)

	// Validate token and email (don't delete it yet, as we may need it for POST).
	data, err := tmptokens.Check(email)
	if err != nil {
		return c.Render(http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("users.invalidResetLink")))
	}

	tk, ok := data.(string)
	if !ok || tk != token {
		return c.Render(http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("users.invalidResetLink")))
	}

	// Validate that the user exists.
	_, err = a.core.GetUser("", "", email)
	if err != nil {
		return c.Render(http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("users.invalidResetLink")))
	}

	// Process the reset password request form with the new passwords.
	if c.Request().Method == http.MethodPost {
		return a.doResetPassword(c, token, email)
	}

	// Render the reset password form for GET request.
	return a.renderResetPasswordPage(c, token, email, "")
}

// renderLoginPage renders the login page and handles the login form.
func (a *App) renderLoginPage(c echo.Context, loginErr error) error {
	next := getRequestedNextURI(c)
	if next == "/" {
		next = uriAdmin
	}

	out := loginTpl{
		Title:           a.i18n.T("users.login"),
		PasswordEnabled: true,
		NextURI:         next,
	}

	// If there was an error in the previous state (POST reqest), set it to render in the template.
	if loginErr != nil {
		if e, ok := loginErr.(*echo.HTTPError); ok {
			out.Error = e.Message.(string)
		} else {
			out.Error = loginErr.Error()
		}
	}

	// Render the login page.
	return c.Render(http.StatusOK, "admin-login", out)
}

// renderLoginSetupPage renders the first time user setup page.
func (a *App) renderLoginSetupPage(c echo.Context, loginErr error) error {
	next := getRequestedNextURI(c)
	if next == "/" {
		next = uriAdmin
	}

	out := loginTpl{
		Title:           a.i18n.T("users.login"),
		PasswordEnabled: true,
		NextURI:         next,
	}

	// If there was an error in the previous state (POST reqest), set it to render in the template.
	if loginErr != nil {
		if e, ok := loginErr.(*echo.HTTPError); ok {
			out.Error = e.Message.(string)
		} else {
			out.Error = loginErr.Error()
		}
	}

	return c.Render(http.StatusOK, "admin-login-setup", out)
}

// doLogin logs a user in with a username and password.
func (a *App) doLogin(c echo.Context) error {
	var (
		startTime = time.Now()
		username  = strings.TrimSpace(c.FormValue("username"))
		password  = strings.TrimSpace(c.FormValue("password"))
	)

	// Ensure timing mitigation is applied regardless of early returns
	defer func() {
		if elapsed := time.Since(startTime).Milliseconds(); elapsed < 100 {
			time.Sleep(time.Duration(100-elapsed) * time.Millisecond)
		}
	}()

	if !strHasLen(username, 3, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "username"))
	}
	if !strHasLen(password, 8, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
	}

	// Log the user in by fetching and verifying credentials from the DB.
	user, err := a.auth.LoginUser(username, password)
	if err != nil {
		return err
	}

	// If TOTP is enabled for the user, create a temp token and redirect to the 2FA page.
	if user.TwofaType == models.TwofaTypeTOTP {
		// Generate a random token.
		token, err := generateRandomString(tmpAuthTokenLen)
		if err != nil {
			a.log.Printf("error generating 2FA token: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
		}

		// Set the token.
		tmptokens.Set(token, twofaTokenTTL, user.RecordID)

		// Redirect to 2FA page.
		next := utils.SanitizeURI(c.FormValue("next"))
		return c.Redirect(http.StatusFound, fmt.Sprintf("%s/login/twofa?token=%s&next=%s", uriAdmin, token, url.QueryEscape(next)))
	}

	return a.completeAuth(c, user, utils.SanitizeURI(c.FormValue("next")))
}

// doFirstTimeSetup sets a user up for the first time.
func (a *App) doFirstTimeSetup(c echo.Context) error {
	var (
		email     = strings.TrimSpace(c.FormValue("email"))
		username  = strings.TrimSpace(c.FormValue("username"))
		password  = strings.TrimSpace(c.FormValue("password"))
		password2 = strings.TrimSpace(c.FormValue("password2"))
	)
	a.log.Printf("first-time setup: starting for username=%q email=%q", username, email)
	if !utils.ValidateEmail(email) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "email"))
	}
	if !strHasLen(username, 3, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "username"))
	}
	if !strHasLen(password, 8, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
	}
	if password != password2 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.passwordMismatch"))
	}

	// Resolve or create the default "Super Admin" role in an idempotent way.
	r := auth.Role{
		Type: auth.RoleTypeUser,
		Name: null.NewString("Super Admin", true),
	}
	for p := range a.cfg.Permissions {
		r.Permissions = append(r.Permissions, p)
	}

	role, err := a.core.CreateRole(r)
	if err != nil {
		a.log.Printf("first-time setup: create role failed for username=%q email=%q: %v", username, email, err)
		return err
	}
	a.log.Printf("first-time setup: resolved super admin role id=%d for username=%q", role.ID, username)

	// Create the super admin user in the DB.
	u := auth.User{
		Type:          auth.UserTypeUser,
		HasPassword:   true,
		PasswordLogin: true,
		Username:      username,
		Name:          username,
		Password:      null.NewString(password, true),
		Email:         null.NewString(email, true),
		UserRoleID:    role.ID,
		Status:        auth.UserStatusEnabled,
	}
	authRec, err := a.auth.UpsertUserAuthRecord(u, "")
	if err != nil {
		a.log.Printf("first-time setup: create auth record failed for username=%q email=%q role_id=%d: %v", username, email, role.ID, err)
		return err
	}
	a.log.Printf("first-time setup: created auth record for username=%q", username)

	user, err := a.core.CreateUser(u)
	if err != nil {
		if authRec != nil {
			_ = a.pb.Delete(authRec)
		}
		a.log.Printf("first-time setup: create user failed for username=%q email=%q role_id=%d: %v", username, email, role.ID, err)
		return err
	}
	a.log.Printf("first-time setup: created user id=%d username=%q role_id=%d", user.ID, user.Username, role.ID)

	if _, err := refreshAuthCache(a.auth); err != nil {
		a.log.Printf("first-time setup: refresh auth cache failed for username=%q user_id=%d: %v", user.Username, user.ID, err)
		return err
	}
	a.log.Printf("first-time setup: refreshed auth cache for username=%q user_id=%d", user.Username, user.ID)

	authUser, err := a.auth.UpsertUserAuthRecord(user, u.Username)
	if err != nil {
		a.log.Printf("first-time setup: finalize auth record failed for username=%q user_id=%d: %v", user.Username, user.ID, err)
		return err
	}
	_ = authUser
	a.log.Printf("first-time setup: finalized auth record for username=%q user_id=%d", user.Username, user.ID)

	if err := a.completeAuth(c, user, utils.SanitizeURI(c.FormValue("next"))); err != nil {
		a.log.Printf("first-time setup: save session failed for username=%q user_id=%d: %v", user.Username, user.ID, err)
		return err
	}
	a.log.Printf("first-time setup: completed for username=%q user_id=%d", user.Username, user.ID)

	return nil
}

// renderResetPasswordPage renders the reset password page.
func (a *App) renderResetPasswordPage(c echo.Context, token, email, errMsg string) error {
	out := resetPasswordTpl{
		Title: a.i18n.T("users.resetPassword"),
		Token: token,
		Email: email,
		Error: errMsg,
	}
	return c.Render(http.StatusOK, "admin-reset-password", out)
}

// doForgotPassword handles the forgot password form submission.
func (a *App) doForgotPassword(c echo.Context) error {
	var (
		email = strings.ToLower(strings.TrimSpace(c.FormValue("email")))
	)

	// Validate email format.
	if !utils.ValidateEmail(email) {
		return c.Render(http.StatusOK, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("users.resetLinkSent")))
	}

	// Get the user by email.
	user, err := a.core.GetUser("", "", email)
	if err != nil {
		return c.Render(http.StatusOK, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("users.resetLinkSent")))
	}

	// If the password login is disabled, do not proceed, but show success message to prevent email enumeration.
	if !user.PasswordLogin {
		return c.Render(http.StatusOK, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("users.resetLinkSent")))
	}

	// Generate a random token.
	token, err := generateRandomString(tmpAuthTokenLen)
	if err != nil {
		a.log.Printf("error generating reset token: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}

	// Store the reset token in tmptokens.
	tmptokens.Set(email, passwordResetTTL, token)

	// Prepare the reset URL.
	resetURL := fmt.Sprintf("%s/admin/reset?token=%s&email=%s", a.urlCfg.RootURL, token, url.QueryEscape(email))

	// Prepare the email.
	var msg bytes.Buffer
	data := struct {
		ResetURL string
		L        *i18n.I18n
	}{
		ResetURL: resetURL,
		L:        a.i18n,
	}

	// Render the email template.
	if err := notifs.Tpls.ExecuteTemplate(&msg, notifs.TplForgotPassword, data); err != nil {
		a.log.Printf("error compiling notification template '%s': %v", notifs.TplForgotPassword, err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}

	subject, body := notifs.GetTplSubject(a.i18n.T("email.forgotPassword.subject"), msg.Bytes())

	// Send the email.
	if err := a.emailMsgr.Push(models.Message{
		From:    a.cfg.FromEmail,
		To:      []string{email},
		Subject: subject,
		Body:    body,
	}); err != nil {
		a.log.Printf("error sending reset email: %s", err)
	}

	// Show the success e-mail nonetheless to prevent e-mail enumeration.
	return c.Render(http.StatusOK, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("users.resetLinkSent")))
}

// doResetPassword handles the reset password form submission.
func (a *App) doResetPassword(c echo.Context, token, email string) error {
	var (
		password  = c.FormValue("password")
		password2 = c.FormValue("password2")
	)

	// Validate password.
	if !strHasLen(password, 8, stdInputMaxLen) {
		return a.renderResetPasswordPage(c, token, email, a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
	}
	if password != password2 {
		return a.renderResetPasswordPage(c, token, email, a.i18n.T("users.passwordMismatch"))
	}

	// Validate and consume the token (this deletes it).
	data, err := tmptokens.Get(email)
	if err != nil {
		return c.Render(http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("users.invalidResetLink")))
	}

	tk, ok := data.(string)
	if !ok || tk != token {
		return c.Render(http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("users.invalidResetLink")))
	}

	// Get the user.
	user, err := a.core.GetUser("", "", email)
	if err != nil {
		return c.Render(http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("users.invalidResetLink")))
	}

	// Password login is disabled for the user.
	if !user.PasswordLogin {
		return c.Render(http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("users.resetPassword"), "", a.i18n.T("public.invalidFeature")))
	}

	user.Password = null.NewString(password, true)
	if _, err := a.core.UpdateUserProfile(user.RecordID, user); err != nil {
		a.log.Printf("error updating user password: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}

	return a.completeAuth(c, user, uriAdmin)
}

// renderTwofaPage renders the 2FA verification page.
func (a *App) renderTwofaPage(c echo.Context, token, next, errMsg string) error {
	out := twofaTpl{
		Title:       a.i18n.T("users.twoFA"),
		Description: "",
		Token:       token,
		NextURI:     next,
		Error:       errMsg,
	}
	return c.Render(http.StatusOK, "admin-twofa", out)
}

// doTwofaVerify handles the 2FA verification form submission.
func (a *App) doTwofaVerify(c echo.Context, token string, userRecordID string, next string) error {
	totpCode := strings.TrimSpace(c.FormValue("totp_code"))

	// Validate.
	if !strHasLen(totpCode, 6, 6) {
		return a.renderTwofaPage(c, token, next, a.i18n.T("globals.messages.invalidValue"))
	}

	// Get the user.
	user, err := a.core.GetUser(userRecordID, "", "")
	if err != nil {
		return a.renderTwofaPage(c, token, next, a.i18n.T("users.invalidRequest"))
	}

	// Verify that TOTP is actually enabled for the user.
	if user.TwofaType != models.TwofaTypeTOTP {
		return a.renderTwofaPage(c, token, next, a.i18n.T("users.twoFANotEnabled"))
	}

	// Verify the TOTP code.
	valid := totp.Validate(totpCode, user.TwofaKey.String)
	if !valid {
		return a.renderTwofaPage(c, token, next, a.i18n.T("globals.messages.invalidValue"))
	}

	// Invalidate the token.
	tmptokens.Delete(token)

	return a.completeAuth(c, user, next)
}

func (a *App) completeAuth(c echo.Context, user auth.User, next string) error {
	next = adminRedirectPath(next)

	a.log.Printf("auth bridge: issuing PocketBase auth for username=%q user_id=%d next=%q", user.Username, user.ID, next)
	clientAuth, err := a.auth.IssueClientAuth(user)
	if err != nil {
		a.log.Printf("auth bridge: failed issuing PocketBase auth for username=%q user_id=%d: %v", user.Username, user.ID, err)
		return err
	}
	a.log.Printf("auth bridge: issued PocketBase auth for username=%q user_id=%d token_len=%d", user.Username, user.ID, len(clientAuth.Token))

	user.Password = null.String{}
	clientAuth.Record["profile"] = user

	payloadJSON, err := json.Marshal(map[string]any{
		"token":  clientAuth.Token,
		"record": clientAuth.Record,
	})
	if err != nil {
		a.log.Printf("auth bridge: failed marshaling auth payload for username=%q user_id=%d: %v", user.Username, user.ID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}
	a.log.Printf("auth bridge: rendering bridge page for username=%q user_id=%d", user.Username, user.ID)

	return c.Render(http.StatusOK, "admin-auth-bridge", authBridgeTpl{
		Title:       a.i18n.T("users.login"),
		Description: "",
		NextURI:     next,
		PayloadJSON: template.JS(string(payloadJSON)),
	})
}

// GenerateTOTPQR generates a TOTP QR code for a user to scan with their authenticator app.
func (a *App) GenerateTOTPQR(c echo.Context) error {
	u := c.Get(auth.UserHTTPCtxKey).(auth.User)

	// If TOTP is already enabled, don't generate a new key.
	if u.TwofaType == models.TwofaTypeTOTP {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.twoFAAlreadyEnabled"))
	}

	// Generate a new TOTP key.
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      a.cfg.SiteName,
		AccountName: u.Email.String,
	})
	if err != nil {
		a.log.Printf("error generating TOTP key: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}

	// Convert the TOTP key to a QR code image.
	img, err := key.Image(200, 200)
	if err != nil {
		a.log.Printf("error generating QR code: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}

	// Encode the QR code as a PNG and return it as base64.
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		a.log.Printf("error encoding QR code: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}

	return c.JSON(http.StatusOK, okResp{struct {
		Secret string `json:"secret"`
		QR     string `json:"qr"`
	}{
		Secret: key.Secret(),
		QR:     base64.StdEncoding.EncodeToString(buf.Bytes()),
	}})
}
