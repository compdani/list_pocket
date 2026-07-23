package main

import (
	pbcore "github.com/pocketbase/pocketbase/core"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/labstack/echo/v4"
)

// GetInboundEmailInbox returns a paginated list of all inbound emails.
// Handler: GET /mailapi/inbound-emails
func (a *App) GetInboundEmailInbox(re *pbcore.RequestEvent) error {
	limit, _ := strconv.Atoi(queryParam(re, "limit"))
	offset, _ := strconv.Atoi(queryParam(re, "offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	params := core.InboxQueryParams{
		Limit:      limit,
		Offset:     offset,
		Search:     strings.TrimSpace(queryParam(re, "search")),
		SpamStatus: strings.TrimSpace(queryParam(re, "spam_status")),
		SortOrder:  strings.TrimSpace(queryParam(re, "sort")),
	}

	if sd := strings.TrimSpace(queryParam(re, "start_date")); sd != "" {
		if t, err := time.Parse(time.RFC3339, sd); err == nil {
			params.StartDate = &t
		}
	}
	if ed := strings.TrimSpace(queryParam(re, "end_date")); ed != "" {
		if t, err := time.Parse(time.RFC3339, ed); err == nil {
			params.EndDate = &t
		}
	}

	emails, total, err := a.core.GetInboundEmailInbox(re.Request.Context(), params)
	if err != nil {
		return err
	}
	return okJSON(re, map[string]any{
		"results": emails,
		"total":   total,
	})
}

// GetInboundEmailByID returns a single inbound email with full body content.
// Handler: GET /mailapi/inbound-emails/:id
func (a *App) GetInboundEmailByID(re *pbcore.RequestEvent) error {
	id := strings.TrimSpace(pathParam(re, "id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing id")
	}
	email, err := a.core.GetInboundEmailByID(re.Request.Context(), id)
	if err != nil {
		return err
	}
	return okJSON(re, email)
}

// UpdateInboundEmailSpamStatus marks or unmarks an inbound email as spam.
// Handler: PUT /mailapi/inbound-emails/:id/spam
func (a *App) UpdateInboundEmailSpamStatus(re *pbcore.RequestEvent) error {
	id := strings.TrimSpace(pathParam(re, "id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing id")
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := bindJSON(re, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := a.core.UpdateInboundEmailSpamStatus(re.Request.Context(), id, strings.TrimSpace(req.Status)); err != nil {
		return err
	}
	return okJSON(re, true)
}

// GetInboundSpamRules returns paginated spam rules.
// Handler: GET /mailapi/inbound-email-spam-rules
func (a *App) GetInboundSpamRules(re *pbcore.RequestEvent) error {
	limit, _ := strconv.Atoi(queryParam(re, "limit"))
	offset, _ := strconv.Atoi(queryParam(re, "offset"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	ruleType := strings.TrimSpace(queryParam(re, "type"))

	rules, total, err := a.core.GetInboundSpamRules(re.Request.Context(), limit, offset, ruleType)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch spam rules")
	}
	return okJSON(re, map[string]any{
		"results": rules,
		"total":   total,
	})
}

// DeleteInboundSpamRule removes a spam rule by its ID.
// Handler: DELETE /mailapi/inbound-email-spam-rules/:id
func (a *App) DeleteInboundSpamRule(re *pbcore.RequestEvent) error {
	id := strings.TrimSpace(pathParam(re, "id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing id")
	}
	if err := a.core.DeleteInboundSpamRule(re.Request.Context(), id); err != nil {
		return err
	}
	return okJSON(re, true)
}

// GCSpamInboundEmails manually triggers spam email garbage collection.
// Handler: DELETE /mailapi/maintenance/inbound-emails/spam
func (a *App) GCSpamInboundEmails(re *pbcore.RequestEvent) error {
	deleted, err := a.core.DeleteSpamInboundEmails(re.Request.Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "spam GC failed")
	}
	return okJSON(re, map[string]any{"deleted": deleted})
}
