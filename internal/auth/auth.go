package auth

import (
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
)

type BasicAuthConfig struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Config struct {
	BasicAuth BasicAuthConfig

	AuthCollection string
}

type Callbacks struct {
	GetUser           func(recordID string) (User, error)
	GetUsers          func() ([]User, error)
	GetUserByUsername func(username string) (User, error)
}

type Auth struct {
	apiUsers map[string]User
	sync.RWMutex

	cfg       Config
	pb        *pocketbase.PocketBase
	authCol   string
	cb        *Callbacks
	log       *log.Logger
	pbAuth    *PocketBaseAuthService
}

type ClientAuth struct {
	Token  string
	Record map[string]any
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

	a.pbAuth = newPocketBaseAuthService(a)

	if err := a.ensureAuthCollection(); err != nil {
		return nil, err
	}

	return a, nil
}

// UpsertUserAuthRecord writes user credentials and status to PocketBase auth records.
func (o *Auth) UpsertUserAuthRecord(u User, lookupUsername string) (*pbcore.Record, error) {
	return o.pbAuth.UpsertUser(u, lookupUsername)
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

	if col.Fields.GetByName("legacy_user_id") == nil {
		col.Fields.Add(&pbcore.NumberField{Name: "legacy_user_id", OnlyInt: true})
		changed = true
	}

	if col.Fields.GetByName("role") == nil {
		col.Fields.Add(&pbcore.TextField{Name: "role"})
		changed = true
	}

	if col.Fields.GetByName("password_login") == nil {
		col.Fields.Add(&pbcore.BoolField{Name: "password_login"})
		changed = true
	}

	if col.Fields.GetByName("list_role_id") == nil {
		col.Fields.Add(&pbcore.NumberField{Name: "list_role_id", OnlyInt: true})
		changed = true
	}

	if col.Fields.GetByName("twofa_type") == nil {
		col.Fields.Add(&pbcore.TextField{Name: "twofa_type"})
		changed = true
	}

	if col.Fields.GetByName("twofa_key") == nil {
		col.Fields.Add(&pbcore.TextField{Name: "twofa_key"})
		changed = true
	}

	if col.Fields.GetByName("loggedin_at") == nil {
		col.Fields.Add(&pbcore.DateField{Name: "loggedin_at"})
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
	_, err := o.pbAuth.UpsertUser(u, "")
	return err
}

func (o *Auth) DeleteUsers(recordIDs []string) error {
	for _, recordID := range recordIDs {
		if err := o.pbAuth.DeleteByRecordID(recordID); err != nil {
			return err
		}
	}
	return nil
}

func (o *Auth) findAuthRecordByRecordID(recordID string) (*pbcore.Record, error) {
	recordID = strings.TrimSpace(recordID)
	if recordID == "" {
		return nil, sql.ErrNoRows
	}

	return o.pb.FindRecordById(o.authCol, recordID)
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

	rec, err := o.pbAuth.AuthenticatePassword(user.Username, password)
	if err != nil || rec == nil {
		return User{}, echo.NewHTTPError(http.StatusForbidden, "invalid credentials")
	}

	return user, nil
}

func (o *Auth) ValidateUserPassword(username, password string) bool {
	u, err := o.LoginUser(username, password)
	return err == nil && strings.TrimSpace(u.RecordID) != ""
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

	rec, err := o.pbAuth.AuthenticatePassword(u.Username, token)
	if err != nil || rec == nil {
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
			Base: Base{
				RecordID: rec.Id,
			},
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

// Middleware is the HTTP middleware used for wrapping HTTP handlers registered on the echo router.
// It authorizes PocketBase bearer tokens or legacy API key/token pairs and sets the
// authenticated User{} on the echo context on success.
func (o *Auth) Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		hdr := strings.TrimSpace(c.Request().Header.Get("Authorization"))
		if hdr == "" {
			c.Set(UserHTTPCtxKey, echo.NewHTTPError(http.StatusForbidden, "missing auth token"))
			return next(c)
		}

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

		user, ok := o.ValidateAPIToken(key, token)
		if !ok {
			c.Set(UserHTTPCtxKey, echo.NewHTTPError(http.StatusForbidden, "invalid API credentials"))
			return next(c)
		}

		c.Set(UserHTTPCtxKey, user)
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
		if ExtractRoleID(c) == SuperAdminRoleID || u.UserRoleID == SuperAdminRoleID {
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

	return 0
}

// IssueClientAuth syncs a user into PocketBase auth, creates an auth token,
// and returns the payload expected by the JS SDK authStore.
func (o *Auth) IssueClientAuth(u User) (ClientAuth, error) {
	if err := o.SyncUser(u); err != nil {
		o.log.Printf("error syncing auth user: %v", err)
		return ClientAuth{}, echo.NewHTTPError(http.StatusInternalServerError, "error creating auth token")
	}

	rec, err := o.pbAuth.FindByUsername(u.Username)
	if err != nil {
		o.log.Printf("error fetching auth user record: %v", err)
		return ClientAuth{}, echo.NewHTTPError(http.StatusInternalServerError, "error creating auth token")
	}

	token, err := rec.NewAuthToken()
	if err != nil {
		o.log.Printf("error generating auth token: %v", err)
		return ClientAuth{}, echo.NewHTTPError(http.StatusInternalServerError, "error creating auth token")
	}

	return ClientAuth{
		Token:  token,
		Record: rec.PublicExport(),
	}, nil
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
