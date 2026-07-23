package main

import (
	"github.com/compdani/list_pocket/internal/apperr"
	pbcore "github.com/pocketbase/pocketbase/core"
	"regexp"
	"strings"

	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/utils"
	"github.com/compdani/list_pocket/models"
	"github.com/pquerna/otp/totp"
	"gopkg.in/volatiletech/null.v6"
)

var (
	reUsername = regexp.MustCompile(`^[a-zA-Z0-9_\-\.@]+$`)
)

type userReq struct {
	auth.User
	UserRoleRecordID string `json:"user_role_id"`
	ListRoleRecordID string `json:"list_role_id"`
}

func (a *App) userFromReq(req userReq) (auth.User, error) {
	u := req.User

	if strings.TrimSpace(req.UserRoleRecordID) != "" {
		roleID, err := a.core.ResolveRoleLegacyID(req.UserRoleRecordID)
		if err != nil {
			return auth.User{}, apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
		}
		u.UserRoleID = roleID
		u.UserRoleRecID = strings.TrimSpace(req.UserRoleRecordID)
	}

	if strings.TrimSpace(req.ListRoleRecordID) != "" {
		roleID, err := a.core.ResolveRoleLegacyID(req.ListRoleRecordID)
		if err != nil {
			return auth.User{}, apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
		}
		u.ListRoleID = &roleID
		u.ListRoleRecID = strings.TrimSpace(req.ListRoleRecordID)
	} else {
		u.ListRoleID = nil
		u.ListRoleRecID = ""
	}

	return u, nil
}

func userRouteRecordID(re *pbcore.RequestEvent) (string, error) {
	recordID := strings.TrimSpace(pathParam(re, "id"))
	if recordID == "" {
		return "", apperr.BadRequest("invalid ID")
	}

	return recordID, nil
}

// GetUser retrieves a single user by ID.
func (a *App) GetUser(re *pbcore.RequestEvent) error {
	// Get the user from the DB.
	recordID, err := userRouteRecordID(re)
	if err != nil {
		return err
	}
	out, err := a.core.GetUser(recordID, "", "")
	if err != nil {
		return err
	}

	// Blank out the password hash in the response.
	out.Password = null.String{}

	return okJSON(re, out)
}

// GetUsers retrieves all users.
func (a *App) GetUsers(re *pbcore.RequestEvent) error {
	// Get all users from the DB.
	out, err := a.core.GetUsers()
	if err != nil {
		return err
	}

	// Blank out the password hash in the response.
	for n := range out {
		out[n].Password = null.String{}
	}

	return okJSON(re, out)
}

// CreateUser handles user creation.
func (a *App) CreateUser(re *pbcore.RequestEvent) error {
	var req userReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	u, err := a.userFromReq(req)
	if err != nil {
		return err
	}

	u.Username = strings.TrimSpace(u.Username)
	u.Name = strings.TrimSpace(u.Name)
	email := strings.ToLower(strings.TrimSpace(u.Email.String))

	// Validate fields.
	if !strHasLen(u.Username, 3, stdInputMaxLen) {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "username"))
	}
	if !reUsername.MatchString(u.Username) {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "username"))
	}
	if u.Type != auth.UserTypeAPI {
		if !utils.ValidateEmail(email) {
			return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "email"))
		}
		if u.PasswordLogin {
			if !strHasLen(u.Password.String, 8, stdInputMaxLen) {
				return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
			}
		}

		u.Email = null.String{String: email, Valid: true}
	}

	if u.Name == "" {
		u.Name = u.Username
	}

	// Write auth credentials/status to PocketBase first.
	authRec, err := a.auth.UpsertUserAuthRecord(u, "")
	if err != nil {
		return err
	}

	// Create the user in the DB and then mirror role/user metadata back to PocketBase.
	user, err := a.core.CreateUser(u)
	if err != nil {
		if authRec != nil {
			_ = a.pb.Delete(authRec)
		}
		return err
	}

	// Blank out the password hash in the response.
	if user.Type != auth.UserTypeAPI {
		user.Password = null.String{}
	}

	// Cache the API token for in-memory, off-DB /api/* request auth.
	if _, err := refreshAuthCache(a.auth); err != nil {
		return err
	}

	if _, err := a.auth.UpsertUserAuthRecord(user, u.Username); err != nil {
		return err
	}

	return okJSON(re, user)
}

