package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/labstack/echo/v4"
)

// GetInboundEmailInbox returns a paginated list of all inbound emails.
// Handler: GET /mailapi/inbound-emails
func (a *App) GetInboundEmailInbox(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	params := core.InboxQueryParams{
		Limit:      limit,
		Offset:     offset,
		Search:     strings.TrimSpace(c.QueryParam("search")),
		SpamStatus: strings.TrimSpace(c.QueryParam("spam_status")),
		SortOrder:  strings.TrimSpace(c.QueryParam("sort")),
	}

	if sd := strings.TrimSpace(c.QueryParam("start_date")); sd != "" {
		if t, err := time.Parse(time.RFC3339, sd); err == nil {
			params.StartDate = &t
		}
	}
	if ed := strings.TrimSpace(c.QueryParam("end_date")); ed != "" {
		if t, err := time.Parse(time.RFC3339, ed); err == nil {
			params.EndDate = &t
		}
	}

	emails, total, err := a.core.GetInboundEmailInbox(c.Request().Context(), params)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{Data: map[string]any{
		"results": emails,
		"total":   total,
	}})
}

// GetInboundEmailByID returns a single inbound email with full body content.
// Handler: GET /mailapi/inbound-emails/:id
func (a *App) GetInboundEmailByID(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing id")
	}
	email, err := a.core.GetInboundEmailByID(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{Data: email})
}

// UpdateInboundEmailSpamStatus marks or unmarks an inbound email as spam.
// Handler: PUT /mailapi/inbound-emails/:id/spam
func (a *App) UpdateInboundEmailSpamStatus(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing id")
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := a.core.UpdateInboundEmailSpamStatus(c.Request().Context(), id, strings.TrimSpace(req.Status)); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{Data: true})
}

// GetInboundSpamRules returns paginated spam rules.
// Handler: GET /mailapi/inbound-email-spam-rules
func (a *App) GetInboundSpamRules(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	ruleType := strings.TrimSpace(c.QueryParam("type"))

	rules, total, err := a.core.GetInboundSpamRules(c.Request().Context(), limit, offset, ruleType)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch spam rules")
	}
	return c.JSON(http.StatusOK, okResp{Data: map[string]any{
		"results": rules,
		"total":   total,
	}})
}

// DeleteInboundSpamRule removes a spam rule by its ID.
// Handler: DELETE /mailapi/inbound-email-spam-rules/:id
func (a *App) DeleteInboundSpamRule(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing id")
	}
	if err := a.core.DeleteInboundSpamRule(c.Request().Context(), id); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{Data: true})
}

// GCSpamInboundEmails manually triggers spam email garbage collection.
// Handler: DELETE /mailapi/maintenance/inbound-emails/spam
func (a *App) GCSpamInboundEmails(c echo.Context) error {
	deleted, err := a.core.DeleteSpamInboundEmails(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "spam GC failed")
	}
	return c.JSON(http.StatusOK, okResp{Data: map[string]any{"deleted": deleted}})
}
