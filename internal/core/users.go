package core

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/utils"
	pbcore "github.com/pocketbase/pocketbase/core"
	"gopkg.in/volatiletech/null.v6"
)

func (c *Core) GetUsers() ([]auth.User, error) {
	pb := c.db.PocketBase()
	if pb == nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.users}", "error", "pocketbase unavailable"))
	}

	recs, err := pb.FindRecordsByFilter("users", "", "created", 0, 0)
	if err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.users}", "error", err.Error()))
	}

	out := make([]auth.User, 0, len(recs))
	for _, rec := range recs {
		out = append(out, userFromAuthRecord(rec))
	}

	return c.hydrateUsers(out)
}

// GetUser retrieves a specific user based on any one given identifier.
func (c *Core) GetUser(recordID, username, email string) (auth.User, error) {
	rec, err := c.findAuthUserRecord(recordID, username, email)
	if err != nil {
		if err == sql.ErrNoRows {
			return auth.User{}, apperr.NotFound(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.user}"))
		}

		return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.users}", "error", err.Error()))
	}

	users, err := c.hydrateUsers([]auth.User{userFromAuthRecord(rec)})
	if err != nil {
		return auth.User{}, err
	}

	return users[0], nil
}

// CreateUser creates a new user.
func (c *Core) CreateUser(u auth.User) (auth.User, error) {
	pb := c.db.PocketBase()
	if pb == nil {
		return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.user}", "error", "pocketbase unavailable"))
	}

	// If it's an API user, generate a random token for password
	// and set the e-mail to default.
	if u.Type == auth.UserTypeAPI {
		tk, err := utils.GenerateRandomString(32)
		if err != nil {
			return auth.User{}, err
		}

		u.Email = null.String{String: u.Username + "@api", Valid: true}
		u.PasswordLogin = false
		u.Password = null.String{String: tk, Valid: true}
	}

	rec, err := c.findAuthUserRecord("", u.Username, "")
	if err != nil && err != sql.ErrNoRows {
		return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.user}", "error", err.Error()))
	}
	if err == sql.ErrNoRows || rec == nil {
		col, colErr := pb.FindCollectionByNameOrId("users")
		if colErr != nil {
			return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.user}", "error", colErr.Error()))
		}
		rec = pbcore.NewRecord(col)
	}

	if rec.GetInt("legacy_user_id") <= 0 {
		nextID, err := c.nextLegacyUserID()
		if err != nil {
			return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.user}", "error", err.Error()))
		}
		rec.Set("legacy_user_id", nextID)
	}

	if err := c.applyUserToRecord(rec, u, true); err != nil {
		return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	if err := pb.Save(rec); err != nil {
		return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	out, err := c.GetUser(rec.Id, "", "")
	if err != nil {
		return auth.User{}, err
	}

	// Expose the generated API token once on creation.
	if u.Type == auth.UserTypeAPI {
		out.Password = u.Password
	} else {
		out.Password = null.String{}
	}

	return out, nil
}

// UpdateUser updates a given user.
func (c *Core) UpdateUser(recordID string, u auth.User) (auth.User, error) {
	rec, err := c.findAuthUserRecord(recordID, "", "")
	if err != nil {
		if err == sql.ErrNoRows {
			return auth.User{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.user}"))
		}
		return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	oldUser, err := c.GetUser(recordID, "", "")
	if err != nil {
		return auth.User{}, err
	}

	if oldUser.Type == auth.UserTypeUser && oldUser.Status == auth.UserStatusEnabled &&
		oldUser.UserRoleID == auth.SuperAdminRoleID &&
		(u.UserRoleID != auth.SuperAdminRoleID || u.Status != auth.UserStatusEnabled) {
		num, err := c.countOtherEnabledSuperAdmins(recordID)
		if err != nil {
			return auth.User{}, err
		}
		if num == 0 {
			return auth.User{}, apperr.BadRequest(c.i18n.T("users.needSuper"))
		}
	}

	if err := c.applyUserToRecord(rec, u, false); err != nil {
		return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	if err := c.db.PocketBase().Save(rec); err != nil {
		return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	return c.GetUser(recordID, "", "")
}

// UpdateUserProfile updates the basic fields of a given user (name, email, password).
func (c *Core) UpdateUserProfile(recordID string, u auth.User) (auth.User, error) {
	rec, err := c.findAuthUserRecord(recordID, "", "")
	if err != nil {
		if err == sql.ErrNoRows {
			return auth.User{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.user}"))
		}
		return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	if strings.TrimSpace(u.Name) != "" {
		rec.Set("name", u.Name)
	}
	if u.Email.Valid {
		rec.SetEmail(u.Email.String)
	}
	if u.Password.String != "" {
		rec.Set(pbcore.FieldNamePassword, u.Password.String)
		rec.Set("passwordConfirm", u.Password.String)
	}

	if err := c.db.PocketBase().Save(rec); err != nil {
		return auth.User{}, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	return c.GetUser(recordID, "", "")
}

// UpdateUserLogin updates a user's record post-login.
func (c *Core) UpdateUserLogin(recordID string, avatar string) error {
	rec, err := c.findAuthUserRecord(recordID, "", "")
	if err != nil {
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	rec.Set("loggedin_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))
	if strings.TrimSpace(avatar) != "" {
		rec.Set("avatar", avatar)
	}

	if err := c.db.PocketBase().Save(rec); err != nil {
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	return nil
}

// SetTwoFA sets or clears the 2FA configuration for a user.
func (c *Core) SetTwoFA(recordID string, twofaType, twofaKey string) error {
	rec, err := c.findAuthUserRecord(recordID, "", "")
	if err != nil {
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	rec.Set("twofa_type", twofaType)
	rec.Set("twofa_key", twofaKey)

	if err := c.db.PocketBase().Save(rec); err != nil {
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.user}", "error", err.Error()))
	}

	return nil
}

// DeleteUsers validates deletion of a given user set. The actual auth record
// removal is handled by the auth module so it can stay the single deleter.
func (c *Core) DeleteUsers(recordIDs []string) error {
	users, err := c.GetUsers()
	if err != nil {
		return err
	}

	deleting := make(map[string]struct{}, len(recordIDs))
	for _, recordID := range recordIDs {
		deleting[strings.TrimSpace(recordID)] = struct{}{}
	}

	numEnabledSupers := 0
	for _, u := range users {
		if _, ok := deleting[u.RecordID]; ok {
			continue
		}
		if u.Type == auth.UserTypeUser && u.Status == auth.UserStatusEnabled && u.UserRoleID == auth.SuperAdminRoleID {
			numEnabledSupers++
		}
	}

	for _, u := range users {
		if _, ok := deleting[u.RecordID]; !ok {
			continue
		}
		if u.Type == auth.UserTypeUser && u.Status == auth.UserStatusEnabled && u.UserRoleID == auth.SuperAdminRoleID && numEnabledSupers == 0 {
			return apperr.BadRequest(c.i18n.T("users.needSuper"))
		}
	}

	return nil
}

// LoginUser attempts to log the given user in by matching the password.
func (c *Core) LoginUser(username, password string) (auth.User, error) {
	rec, err := c.findAuthUserRecord("", username, "")
	if err != nil || rec == nil || !rec.ValidatePassword(password) {
		return auth.User{}, apperr.Forbidden(c.i18n.T("users.invalidLogin"))
	}

	return c.GetUser("", username, "")
}

func (c *Core) findAuthUserRecord(recordID, username, email string) (*pbcore.Record, error) {
	pb := c.db.PocketBase()
	if pb == nil {
		return nil, fmt.Errorf("pocketbase unavailable")
	}

	switch {
	case strings.TrimSpace(recordID) != "":
		return pb.FindRecordById("users", strings.TrimSpace(recordID))
	case strings.TrimSpace(username) != "":
		return pb.FindFirstRecordByData("users", "username", strings.TrimSpace(username))
	case strings.TrimSpace(email) != "":
		return pb.FindFirstRecordByData("users", "email", strings.TrimSpace(email))
	default:
		return nil, sql.ErrNoRows
	}
}

func (c *Core) nextLegacyUserID() (int, error) {
	pb := c.db.PocketBase()
	if pb == nil {
		return 0, fmt.Errorf("pocketbase unavailable")
	}

	recs, err := pb.FindRecordsByFilter("users", "", "-legacy_user_id", 1, 0)
	if err != nil || len(recs) == 0 {
		return 1, nil
	}

	return recs[0].GetInt("legacy_user_id") + 1, nil
}

func (c *Core) applyUserToRecord(rec *pbcore.Record, u auth.User, create bool) error {
	email := strings.TrimSpace(u.Email.String)
	if email == "" {
		email = fmt.Sprintf("%s@api.local", strings.ToLower(u.Username))
	}

	roleID := u.UserRoleID
	if roleID < 1 {
		if strings.TrimSpace(u.UserRoleRecID) != "" {
			resolvedRoleID, err := c.ResolveRoleLegacyID(u.UserRoleRecID)
			if err != nil {
				return err
			}
			roleID = resolvedRoleID
		}
	}

	rec.SetEmail(email)
	rec.Set("username", u.Username)
	rec.Set("name", u.Name)
	rec.Set("user_type", u.Type)
	rec.Set("status", u.Status)
	rec.Set("role", strconv.Itoa(roleID))
	rec.Set("password_login", u.PasswordLogin)
	rec.SetVerified(true)

	if u.ListRoleID != nil {
		rec.Set("list_role_id", *u.ListRoleID)
	} else {
		rec.Set("list_role_id", 0)
	}

	rec.Set("twofa_type", u.TwofaType)
	rec.Set("twofa_key", u.TwofaKey.String)

	if create && rec.GetInt("legacy_user_id") <= 0 {
		nextID, err := c.nextLegacyUserID()
		if err != nil {
			return err
		}
		rec.Set("legacy_user_id", nextID)
	}

	if u.Password.String != "" {
		rec.Set(pbcore.FieldNamePassword, u.Password.String)
		rec.Set("passwordConfirm", u.Password.String)
	} else if create {
		placeholder := fmt.Sprintf("lm-disabled-%d-%d", rec.GetInt("legacy_user_id"), time.Now().UnixNano())
		rec.Set(pbcore.FieldNamePassword, placeholder)
		rec.Set("passwordConfirm", placeholder)
	}

	return nil
}

func userFromAuthRecord(rec *pbcore.Record) auth.User {
	out := auth.User{
		Base: auth.Base{
			ID:        rec.GetInt("legacy_user_id"),
			RecordID:  rec.Id,
			CreatedAt: parseNullTime(rec.GetString("created")),
			UpdatedAt: parseNullTime(rec.GetString("updated")),
		},
		Username:      rec.GetString("username"),
		Email:         null.NewString(rec.Email(), strings.TrimSpace(rec.Email()) != ""),
		Name:          rec.GetString("name"),
		Type:          strings.TrimSpace(rec.GetString("user_type")),
		Status:        strings.TrimSpace(rec.GetString("status")),
		Avatar:        null.NewString(rec.GetString("avatar"), strings.TrimSpace(rec.GetString("avatar")) != ""),
		TwofaType:     rec.GetString("twofa_type"),
		TwofaKey:      null.NewString(rec.GetString("twofa_key"), strings.TrimSpace(rec.GetString("twofa_key")) != ""),
		LoggedInAt:    parseNullTime(rec.GetString("loggedin_at")),
		UserRoleID:    auth.ExtractRoleIDFromRecord(rec),
		PasswordLogin: rec.GetBool("password_login"),
	}

	if out.Type == "" {
		out.Type = auth.UserTypeUser
	}
	if out.Status == "" {
		out.Status = auth.UserStatusEnabled
	}
	if !out.PasswordLogin && out.Type == auth.UserTypeUser && strings.TrimSpace(rec.GetString("password:hash")) != "" {
		out.PasswordLogin = true
	}

	if listRoleID := rec.GetInt("list_role_id"); listRoleID > 0 {
		out.ListRoleID = &listRoleID
	}

	if strings.TrimSpace(rec.GetString("password:hash")) != "" {
		out.HasPassword = true
	}

	return out
}

func parseNullTime(value string) null.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return null.Time{}
	}

	layouts := []string{
		"2006-01-02 15:04:05.000Z",
		time.RFC3339,
		"2006-01-02 15:04:05",
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return null.NewTime(t, true)
		}
	}

	return null.Time{}
}

func (c *Core) hydrateUsers(users []auth.User) ([]auth.User, error) {
	userRoles, err := c.GetRoles()
	if err != nil {
		return nil, err
	}

	listRoles, err := c.GetListRoles()
	if err != nil {
		return nil, err
	}

	userRoleMap := make(map[int]auth.Role, len(userRoles))
	userRoleRecMap := make(map[string]auth.Role, len(userRoles))
	for _, role := range userRoles {
		userRoleMap[role.ID] = role
		userRoleRecMap[role.RecordID] = role
	}

	listRoleMap := make(map[int]auth.ListRole, len(listRoles))
	listRoleRecMap := make(map[string]auth.ListRole, len(listRoles))
	for _, role := range listRoles {
		listRoleMap[role.ID] = role
		listRoleRecMap[role.RecordID] = role
	}

	for i := range users {
		u := &users[i]

		if role, ok := userRoleMap[u.UserRoleID]; ok {
			u.UserRoleRecID = role.RecordID
			u.UserRoleName = role.Name.String
			u.UserRolePerms = role.Permissions
			u.UserRole.ID = role.RecordID
		}

		if u.ListRoleID != nil {
			if role, ok := listRoleMap[*u.ListRoleID]; ok {
				u.ListRoleRecID = role.RecordID
				u.ListRoleName = role.Name
				if b, err := json.Marshal(role.Lists); err == nil {
					raw := json.RawMessage(b)
					u.ListsPermsRaw = &raw
				}
			}
		}

		if u.UserRoleRecID == "" {
			if role, ok := userRoleRecMap[u.UserRole.ID]; ok {
				u.UserRoleID = role.ID
				u.UserRoleRecID = role.RecordID
			}
		}
		if u.ListRoleRecID != "" {
			if role, ok := listRoleRecMap[u.ListRoleRecID]; ok {
				id := role.ID
				u.ListRoleID = &id
			}
		}
	}

	return c.setupUserFields(users), nil
}

func (c *Core) countOtherEnabledSuperAdmins(excludeRecordID string) (int, error) {
	users, err := c.GetUsers()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, u := range users {
		if u.RecordID == excludeRecordID {
			continue
		}
		if u.Type == auth.UserTypeUser && u.Status == auth.UserStatusEnabled && u.UserRoleID == auth.SuperAdminRoleID {
			count++
		}
	}

	return count, nil
}

// setupUserFields prepares and sets up various user fields.
func (c *Core) setupUserFields(users []auth.User) []auth.User {
	for n, u := range users {
		u := u

		if u.HasPassword {
			u.PasswordLogin = true
		}

		if u.Type == auth.UserTypeAPI {
			u.Email = null.String{}
		}

		u.UserRole.ID = u.UserRoleRecID
		u.UserRole.Name = u.UserRoleName
		u.UserRole.Permissions = u.UserRolePerms

		// Prepare lookup maps.
		u.ListPermissionsMap = make(map[string]map[string]struct{})
		u.PermissionsMap = make(map[string]struct{})
		for _, p := range u.UserRolePerms {
			u.PermissionsMap[p] = struct{}{}
		}

		if u.ListRoleID != nil {
			var listPerms []auth.ListPermission
			if u.ListsPermsRaw != nil {
				if err := json.Unmarshal(*u.ListsPermsRaw, &listPerms); err != nil {
					c.log.Printf("error unmarshalling list permissions for role %d: %v", u.ID, err)
				}
			}

			u.ListRole = &auth.ListRolePermissions{ID: u.ListRoleRecID, Name: u.ListRoleName.String, Lists: listPerms}

			for _, p := range listPerms {
				u.ListPermissionsMap[p.ID] = make(map[string]struct{})

				for _, perm := range p.Permissions {
					u.ListPermissionsMap[p.ID][perm] = struct{}{}

					if perm == auth.PermListGet {
						u.GetListIDs = append(u.GetListIDs, p.ID)
					}
					if perm == auth.PermListManage {
						u.ManageListIDs = append(u.ManageListIDs, p.ID)
					}
				}
			}
		}

		users[n] = u
	}

	return users
}