// UpdateUser handles user modification.
func (a *App) UpdateUser(re *pbcore.RequestEvent) error {
	// Incoming params.
	var req userReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	u, err := a.userFromReq(req)
	if err != nil {
		return err
	}

	u.Username = strings.TrimSpace(u.Username)
	u.Name = strings.TrimSpace(u.Name)
	email := strings.ToLower(strings.TrimSpace(u.Email.String))

	// Validate fields.
	if !strHasLen(u.Username, 3, stdInputMaxLen) {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "username"))
	}
	if !reUsername.MatchString(u.Username) {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "username"))
	}

	// Get the user ID.
	recordID, err := userRouteRecordID(re)
	if err != nil {
		return err
	}
	if u.Type != auth.UserTypeAPI {
		if !utils.ValidateEmail(email) {
			return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "email"))
		}

		// Validate password if password login is enabled.
		if u.PasswordLogin && u.Password.String != "" {
			if !strHasLen(u.Password.String, 8, stdInputMaxLen) {
				return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
			}

			if u.Password.String != "" {
				// If a password is sent, validate it before updating in the DB. If it's not set, leave the password in the DB untouched.
				if !strHasLen(u.Password.String, 8, stdInputMaxLen) {
					return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
				}
			} else {
				// Get the user from the DB.
				user, err := a.core.GetUser(recordID, "", "")
				if err != nil {
					return err
				}

				// If password login is enabled, but there's no password in the DB and there's no incoming
				// password, throw an error.
				if !user.HasPassword {
					return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
				}
			}
		}

		u.Email = null.String{String: email, Valid: true}
	}

	// Default the name to username if not set.
	if u.Name == "" {
		u.Name = u.Username
	}

	oldUser, err := a.core.GetUser(recordID, "", "")
	if err != nil {
		return err
	}

	// Write auth credentials/status to PocketBase first.
	if _, err := a.auth.UpsertUserAuthRecord(u, oldUser.Username); err != nil {
		return err
	}

	// Update the user in the DB and then mirror role/user metadata back to PocketBase.
	user, err := a.core.UpdateUser(recordID, u)
	if err != nil {
		_ = a.auth.SyncUser(oldUser)
		return err
	}

	// Blank out the password hash in the response.
	user.Password = null.String{}

	// Cache the API token for in-memory, off-DB /api/* request auth.
	if _, err := refreshAuthCache(a.auth); err != nil {
		return err
	}

	if _, err := a.auth.UpsertUserAuthRecord(user, oldUser.Username); err != nil {
		return err
	}

	return okJSON(re, user)
}

// DeleteUser handles the deletion of a single user by ID.
func (a *App) DeleteUser(re *pbcore.RequestEvent) error {
	// Delete the user(s) from the DB.
	recordID, err := userRouteRecordID(re)
	if err != nil {
		return err
	}
	if err := a.core.DeleteUsers([]string{recordID}); err != nil {
		return err
	}

	// Cache the API token for in-memory, off-DB /api/* request auth.
	if _, err := refreshAuthCache(a.auth); err != nil {
		return err
	}

	if err := a.auth.DeleteUsers([]string{recordID}); err != nil {
		return err
	}

	return okJSON(re, true)
}

// DeleteUsers handles user deletion, either a single one (ID in the URI), or a list.
func (a *App) DeleteUsers(re *pbcore.RequestEvent) error {
	recordIDs := getQueryStrings("record_id", re.Request.URL.Query())
	if len(recordIDs) == 0 {
		return apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
	}

	// Delete the user(s) from the DB.
	if err := a.core.DeleteUsers(recordIDs); err != nil {
		return err
	}

	// Cache the API token for in-memory, off-DB /api/* request auth.
	if _, err := refreshAuthCache(a.auth); err != nil {
		return err
	}

	if err := a.auth.DeleteUsers(recordIDs); err != nil {
		return err
	}

	return okJSON(re, true)
}

