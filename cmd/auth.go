package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
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

	NextURI     string
	Error       string
}

type loginReq struct {
	Username string `json:"username" form:"username"`
	Password string `json:"password" form:"password"`
	Next     string `json:"next" form:"next"`
}

type twofaVerifyReq struct {
	Token    string `json:"token" form:"token"`
	TOTPCode string `json:"totp_code" form:"totp_code"`
	Next     string `json:"next" form:"next"`
}

type forgotPasswordReq struct {
	Email string `json:"email" form:"email"`
}

type resetPasswordReq struct {
	Token     string `json:"token" form:"token"`
	Email     string `json:"email" form:"email"`
	Password  string `json:"password" form:"password"`
	Password2 string `json:"password2" form:"password2"`
}

type clientAuthResp struct {
	Status string          `json:"status"`
	Next   string          `json:"next,omitempty"`
	Token  string          `json:"token,omitempty"`
	Record map[string]any  `json:"record,omitempty"`
}

type twofaChallengeResp struct {
	Status string `json:"status"`
	Token  string `json:"token"`
	Next   string `json:"next"`
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

func (a *App) isSetupRequired() bool {
	a.Lock()
	defer a.Unlock()
	return a.needsUserSetup
}

// LoginSetupPage renders the first time user login page and handles the login form.
func (a *App) LoginSetupPage(c echo.Context) error {
	if !a.isSetupRequired() {
		return c.Redirect(http.StatusFound, path.Join(uriAdmin, "/login"))
	}

	// Process POST login request.
	var loginErr error
	if c.Request().Method == http.MethodPost {
		loginErr = a.doFirstTimeSetup(c)
		if loginErr == nil {
			a.Lock()
			a.needsUserSetup = false
			a.Unlock()
			return c.Redirect(http.StatusFound, path.Join(uriAdmin, "/login"))
		}
	}

	// Render the page, with or without POST.
	return a.renderLoginSetupPage(c, loginErr)
}

// Logout logs a user out.
func (a *App) Logout(c echo.Context) error {
	// API auth is token-based via PocketBase. Logout is handled by clearing
	// the token on the client and does not depend on server-side sessions.
	return c.JSON(http.StatusOK, okResp{true})
}

// renderLoginSetupPage renders the first time user setup page.
func (a *App) renderLoginSetupPage(c echo.Context, loginErr error) error {
	next := getRequestedNextURI(c)
	if next == "/" {
		next = uriAdmin
	}

	out := loginTpl{
		Title:   a.i18n.T("users.login"),
		NextURI: next,
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

// AuthLogin authenticates a user and returns a JSON response for the Vue app.
func (a *App) AuthLogin(c echo.Context) error {
	var req loginReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidJSON"))
	}

	var (
		startTime = time.Now()
		username  = strings.TrimSpace(req.Username)
		password  = strings.TrimSpace(req.Password)
		next      = utils.SanitizeURI(req.Next)
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

		return c.JSON(http.StatusOK, okResp{twofaChallengeResp{
			Status: "twofa_required",
			Token:  token,
			Next:   adminRedirectPath(next),
		}})
	}

	return a.writeClientAuth(c, user, next)
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

	return nil
}

// AuthForgotPassword starts the reset password flow.
func (a *App) AuthForgotPassword(c echo.Context) error {
	var req forgotPasswordReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidJSON"))
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	success := okResp{map[string]string{"status": "ok", "message": a.i18n.T("users.resetLinkSent")}}

	// Validate email format.
	if !utils.ValidateEmail(email) {
		return c.JSON(http.StatusOK, success)
	}

	// Get the user by email.
	user, err := a.core.GetUser("", "", email)
	if err != nil {
		return c.JSON(http.StatusOK, success)
	}

	// If the password login is disabled, do not proceed, but show success message to prevent email enumeration.
	if !user.PasswordLogin {
		return c.JSON(http.StatusOK, success)
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
	return c.JSON(http.StatusOK, success)
}

// AuthResetPassword validates a reset token and signs the user in with the new password.
func (a *App) AuthResetPassword(c echo.Context) error {
	var req resetPasswordReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidJSON"))
	}

	token := strings.TrimSpace(req.Token)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := req.Password
	password2 := req.Password2

	// Validate password.
	if !strHasLen(password, 8, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
	}
	if password != password2 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.passwordMismatch"))
	}

	// Validate and consume the token (this deletes it).
	data, err := tmptokens.Get(email)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidResetLink"))
	}

	tk, ok := data.(string)
	if !ok || tk != token {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidResetLink"))
	}

	// Get the user.
	user, err := a.core.GetUser("", "", email)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidResetLink"))
	}

	// Password login is disabled for the user.
	if !user.PasswordLogin {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("public.invalidFeature"))
	}

	user.Password = null.NewString(password, true)
	if _, err := a.core.UpdateUserProfile(user.RecordID, user); err != nil {
		a.log.Printf("error updating user password: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}

	return a.writeClientAuth(c, user, uriAdmin)
}

// AuthVerifyTwoFA completes a TOTP challenge and returns PocketBase auth payload.
func (a *App) AuthVerifyTwoFA(c echo.Context) error {
	var req twofaVerifyReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidJSON"))
	}

	token := strings.TrimSpace(req.Token)
	next := utils.SanitizeURI(req.Next)
	totpCode := strings.TrimSpace(req.TOTPCode)

	// Validate.
	if !strHasLen(totpCode, 6, 6) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidValue"))
	}

	data, err := tmptokens.Check(token)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidRequest"))
	}

	userRecordID, ok := data.(string)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidRequest"))
	}

	// Get the user.
	user, err := a.core.GetUser(userRecordID, "", "")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidRequest"))
	}

	// Verify that TOTP is actually enabled for the user.
	if user.TwofaType != models.TwofaTypeTOTP {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.twoFANotEnabled"))
	}

	// Verify the TOTP code.
	valid := totp.Validate(totpCode, user.TwofaKey.String)
	if !valid {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidValue"))
	}

	// Invalidate the token.
	tmptokens.Delete(token)

	return a.writeClientAuth(c, user, next)
}

func (a *App) writeClientAuth(c echo.Context, user auth.User, next string) error {
	next = adminRedirectPath(next)

	clientAuth, err := a.auth.IssueClientAuth(user)
	if err != nil {
		return err
	}

	user.Password = null.String{}
	clientAuth.Record["profile"] = user

	return c.JSON(http.StatusOK, okResp{clientAuthResp{
		Status: "authenticated",
		Next:   next,
		Token:  clientAuth.Token,
		Record: clientAuth.Record,
	}})
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
