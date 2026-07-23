package main

import (
	pbcore "github.com/pocketbase/pocketbase/core"
	"fmt"
	"net/http"
	"strings"

	"github.com/compdani/list_pocket/internal/auth"
	"github.com/labstack/echo/v4"
)

func roleRouteRecordID(re *pbcore.RequestEvent) (string, error) {
	recordID := strings.TrimSpace(pathParam(re, "id"))
	if recordID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
	}
	return recordID, nil
}

// GetUserRoles retrieves roles.
func (a *App) GetUserRoles(re *pbcore.RequestEvent) error {
	// Get all roles.
	out, err := a.core.GetRoles()
	if err != nil {
		return err
	}

	return okJSON(re, out)
}

// GeListRoles retrieves roles.
func (a *App) GeListRoles(re *pbcore.RequestEvent) error {
	// Get all roles.
	out, err := a.core.GetListRoles()
	if err != nil {
		return err
	}

	return okJSON(re, out)
}

// CreateUserRole handles role creation.
func (a *App) CreateUserRole(re *pbcore.RequestEvent) error {
	var r auth.Role
	if err := bindJSON(re, &r); err != nil {
		return err
	}
	if err := a.validateUserRole(r); err != nil {
		return err
	}

	// Create the role in the DB.
	out, err := a.core.CreateRole(r)
	if err != nil {
		return err
	}

	return okJSON(re, out)
}

// CreateListRole handles role creation.
func (a *App) CreateListRole(re *pbcore.RequestEvent) error {
	var r auth.ListRole
	if err := bindJSON(re, &r); err != nil {
		return err
	}
	if err := a.validateListRole(r); err != nil {
		return err
	}

	// Create the role in the DB.
	out, err := a.core.CreateListRole(r)
	if err != nil {
		return err
	}

	return okJSON(re, out)
}

// UpdateUserRole handles role modification.
func (a *App) UpdateUserRole(re *pbcore.RequestEvent) error {
	recordID, err := roleRouteRecordID(re)
	if err != nil {
		return err
	}

	// ID 1 is reserved for the Super Admin user role.
	current, err := a.core.GetRole(recordID)
	if err != nil {
		return err
	}
	if current.ID == auth.SuperAdminRoleID {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidID"))
	}

	// Incoming params.
	var r auth.Role
	if err := bindJSON(re, &r); err != nil {
		return err
	}
	if err := a.validateUserRole(r); err != nil {
		return err
	}

	// Validate.
	r.Name.String = strings.TrimSpace(r.Name.String)

	// Update the role in the DB.
	out, err := a.core.UpdateUserRole(recordID, r)
	if err != nil {
		return err
	}

	// Cache API tokens for in-memory, off-DB /api/* request auth.
	if _, err := refreshAuthCache(a.auth); err != nil {
		return err
	}

	return okJSON(re, out)
}

// UpdateListRole handles role modification.
func (a *App) UpdateListRole(re *pbcore.RequestEvent) error {
	recordID, err := roleRouteRecordID(re)
	if err != nil {
		return err
	}

	// ID 1 is reserved for the Super Admin user role.
	current, err := a.core.GetRole(recordID)
	if err != nil {
		return err
	}
	if current.ID == auth.SuperAdminRoleID {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidID"))
	}

	// Incoming params.
	var r auth.ListRole
	if err := bindJSON(re, &r); err != nil {
		return err
	}

	if err := a.validateListRole(r); err != nil {
		return err
	}

	// Validate.
	r.Name.String = strings.TrimSpace(r.Name.String)

	// Update the role in the DB.
	out, err := a.core.UpdateListRole(recordID, r)
	if err != nil {
		return err
	}

	// Cache API tokens for in-memory, off-DB /api/* request auth.
	if _, err := refreshAuthCache(a.auth); err != nil {
		return err
	}

	return okJSON(re, out)
}

// DeleteRole handles (user|list) role deletion.
func (a *App) DeleteRole(re *pbcore.RequestEvent) error {
	recordID, err := roleRouteRecordID(re)
	if err != nil {
		return err
	}

	// ID 1 is reserved for the Super Admin user role.
	current, err := a.core.GetRole(recordID)
	if err != nil {
		return err
	}
	if current.ID == auth.SuperAdminRoleID {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidID"))
	}

	// Delete the role from the DB.
	if err := a.core.DeleteRole(recordID); err != nil {
		return err
	}

	// Cache API tokens for in-memory, off-DB /api/* request auth.
	if _, err := refreshAuthCache(a.auth); err != nil {
		return err
	}

	return okJSON(re, true)
}

func (a *App) validateUserRole(r auth.Role) error {
	if !strHasLen(r.Name.String, 1, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "name"))
	}

	for _, p := range r.Permissions {
		if _, ok := a.cfg.Permissions[p]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", fmt.Sprintf("permission: %s", p)))
		}
	}

	return nil
}

func (a *App) validateListRole(r auth.ListRole) error {
	if !strHasLen(r.Name.String, 1, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "name"))
	}

	for _, l := range r.Lists {
		for _, p := range l.Permissions {
			if p != auth.PermListGet && p != auth.PermListManage {
				return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", fmt.Sprintf("list permission: %s", p)))
			}
		}
	}

	return nil
}