// GetUserProfile fetches the uesr profile for the currently logged in user.
func (a *App) GetUserProfile(re *pbcore.RequestEvent) error {
	// Get the authenticated user.
	user := auth.GetUserRE(re)

	// Blank out the password hash in the response.
	user.Password.String = ""
	user.Password.Valid = false

	return okJSON(re, user)
}

// UpdateUserProfile update's the current user's profile.
func (a *App) UpdateUserProfile(re *pbcore.RequestEvent) error {
	// Get the authenticated user.
	user := auth.GetUserRE(re)

	// Incoming params.
	u := auth.User{}
	if err := bindJSON(re, &u); err != nil {
		return err
	}
	u.PasswordLogin = user.PasswordLogin
	u.Name = strings.TrimSpace(u.Name)
	email := strings.TrimSpace(u.Email.String)

	// Validate fields.
	if user.PasswordLogin {
		if !utils.ValidateEmail(email) {
			return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "email"))
		}
		u.Email = null.String{String: email, Valid: true}
	}

	if u.PasswordLogin && u.Password.String != "" {
		if !strHasLen(u.Password.String, 8, stdInputMaxLen) {
			return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
		}
	}

	// Update the user in the DB.
	out, err := a.core.UpdateUserProfile(user.RecordID, u)
	if err != nil {
		return err
	}

	// Blank out the password hash in the response.
	out.Password = null.String{}

	if err := a.auth.SyncUser(out); err != nil {
		return err
	}

	return okJSON(re, out)
}

// EnableTOTP enables TOTP 2FA for a user after verifying the code.
func (a *App) EnableTOTP(re *pbcore.RequestEvent) error {
	var (
		u      = auth.GetUserRE(re)
		secret = strings.TrimSpace(re.Request.FormValue("secret"))
		code   = strings.TrimSpace(re.Request.FormValue("code"))
	)

	if secret == "" || code == "" {
		return apperr.BadRequest(a.i18n.T("globals.messages.invalidFields"))
	}

	// If password login is disabled, can't enable TOTP.
	if !u.PasswordLogin {
		return apperr.BadRequest(a.i18n.T("public.invalidFeature"))
	}

	// If TOTP is already enabled, don't allow re-enabling.
	if u.TwofaType == models.TwofaTypeTOTP {
		return apperr.BadRequest(a.i18n.T("users.twoFAAlreadyEnabled"))
	}

	// Verify the TOTP code.
	valid := totp.Validate(code, secret)
	if !valid {
		return apperr.BadRequest(a.i18n.T("users.invalidTOTPCode"))
	}

	// Enable TOTP in the DB.
	if err := a.core.SetTwoFA(u.RecordID, models.TwofaTypeTOTP, secret); err != nil {
		return err
	}

	return okJSON(re, true)
}

// DisableTOTP disables TOTP 2FA for a user after verifying the password.
func (a *App) DisableTOTP(re *pbcore.RequestEvent) error {
	var (
		u        = auth.GetUserRE(re)
		password = re.Request.FormValue("password")
	)

	// TOTP isn't enabled.
	if u.TwofaType != models.TwofaTypeTOTP {
		return apperr.BadRequest(a.i18n.T("users.twoFANotEnabled"))
	}

	// Validate password.
	if !strHasLen(password, 8, stdInputMaxLen) {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
	}

	// Verify the password.
	if _, err := a.auth.LoginUser(u.Username, password); err != nil {
		return apperr.Forbidden(a.i18n.T("users.invalidPassword"))
	}

	// Disable TOTP in the DB.
	if err := a.core.SetTwoFA(u.RecordID, models.TwofaTypeNone, ""); err != nil {
		return err
	}

	return okJSON(re, true)
}

// refreshAuthCache reloads startup-critical auth state from PocketBase records.
// It returns whether any enabled non-API user exists, which drives first-run setup.
func refreshAuthCache(a *auth.Auth) (bool, error) {
	return a.LoadCachedUsersFromPocketBase()
}
