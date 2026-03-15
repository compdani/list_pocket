package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/zerodha/simplesessions/v3"
	"golang.org/x/oauth2"
)

type OIDCclaim struct {
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	Sub               string `json:"sub"`
	Picture           string `json:"picture"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

type OIDCConfig struct {
	Enabled           bool   `json:"enabled"`
	ProviderURL       string `json:"provider_url"`
	RedirectURL       string `json:"redirect_url"`
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	AutoCreateUsers   bool   `json:"auto_create_users"`
	DefaultUserRoleID int    `json:"default_user_role_id"`
	DefaultListRoleID int    `json:"default_list_role_id"`
}

type BasicAuthConfig struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Config struct {
	OIDC      OIDCConfig
	BasicAuth BasicAuthConfig

	AuthCollection string
}

// Callbacks takes two callback functions required by simplesessions.
type Callbacks struct {
	SetCookie         func(cookie *http.Cookie, w any) error
	GetCookie         func(name string, r any) (*http.Cookie, error)
	GetUser           func(id int) (User, error)
	GetUsers          func() ([]User, error)
	GetUserByUsername func(username string) (User, error)
}

type Auth struct {
	apiUsers map[string]User
	sync.RWMutex

	cfg       Config
	pb        *pocketbase.PocketBase
	authCol   string
	oauthCfg  oauth2.Config
	verifier  *oidc.IDTokenVerifier
	provider  *oidc.Provider
	sess      *simplesessions.Manager
	sessStore *memoryStore
	cb        *Callbacks
	log       *log.Logger
}

var regexBcryptHash = regexp.MustCompile(`^\$2[abxy]\$`)

// New returns an initialize Auth instance.
func New(cfg Config, db *sql.DB, pb *pocketbase.PocketBase, cb *Callbacks, lo *log.Logger) (*Auth, error) {
	authCollection := cfg.AuthCollection
	if authCollection == "" {
		authCollection = "users"
	}

	a := &Auth{
		cfg:     cfg,
		pb:      pb,
		authCol: authCollection,
		cb:      cb,
		log:     lo,

		apiUsers: map[string]User{},
	}

	// Initialize session manager.
	a.sess = simplesessions.New(simplesessions.Options{
		EnableAutoCreate: false,
		SessionIDLength:  64,
		Cookie: simplesessions.CookieOptions{
			IsHTTPOnly: true,
			MaxAge:     time.Hour * 24 * 7,
		},
	})
	st := newMemoryStore()
	a.sessStore = st
	a.sess.UseStore(st)
	a.sess.SetCookieHooks(cb.GetCookie, cb.SetCookie)

	if err := a.ensureAuthCollection(); err != nil {
		return nil, err
	}

	return a, nil
}

// CacheAPIUsers caches API users for authenticating requests. It wipes
// the existing cache every time and is meant for syncing all API users
// in the database in one shot.
func (o *Auth) CacheAPIUsers(users []User) {
	o.Lock()
	defer o.Unlock()

	o.apiUsers = map[string]User{}
	for _, u := range users {
		o.apiUsers[u.Username] = u
	}
}

func (o *Auth) ensureAuthCollection() error {
	if o.pb == nil {
		return fmt.Errorf("pocketbase instance is nil")
	}

	col, err := o.pb.FindCollectionByNameOrId(o.authCol)
	if err != nil {
		return err
	}

	changed := false

	if col.Fields.GetByName("username") == nil {
		col.Fields.Add(&pbcore.TextField{Name: "username", Required: true, Min: 3, Max: 255})
		changed = true
	}

	if col.Fields.GetByName("user_type") == nil {
		col.Fields.Add(&pbcore.TextField{Name: "user_type", Required: true})
		changed = true
	}

	if col.Fields.GetByName("status") == nil {
		col.Fields.Add(&pbcore.TextField{Name: "status", Required: true})
		changed = true
	}

	if col.Fields.GetByName("role") == nil {
		col.Fields.Add(&pbcore.TextField{Name: "role"})
		changed = true
	}

	if changed {
		return o.pb.Save(col)
	}

	return nil
}

func (o *Auth) SyncUsers(users []User) error {
	for _, u := range users {
		if err := o.SyncUser(u); err != nil {
			return err
		}
	}
	return nil
}

func (o *Auth) SyncUser(u User) error {
	rec, err := o.findAuthRecordByUsername(u.Username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if rec == nil {
		col, err := o.pb.FindCollectionByNameOrId(o.authCol)
		if err != nil {
			return err
		}
		rec = pbcore.NewRecord(col)
	}

	email := strings.TrimSpace(u.Email.String)
	if email == "" {
		email = fmt.Sprintf("%s@api.local", strings.ToLower(u.Username))
	}

	rec.SetEmail(email)
	rec.Set("username", u.Username)
	rec.Set("user_type", u.Type)
	rec.Set("status", u.Status)
	rec.Set("role", strconv.Itoa(u.UserRoleID))
	rec.SetVerified(true)

	if u.Password.String != "" {
		if regexBcryptHash.MatchString(u.Password.String) {
			rec.SetRaw(pbcore.FieldNamePassword, u.Password.String)
		} else {
			rec.Set(pbcore.FieldNamePassword, u.Password.String)
			rec.Set("passwordConfirm", u.Password.String)
		}
	} else if rec.Id == "" {
		placeholder := fmt.Sprintf("lm-disabled-%d-%d", u.ID, time.Now().UnixNano())
		rec.Set(pbcore.FieldNamePassword, placeholder)
		rec.Set("passwordConfirm", placeholder)
	}

	return o.pb.Save(rec)
}

func (o *Auth) DeleteUsers(ids []int) error {
	for _, id := range ids {
		rec, err := o.findAuthRecordByUserID(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return err
		}

		if err := o.pb.Delete(rec); err != nil {
			return err
		}
	}
	return nil
}

func (o *Auth) findAuthRecordByUserID(userID int) (*pbcore.Record, error) {
	if userID < 1 {
		return nil, sql.ErrNoRows
	}

	return o.pb.FindRecordById(o.authCol, strconv.Itoa(userID))
}

func (o *Auth) findAuthRecordByUsername(username string) (*pbcore.Record, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, sql.ErrNoRows
	}

	return o.pb.FindFirstRecordByData(o.authCol, "username", username)
}

func (o *Auth) LoginUser(username, password string) (User, error) {
	if o.cb == nil || o.cb.GetUserByUsername == nil {
		return User{}, echo.NewHTTPError(http.StatusInternalServerError, "user lookup callback missing")
	}

	user, err := o.cb.GetUserByUsername(username)
	if err != nil {
		return User{}, echo.NewHTTPError(http.StatusForbidden, "invalid credentials")
	}

	if user.Type != UserTypeUser || user.Status != UserStatusEnabled || !user.PasswordLogin {
		return User{}, echo.NewHTTPError(http.StatusForbidden, "invalid credentials")
	}

	if err := o.SyncUser(user); err != nil {
		return User{}, echo.NewHTTPError(http.StatusInternalServerError, "failed syncing auth user")
	}

	rec, err := o.findAuthRecordByUsername(user.Username)
	if err != nil || rec == nil || !rec.ValidatePassword(password) {
		return User{}, echo.NewHTTPError(http.StatusForbidden, "invalid credentials")
	}

	return user, nil
}

func (o *Auth) ValidateUserPassword(username, password string) bool {
	u, err := o.LoginUser(username, password)
	return err == nil && u.ID > 0
}

// CacheAPIUser caches an API user for authenticating requests.
func (o *Auth) CacheAPIUser(u User) {
	o.Lock()
	o.apiUsers[u.Username] = u
	o.Unlock()
}

// GetAPIToken validates an API user+token.
func (o *Auth) GetAPIToken(user string, token string) (User, bool) {
	o.RLock()
	t, ok := o.apiUsers[user]
	o.RUnlock()

	if !ok || subtle.ConstantTimeCompare([]byte(t.Password.String), []byte(token)) != 1 {
		return User{}, false
	}

	return t, true
}

func (o *Auth) ValidateAPIToken(user string, token string) (User, bool) {
	// Legacy in-memory cache fallback.
	if out, ok := o.GetAPIToken(user, token); ok {
		return out, true
	}

	if o.cb == nil || o.cb.GetUserByUsername == nil {
		return User{}, false
	}

	u, err := o.cb.GetUserByUsername(user)
	if err != nil || u.Status != UserStatusEnabled || u.Type != UserTypeAPI {
		return User{}, false
	}

	if err := o.SyncUser(u); err != nil {
		return User{}, false
	}

	rec, err := o.findAuthRecordByUsername(u.Username)
	if err != nil || rec == nil || !rec.ValidatePassword(token) {
		return User{}, false
	}

	return u, true
}

// LoadCachedUsersFromPocketBase refreshes API token cache and user presence
// info from the auth collection, without querying legacy SQL tables.
func (o *Auth) LoadCachedUsersFromPocketBase() (bool, error) {
	if o.pb == nil {
		return false, fmt.Errorf("pocketbase instance is nil")
	}

	recs, err := o.pb.FindRecordsByFilter(o.authCol, "", "", 0, 0)
	if err != nil {
		return false, err
	}

	hasUser := false
	apiUsers := make([]User, 0, len(recs))
	for _, rec := range recs {
		u := User{
			Username: rec.GetString("username"),
			Type:     rec.GetString("user_type"),
			Status:   rec.GetString("status"),
		}

		if o.cb != nil && o.cb.GetUserByUsername != nil {
			if dbUser, err := o.cb.GetUserByUsername(u.Username); err == nil {
				u.ID = dbUser.ID
			}
		}

		if u.Type == UserTypeUser && u.Status == UserStatusEnabled {
			hasUser = true
		}
		if u.Type == UserTypeAPI && u.Status == UserStatusEnabled {
			apiUsers = append(apiUsers, u)
		}
	}

	o.CacheAPIUsers(apiUsers)
	return hasUser, nil
}

// initOIDC initializes the OIDC provider, verifier, and OAuth config.
func (o *Auth) initOIDC() error {
	if !o.cfg.OIDC.Enabled {
		return fmt.Errorf("OIDC is not enabled")
	}

	provider, err := oidc.NewProvider(context.Background(), o.cfg.OIDC.ProviderURL)
	if err != nil {
		return fmt.Errorf("error initializing OIDC OAuth provider: %v", err)
	}

	o.verifier = provider.Verifier(&oidc.Config{
		ClientID: o.cfg.OIDC.ClientID,
	})

	o.oauthCfg = oauth2.Config{
		ClientID:     o.cfg.OIDC.ClientID,
		ClientSecret: o.cfg.OIDC.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  o.cfg.OIDC.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	o.provider = provider

	return nil
}

// getProvider returns the OIDC provider, initializing it if necessary.
func (o *Auth) getProvider() (*oidc.Provider, error) {
	o.Lock()
	defer o.Unlock()

	if o.provider == nil {
		if err := o.initOIDC(); err != nil {
			return nil, err
		}
	}
	return o.provider, nil
}

// getVerifier returns the OIDC verifier, initializing it if necessary.
func (o *Auth) getVerifier() (*oidc.IDTokenVerifier, error) {
	o.Lock()
	defer o.Unlock()

	if o.verifier == nil {
		if err := o.initOIDC(); err != nil {
			return nil, err
		}
	}
	return o.verifier, nil
}

// getOAuthConfig returns the OAuth config, initializing it if necessary.
func (o *Auth) getOAuthConfig() (*oauth2.Config, error) {
	o.Lock()
	defer o.Unlock()

	if o.oauthCfg.ClientID == "" {
		if err := o.initOIDC(); err != nil {
			return nil, err
		}
	}
	return &o.oauthCfg, nil
}

// GetOIDCAuthURL returns the OIDC provider's auth URL to redirect to.
func (o *Auth) GetOIDCAuthURL(state, nonce string) string {
	cfg, err := o.getOAuthConfig()
	if err != nil {
		o.log.Printf("error getting OAuth config: %v", err)
		return ""
	}
	return cfg.AuthCodeURL(state, oidc.Nonce(nonce))
}

// ExchangeOIDCToken takes an OIDC authorization code (recieved via redirect from the OIDC provider),
// validates it, and returns an OIDC token for subsequent auth.
func (o *Auth) ExchangeOIDCToken(code, nonce string) (string, OIDCclaim, error) {
	cfg, err := o.getOAuthConfig()
	if err != nil {
		return "", OIDCclaim{}, echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("error getting OAuth config: %v", err))
	}

	tk, err := cfg.Exchange(context.TODO(), code)
	if err != nil {
		return "", OIDCclaim{}, echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("error exchanging token: %v", err))
	}

	rawIDTk, ok := tk.Extra("id_token").(string)
	if !ok {
		return "", OIDCclaim{}, echo.NewHTTPError(http.StatusUnauthorized, "`id_token` missing.")
	}

	verifier, err := o.getVerifier()
	if err != nil {
		return "", OIDCclaim{}, echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("error getting verifier: %v", err))
	}

	idTk, err := verifier.Verify(context.TODO(), rawIDTk)
	if err != nil {
		return "", OIDCclaim{}, echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("error verifying ID token: %v", err))
	}

	if idTk.Nonce != nonce {
		return "", OIDCclaim{}, echo.NewHTTPError(http.StatusUnauthorized, "nonce did not match")
	}

	var claims OIDCclaim
	if err := idTk.Claims(&claims); err != nil {
		return "", OIDCclaim{}, errors.New("error getting user from OIDC")
	}

	// If claims doesn't have the e-mail, attempt to fetch it from the userinfo endpoint.
	if claims.Email == "" {
		provider, err := o.getProvider()
		if err != nil {
			return "", OIDCclaim{}, fmt.Errorf("error getting provider: %v", err)
		}

		userInfo, err := provider.UserInfo(context.TODO(), oauth2.StaticTokenSource(tk))
		if err != nil {
			return "", OIDCclaim{}, errors.New("error fetching user info from OIDC")
		}

		// Parse the UserInfo claims into the claims struct
		if err := userInfo.Claims(&claims); err != nil {
			return "", OIDCclaim{}, errors.New("error parsing user info claims")
		}
	}

	return rawIDTk, claims, nil
}

// Middleware is the HTTP middleware used for wrapping HTTP handlers registered on the echo router.
// It authorizes token (BasicAuth/token) based and cookie based sessions and on successful auth,
// sets the authenticated User{} on the echo context on the key UserKey. On failure, it sets an Error{}
// instead on the same key.
func (o *Auth) Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// It's an `Authorization` header request.
		hdr := strings.TrimSpace(c.Request().Header.Get("Authorization"))

		// If cookie is set, ignore BasicAuth. This is to preserve backwards compatibility
		// in v3 -> v4 upgrade where the user browser sessions would still have old
		// BasicAuth credentials, which no longer work in the new system which expects
		// session cookies instead, which causes a redirect loop despite loggin in and session
		// cookies being set.
		//
		// TODO: This should be removed in a future version.
		if c := strings.TrimSpace(c.Request().Header.Get("Cookie")); strings.Contains(c, "session=") {
			hdr = ""
		}

		if len(hdr) > 0 {
			// Primary auth path: PocketBase auth token.
			if strings.HasPrefix(hdr, "Bearer ") {
				token := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer "))
				user, rec, err := o.validatePBToken(token)
				if err != nil {
					c.Set(UserHTTPCtxKey, echo.NewHTTPError(http.StatusForbidden, "invalid auth token"))
					return next(c)
				}
				c.Set(UserHTTPCtxKey, user)
				c.Set(AuthRecordHTTPCtxKey, rec)
				return next(c)
			}

			// Backward-compatibility for legacy api_key:token auth header.
			key, token, err := parseAuthHeader(hdr)
			if err != nil {
				c.Set(UserHTTPCtxKey, echo.NewHTTPError(http.StatusForbidden, err.Error()))
				return next(c)
			}

			// Validate the token.
			user, ok := o.ValidateAPIToken(key, token)
			if !ok {
				c.Set(UserHTTPCtxKey, echo.NewHTTPError(http.StatusForbidden, "invalid API credentials"))
				return next(c)
			}

			// Set the user details on the handler context.
			c.Set(UserHTTPCtxKey, user)
			return next(c)
		}

		// Is it a cookie based session?
		sess, user, err := o.validateSession(c)
		if err != nil {
			c.Set(UserHTTPCtxKey, echo.NewHTTPError(http.StatusForbidden, "invalid session"))
			return next(c)
		}

		// Set the user details on the handler context.
		c.Set(UserHTTPCtxKey, user)
		if rec, err := o.findAuthRecordByUserID(user.ID); err == nil && rec != nil {
			c.Set(AuthRecordHTTPCtxKey, rec)
		}
		c.Set(SessionKey, sess)
		return next(c)
	}
}

// APIMiddleware is a token-only HTTP middleware for API handlers.
// It validates Authorization Bearer tokens (or access_token query param fallback)
// and sets authenticated user context. It does not use cookie sessions.
func (o *Auth) APIMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		hdr := strings.TrimSpace(c.Request().Header.Get("Authorization"))
		token := ""
		if strings.HasPrefix(hdr, "Bearer ") {
			token = strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer "))
		}
		if token == "" {
			token = strings.TrimSpace(c.QueryParam("access_token"))
		}
		if token == "" {
			c.Set(UserHTTPCtxKey, echo.NewHTTPError(http.StatusForbidden, "missing auth token"))
			return next(c)
		}

		user, rec, err := o.validatePBToken(token)
		if err != nil {
			c.Set(UserHTTPCtxKey, echo.NewHTTPError(http.StatusForbidden, "invalid auth token"))
			return next(c)
		}

		c.Set(UserHTTPCtxKey, user)
		c.Set(AuthRecordHTTPCtxKey, rec)
		return next(c)
	}
}

func (o *Auth) validatePBToken(token string) (User, *pbcore.Record, error) {
	if token == "" {
		return User{}, nil, echo.NewHTTPError(http.StatusForbidden, "empty token")
	}

	rec, err := o.pb.FindAuthRecordByToken(token, pbcore.TokenTypeAuth)
	if err != nil || rec == nil {
		return User{}, nil, echo.NewHTTPError(http.StatusForbidden, "invalid token")
	}

	username := strings.TrimSpace(rec.GetString("username"))
	if username == "" {
		return User{}, nil, echo.NewHTTPError(http.StatusForbidden, "invalid token user")
	}

	user := User{
		Username:       username,
		Type:           strings.TrimSpace(rec.GetString("user_type")),
		Status:         strings.TrimSpace(rec.GetString("status")),
		PermissionsMap: map[string]struct{}{},
	}

	if user.Type == "" {
		user.Type = UserTypeUser
	}
	if user.Status == "" {
		user.Status = UserStatusEnabled
	}

	if roleID := ExtractRoleIDFromRecord(rec); roleID > 0 {
		user.UserRoleID = roleID
		user.UserRole.ID = roleID
	}

	if perms, err := o.loadRolePermissions(rec); err == nil {
		user.PermissionsMap = perms
		user.UserRole.Permissions = make([]string, 0, len(perms))
		for perm := range perms {
			user.UserRole.Permissions = append(user.UserRole.Permissions, perm)
		}
	}

	if o.cb != nil && o.cb.GetUserByUsername != nil {
		if dbUser, err := o.cb.GetUserByUsername(username); err == nil {
			if dbUser.Username == "" {
				dbUser.Username = username
			}
			if dbUser.Type == "" {
				dbUser.Type = user.Type
			}
			if dbUser.Status == "" {
				dbUser.Status = user.Status
			}
			if user.UserRoleID > 0 {
				dbUser.UserRoleID = user.UserRoleID
				dbUser.UserRole.ID = user.UserRoleID
			}
			if len(user.PermissionsMap) > 0 {
				dbUser.PermissionsMap = user.PermissionsMap
				dbUser.UserRole.Permissions = user.UserRole.Permissions
			}
			user = dbUser
		}
	}

	if user.Status != UserStatusEnabled {
		return User{}, nil, echo.NewHTTPError(http.StatusForbidden, "disabled user")
	}

	return user, rec, nil
}

func (o *Auth) loadRolePermissions(rec *pbcore.Record) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if rec == nil || o.pb == nil {
		return out, nil
	}

	roleID := ExtractRoleIDFromRecord(rec)
	if roleID < 1 {
		return out, nil
	}

	roleRec, err := o.pb.FindFirstRecordByFilter("roles", "legacy_id={:id}", dbx.Params{"id": roleID})
	if err != nil || roleRec == nil {
		return out, err
	}

	for _, perm := range normalizeStringArray(roleRec.Get("permissions")) {
		perm = strings.TrimSpace(perm)
		if perm == "" {
			continue
		}
		out[perm] = struct{}{}
	}

	return out, nil
}

func normalizeStringArray(raw any) []string {
	switch value := raw.(type) {
	case []string:
		return value
	case []any:
		out := make([]string, 0, len(value))
		for _, entry := range value {
			if text, ok := entry.(string); ok {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}

		var out []string
		if err := json.Unmarshal([]byte(value), &out); err == nil {
			return out
		}

		return []string{value}
	default:
		return nil
	}
}

// Perm is an HTTP handler middleware that checks if the authenticated user has the required permissions.
func (o *Auth) Perm(next echo.HandlerFunc, perms ...string) echo.HandlerFunc {
	return func(c echo.Context) error {
		u, ok := c.Get(UserHTTPCtxKey).(User)
		if !ok {
			c.Set(UserHTTPCtxKey, echo.NewHTTPError(http.StatusForbidden, "invalid session"))
			return next(c)
		}

		// If the current user is a Super Admin user, do no checks.
		if ExtractRoleID(c) == SuperAdminRoleID || u.UserRole.ID == SuperAdminRoleID {
			return next(c)
		}

		// Check if the current handler's permission is in the user's permission map.
		var (
			has  = false
			perm = ""
		)
		for _, perm = range perms {
			if _, ok := u.PermissionsMap[perm]; ok {
				has = true
				break
			}
		}

		if !has {
			return echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf("permission denied: %s", perm))
		}

		return next(c)
	}
}

// ExtractRoleID returns the role ID from the auth record in the request context.
// Falls back to the hydrated auth.User profile when unavailable.
func ExtractRoleID(c echo.Context) int {
	if rec, ok := c.Get(AuthRecordHTTPCtxKey).(*pbcore.Record); ok && rec != nil {
		if id := ExtractRoleIDFromRecord(rec); id > 0 {
			return id
		}
	}

	if u, ok := c.Get(UserHTTPCtxKey).(User); ok {
		if u.UserRole.ID > 0 {
			return u.UserRole.ID
		}
		if u.UserRoleID > 0 {
			return u.UserRoleID
		}
	}

	return 0
}

func ExtractRoleIDFromRecord(rec *pbcore.Record) int {
	if rec == nil {
		return 0
	}

	if id := rec.GetInt("role"); id > 0 {
		return id
	}

	if raw := strings.TrimSpace(rec.GetString("role")); raw != "" {
		if id, err := strconv.Atoi(raw); err == nil && id > 0 {
			return id
		}
	}

	return 0
}

// SaveSession creates and sets a session (post successful login/auth).
func (o *Auth) SaveSession(u User, oidcToken string, c echo.Context) error {
	sess, err := o.sess.NewSession(c, c)
	if err != nil {
		o.log.Printf("error creating login session: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "error creating session")
	}

	if err := o.SyncUser(u); err != nil {
		o.log.Printf("error syncing auth user: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "error creating session")
	}

	rec, err := o.findAuthRecordByUsername(u.Username)
	if err != nil {
		o.log.Printf("error fetching auth user record: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "error creating session")
	}

	token, err := rec.NewAuthToken()
	if err != nil {
		o.log.Printf("error generating auth token: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "error creating session")
	}

	if err := sess.SetMulti(map[string]any{"user_id": u.ID, "username": u.Username, "oidc_token": oidcToken, "auth_token": token}); err != nil {
		o.log.Printf("error setting login session: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "error creating session")
	}

	return nil
}

// validateSession checks if the cookie session is valid (in the DB) and returns the session and user details.
func (o *Auth) validateSession(c echo.Context) (*simplesessions.Session, User, error) {
	// Cookie session.
	sess, err := o.sess.Acquire(context.TODO(), c, c)
	if err != nil {
		return nil, User{}, echo.NewHTTPError(http.StatusForbidden, err.Error())
	}

	// Get the session variables.
	vars, err := sess.GetMulti("user_id", "username", "oidc_token", "auth_token")
	if err != nil {
		return nil, User{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Validate the user ID in the session.
	userID, err := o.sessStore.Int(vars["user_id"], nil)
	if err != nil || userID < 1 {
		o.log.Printf("error fetching session user ID: %v", err)
		return nil, User{}, echo.NewHTTPError(http.StatusInternalServerError, "invalid session.")
	}

	authToken, ok := vars["auth_token"].(string)
	if !ok || authToken == "" {
		return nil, User{}, echo.NewHTTPError(http.StatusForbidden, "invalid session")
	}

	rec, err := o.pb.FindAuthRecordByToken(authToken, pbcore.TokenTypeAuth)
	if err != nil || rec == nil {
		return nil, User{}, echo.NewHTTPError(http.StatusForbidden, "invalid session")
	}

	username := strings.TrimSpace(rec.GetString("username"))
	if username == "" {
		return nil, User{}, echo.NewHTTPError(http.StatusForbidden, "invalid session")
	}

	if sessionUsername, ok := vars["username"].(string); ok && strings.TrimSpace(sessionUsername) != "" {
		if strings.TrimSpace(sessionUsername) != username {
			return nil, User{}, echo.NewHTTPError(http.StatusForbidden, "invalid session")
		}
	}

	if rec.GetString("status") != UserStatusEnabled {
		return nil, User{}, echo.NewHTTPError(http.StatusForbidden, "invalid session")
	}

	// Fetch user details from the database.
	user, err := o.cb.GetUserByUsername(username)
	if err != nil {
		o.log.Printf("error fetching session user: %v", err)
	}

	return sess, user, err
}

// GetUser retrieves and returns the User object from an authenticated
// HTTP handler request.
func GetUser(c echo.Context) User {
	return c.Get(UserHTTPCtxKey).(User)
}

// parseAuthHeader parses the Authorization header and returns the api_key and access_token.
func parseAuthHeader(h string) (string, string, error) {
	const authBasic = "Basic"
	const authToken = "token"

	var (
		pair  []string
		delim = ":"
	)

	if strings.HasPrefix(h, authToken) {
		// token api_key:access_token.
		pair = strings.SplitN(strings.Trim(h[len(authToken):], " "), delim, 2)
	} else if strings.HasPrefix(h, authBasic) {
		// HTTP BasicAuth. This is supported for backwards compatibility.
		payload, err := base64.StdEncoding.DecodeString(string(strings.Trim(h[len(authBasic):], " ")))
		if err != nil {
			return "", "", echo.NewHTTPError(http.StatusBadRequest, "invalid Base64 value in Basic Authorization header")
		}
		pair = strings.SplitN(string(payload), delim, 2)
	} else {
		return "", "", echo.NewHTTPError(http.StatusBadRequest, "unknown Authorization scheme")
	}

	if len(pair) < 2 {
		return "", "", echo.NewHTTPError(http.StatusBadRequest, "api_key:token missing")
	}

	if len(pair[0]) == 0 || len(pair[1]) == 0 {
		return "", "", echo.NewHTTPError(http.StatusBadRequest, "empty `api_key` or `token`")
	}

	return pair[0], pair[1], nil
}
