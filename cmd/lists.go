package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
)

func (a *App) resolveListRouteID(c echo.Context) (string, error) {
	recordID := strings.TrimSpace(c.Param("id"))
	if recordID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
	}

	return recordID, nil
}

func (a *App) resolveListRequestIDs(recordIDs []string) ([]int, error) {
	ids, err := a.core.ResolveListIDs(nil, recordIDs)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	return ids, nil
}

// GetLists retrieves lists with additional metadata like subscriber counts.
func (a *App) GetLists(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Get the list IDs (or blanket permission) the user has access to.
	hasAllPerm, permittedRecordIDs := user.GetPermittedLists(auth.PermTypeGet)
	permittedIDs, err := a.resolveListRequestIDs(permittedRecordIDs)
	if err != nil {
		return err
	}

	// Minimal query simply returns the list of all lists without JOIN subscriber counts. This is fast.
	minimal, _ := strconv.ParseBool(c.FormValue("minimal"))
	if minimal {
		status := c.FormValue("status")
		res, err := a.core.GetLists("", status, hasAllPerm, permittedIDs)
		if err != nil {
			return err
		}
		if len(res) == 0 {
			return c.JSON(http.StatusOK, okResp{[]struct{}{}})
		}

		// Meta.
		total := len(res)
		out := models.PageResults{
			Results: res,
			Total:   total,
			Page:    1,
			PerPage: total,
		}

		return c.JSON(http.StatusOK, okResp{out})
	}

	// Full list query.
	var (
		query   = strings.TrimSpace(c.FormValue("query"))
		tags    = c.QueryParams()["tag"]
		orderBy = c.FormValue("order_by")
		typ     = c.FormValue("type")
		optin   = c.FormValue("optin")
		status  = c.FormValue("status")
		order   = c.FormValue("order")

		pg = a.pg.NewFromURL(c.Request().URL.Query())
	)
	res, total, err := a.core.QueryLists(query, typ, optin, status, tags, orderBy, order, hasAllPerm, permittedIDs, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}

	out := models.PageResults{
		Query:   query,
		Results: res,
		Total:   total,
		Page:    pg.Page,
		PerPage: pg.PerPage,
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetList retrieves a single list by id.
// It's permission checked by the listPerm middleware.
func (a *App) GetList(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Check if the user has access to the list.
	recordID, err := a.resolveListRouteID(c)
	if err != nil {
		return err
	}
	if err := user.HasListPerm(auth.PermTypeGet, recordID); err != nil {
		return err
	}

	// Get the list from the DB.
	out, err := a.core.GetList(recordID, "")
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// CreateList handles list creation.
func (a *App) CreateList(c echo.Context) error {
	l := models.List{}
	if err := c.Bind(&l); err != nil {
		return err
	}

	// Validate.
	if !strHasLen(l.Name, 1, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.invalidName"))
	}

	out, err := a.core.CreateList(l)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateList handles list modification.
// It's permission checked by the listPerm middleware.
func (a *App) UpdateList(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Check if the user has access to the list.
	recordID, err := a.resolveListRouteID(c)
	if err != nil {
		return err
	}
	if err := user.HasListPerm(auth.PermTypeManage, recordID); err != nil {
		return err
	}

	// Incoming params.
	var l models.List
	if err := c.Bind(&l); err != nil {
		return err
	}

	// Validate.
	if !strHasLen(l.Name, 1, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.invalidName"))
	}

	// Update the list in the DB.
	out, err := a.core.UpdateList(recordID, l)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// DeleteList deletes a single list by ID.
func (a *App) DeleteList(c echo.Context) error {
	recordID, err := a.resolveListRouteID(c)
	if err != nil {
		return err
	}
	// Check if the user has manage permission for the list.
	user := auth.GetUser(c)
	if err := user.HasListPerm(auth.PermTypeManage, recordID); err != nil {
		return err
	}

	// Delete the list from the DB.
	// Pass getAll=true since we've already verified permissions above.
	if err := a.core.DeleteLists([]string{recordID}, "", true, nil); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// DeleteLists deletes multiple lists by IDs or by query.
func (a *App) DeleteLists(c echo.Context) error {
	user := auth.GetUser(c)

	var (
		recordIDs []string
		query     string
		all       bool
	)

	if len(c.Request().URL.Query()["record_id"]) > 0 {
		recordIDs = getQueryStrings("record_id", c.Request().URL.Query())
	} else {
		query = strings.TrimSpace(c.FormValue("query"))
		all = c.FormValue("all") == "true"
	}

	// Validate that either IDs or query is provided.
	if len(recordIDs) == 0 && (query == "" && !all) {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", "record_id or query required"))
	}

	// For ID deletion, check if the user has manage permission for the specific lists.
	if len(recordIDs) > 0 {
		if err := user.HasListPerm(auth.PermTypeManage, recordIDs...); err != nil {
			return err
		}

		// Delete the lists from the DB.
		// Pass getAll=true since we've already verified permissions above.
		if err := a.core.DeleteLists(recordIDs, "", true, nil); err != nil {
			return err
		}
	} else {
		// For query deletion, get the list IDs the user has manage permission for.
		hasAllPerm, permittedRecordIDs := user.GetPermittedLists(auth.PermTypeManage)
		permittedIDs, err := a.resolveListRequestIDs(permittedRecordIDs)
		if err != nil {
			return err
		}

		// Delete the lists from the DB with permission filtering.
		if err := a.core.DeleteLists(nil, query, hasAllPerm, permittedIDs); err != nil {
			return err
		}
	}

	return c.JSON(http.StatusOK, okResp{true})
}
