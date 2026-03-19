package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/notifs"
	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"gopkg.in/volatiletech/null.v6"
)

// campReq is a wrapper over the Campaign model for receiving
// campaign creation and update data from APIs.
type campReq struct {
	models.Campaign

	// This overrides Campaign.Lists to receive and
	// write a list of int IDs during creation and updation.
	// Campaign.Lists is JSONText for sending lists children
	// to the outside world.
	ListIDs       []int    `json:"lists"`
	ListRecordIDs []string `json:"list_record_ids"`

	MediaIDs []int `json:"media"`

	DripEnabled bool                `json:"drip_enabled"`
	Batching    campaignBatchingReq `json:"batching"`

	// This is only relevant to campaign test requests.
	SubscriberEmails pq.StringArray `json:"subscribers"`
}

// campContentReq wraps params coming from API requests for converting
// campaign content formats.
type campContentReq struct {
	models.Campaign
	From string `json:"from"`
	To   string `json:"to"`
}

var (
	reFromAddress = regexp.MustCompile(`((.+?)\s)?<(.+?)@(.+?)>`)
	reSlug        = regexp.MustCompile(`[^\p{L}\p{M}\p{N}]`)
)

func (a *App) resolveCampaignRouteID(c echo.Context) (string, error) {
	recordID := strings.TrimSpace(c.Param("id"))
	if recordID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
	}

	return recordID, nil
}

func bindCampaignReq(c echo.Context, out *campReq) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return err
	}

	normalized, err := normalizeCampaignReqBody(body)
	if err != nil {
		return err
	}

	return json.Unmarshal(normalized, out)
}

func normalizeCampaignReqBody(body []byte) ([]byte, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return body, nil
	}

	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	if raw, ok := payload["body_source"]; ok {
		raw = bytes.TrimSpace(raw)
		if len(raw) > 0 && raw[0] != '"' && string(raw) != "null" {
			encoded, err := json.Marshal(string(raw))
			if err != nil {
				return nil, err
			}
			payload["body_source"] = encoded
		}
	}

	if raw, ok := payload["media"]; ok {
		var vals []any
		if err := json.Unmarshal(raw, &vals); err != nil {
			return nil, err
		}

		mediaIDs := make([]int, 0, len(vals))
		for _, v := range vals {
			switch n := v.(type) {
			case float64:
				mediaIDs = append(mediaIDs, int(n))
			case string:
				id, err := strconv.Atoi(strings.TrimSpace(n))
				if err != nil {
					return nil, err
				}
				mediaIDs = append(mediaIDs, id)
			}
		}

		encoded, err := json.Marshal(mediaIDs)
		if err != nil {
			return nil, err
		}
		payload["media"] = encoded
	}

	return json.Marshal(payload)
}

func campaignListRecordIDs(raw any) []string {
	if raw == nil {
		return nil
	}

	type campaignListRef struct {
		ID string `json:"id"`
	}

	var lists []campaignListRef
	switch v := raw.(type) {
	case []byte:
		_ = json.Unmarshal(v, &lists)
	case string:
		_ = json.Unmarshal([]byte(v), &lists)
	}

	out := make([]string, 0, len(lists))
	for _, item := range lists {
		if id := strings.TrimSpace(item.ID); id != "" {
			out = append(out, id)
		}
	}

	return out
}

// GetCampaigns handles retrieval of campaigns.
func (a *App) GetCampaigns(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	var (
		hasAllPerm     = user.HasPerm(auth.PermCampaignsGetAll)
		permittedLists []int
		err            error
	)

	if !hasAllPerm {
		// Either the user has campaigns:get_all permissions and can view all campaigns,
		// or the campaigns are filtered by the lists the user has get|manage access to.
		permittedRecordIDs := []string{}
		hasAllPerm, permittedRecordIDs = user.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage)
		permittedLists, err = a.core.ResolveListIDs(nil, permittedRecordIDs)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
		}
	}

	var (
		pg = a.pg.NewFromURL(c.Request().URL.Query())

		status    = c.QueryParams()["status"]
		tags      = c.QueryParams()["tag"]
		query     = strings.TrimSpace(c.FormValue("query"))
		orderBy   = c.FormValue("order_by")
		order     = c.FormValue("order")
		noBody, _ = strconv.ParseBool(c.QueryParam("no_body"))
	)

	// Query and retrieve campaigns from the DB.
	res, total, err := a.core.QueryCampaigns(query, status, tags, orderBy, order, hasAllPerm, permittedLists, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}

	// Remove the body from the response if requested.
	if noBody {
		for i := range res {
			res[i].Body = ""
			res[i].BodySource.Valid = false
		}
	}

	// Paginate the response.
	if len(res) == 0 {
		return c.JSON(http.StatusOK, okResp{models.PageResults{Results: []models.Campaign{}}})
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

// GetCampaign handles retrieval of campaigns.
func (a *App) GetCampaign(c echo.Context) error {
	// Get the campaign ID.
	recordID, err := a.resolveCampaignRouteID(c)
	if err != nil {
		return err
	}

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeGet, recordID, c); err != nil {
		return err
	}

	// Get the campaign from the DB.
	out, err := a.core.GetCampaign(recordID, "", "")
	if err != nil {
		return err
	}

	// Blank out the body if requested.
	noBody, _ := strconv.ParseBool(c.QueryParam("no_body"))
	if noBody {
		out.Body = ""
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// PreviewCampaign renders the HTML preview of a campaign body.
func (a *App) PreviewCampaign(c echo.Context) error {
	// Get the campaign ID.
	recordID, err := a.resolveCampaignRouteID(c)
	if err != nil {
		return err
	}

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeGet, recordID, c); err != nil {
		return err
	}

	var (
		isPost      = c.Request().Method == http.MethodPost
		contentType = c.FormValue("content_type")
		tplID       = strings.TrimSpace(c.FormValue("template_id"))
	)
	// For visual content, template ID for previewing is irrelevant.
	if contentType == models.CampaignContentTypeVisual {
		tplID = ""
	}

	// Get the campaign from the DB for previewing with the `template_body` field.
	camp, err := a.core.GetCampaignForPreview(recordID, tplID)
	if err != nil {
		return err
	}

	// There's a body in the request to preview instead of the body in the DB.
	if isPost {
		camp.ContentType = contentType
		camp.Body = c.FormValue("body")

		// For visual campaigns, template body from the DB shouldn't be used.
		if contentType == models.CampaignContentTypeVisual {
			camp.TemplateBody = ""
		}
	}

	// Use a dummy campaign ID to prevent views and clicks from {{ TrackView }}
	// and {{ TrackLink }} being registered on preview.
	camp.UUID = dummySubscriber.UUID
	if err := camp.CompileTemplate(a.manager.TemplateFuncs(&camp)); err != nil {
		a.log.Printf("error compiling template: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("templates.errorCompiling", "error", err.Error()))
	}

	// Render the message body.
	msg, err := a.manager.NewCampaignMessage(&camp, dummySubscriber)
	if err != nil {
		a.log.Printf("error rendering message: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("templates.errorRendering", "error", err.Error()))
	}

	// Plaintext headers for plain body.
	if camp.ContentType == models.CampaignContentTypePlain {
		return c.String(http.StatusOK, string(msg.Body()))
	}

	return c.HTML(http.StatusOK, string(msg.Body()))
}

// PreviewCampaignArchive renders the public campaign archives page.
func (a *App) PreviewCampaignArchive(c echo.Context) error {
	// Get the campaign ID.
	recordID, err := a.resolveCampaignRouteID(c)
	if err != nil {
		return err
	}

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeGet, recordID, c); err != nil {
		return err
	}

	// Fetch the campaign body from the DB.
	tplID := strings.TrimSpace(c.FormValue("template_id"))
	camp, err := a.core.GetCampaignForPreview(recordID, tplID)
	if err != nil {
		return err
	}

	camp.ArchiveMeta = json.RawMessage([]byte(c.FormValue("archive_meta")))

	// "Compile" the campaign template with appropriate data.
	res, err := a.compileArchiveCampaigns([]models.Campaign{camp})
	if err != nil {
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorFetchingCampaign")))
	}

	// Render the campaign body.
	out := res[0].Campaign
	msg, err := a.manager.NewCampaignMessage(out, res[0].Subscriber)
	if err != nil {
		a.log.Printf("error rendering campaign: %v", err)
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorFetchingCampaign")))
	}

	return c.HTML(http.StatusOK, string(msg.Body()))
}

// CampaignContent handles campaign content (body) format conversions.
func (a *App) CampaignContent(c echo.Context) error {
	var camp campContentReq
	if err := c.Bind(&camp); err != nil {
		return err
	}

	// Convert formats, eg: markdown to HTML.
	out, err := camp.ConvertContent(camp.From, camp.To)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// CreateCampaign handles campaign creation.
// Newly created campaigns are always drafts.
func (a *App) CreateCampaign(c echo.Context) error {
	var o campReq
	if err := bindCampaignReq(c, &o); err != nil {
		return err
	}
	a.log.Printf("create campaign: received name=%q type=%q content_type=%q list_ids=%v media_ids=%v messenger=%q archive=%v template_id_valid=%v archive_template_id_valid=%v",
		o.Name, o.Type, o.ContentType, o.ListIDs, o.MediaIDs, o.Messenger, o.Archive, o.TemplateID.Valid, o.ArchiveTemplateID.Valid)

	// Filter lists against the current user's permitted lists.
	user := auth.GetUser(c)
	filteredListRecordIDs := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, o.ListRecordIDs)
	resolvedListIDs, err := a.core.ResolveListIDs(nil, filteredListRecordIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	o.ListIDs = resolvedListIDs
	a.log.Printf("create campaign: filtered lists username=%q role_id=%d permitted_list_ids=%v", user.Username, user.UserRoleID, o.ListIDs)

	// If the campaign's 'opt-in', prepare a default message.
	switch o.Type {
	case models.CampaignTypeOptin:
		op, err := a.makeOptinCampaignMessage(o)
		if err != nil {
			return err
		}
		o = op
	case "":
		o.Type = models.CampaignTypeRegular
	}

	if o.Messenger == "" {
		o.Messenger = "email"
	}

	// Validate.
	if c, err := a.validateCampaignFields(o); err != nil {
		a.log.Printf("create campaign: validation failed name=%q error=%v", o.Name, err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	} else {
		o = c
	}
	a.log.Printf("create campaign: validated name=%q type=%q content_type=%q archive=%v altbody_valid=%v archive_slug_valid=%v send_at_valid=%v",
		o.Name, o.Type, o.ContentType, o.Archive, o.AltBody.Valid, o.ArchiveSlug.Valid, o.SendAt.Valid)

	if o.ArchiveTemplateID.Valid && strings.TrimSpace(o.ArchiveTemplateID.String) != "" {
		o.ArchiveTemplateID = o.TemplateID
	}

	out, err := a.core.CreateCampaign(o.Campaign, o.ListIDs, o.MediaIDs)
	if err != nil {
		a.log.Printf("create campaign: core create failed name=%q error=%v", o.Name, err)
		return err
	}
	out, err = a.syncCampaignDripAutomation(out, o.DripEnabled, campaignDripMetadata{})
	if err != nil {
		return err
	}
	a.log.Printf("create campaign: success name=%q campaign_record_id=%q uuid=%q", out.Name, out.RecordID, out.UUID)

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateCampaign handles campaign modification.
// Campaigns that are done cannot be modified.
func (a *App) UpdateCampaign(c echo.Context) error {
	// Get the campaign ID.
	recordID, err := a.resolveCampaignRouteID(c)
	if err != nil {
		return err
	}

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeManage, recordID, c); err != nil {
		return err
	}

	// Retrieve the campaign from the DB.
	cm, err := a.core.GetCampaign(recordID, "", "")
	if err != nil {
		return err
	}
	existingDripMeta := parseCampaignDripMetadata(cm.Attribs)
	user := auth.GetUser(c)

	if !canEditCampaign(cm.Status) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.cantUpdate"))
	}

	// Clear attribs to avoid merging old and new values as json.Unmarshal in JSON.scan() merges maps,
	// merging values already in the DB and incoming values. If this is nil, then DB values remain
	// unchanged.
	cm.Attribs = nil

	// Read the incoming params into the existing campaign fields from the DB.
	// This allows updating of values that have been sent whereas fields
	// that are not in the request retain the old values.
	o := campReq{Campaign: cm}
	if err := bindCampaignReq(c, &o); err != nil {
		return err
	}
	a.log.Printf("update campaign: received record_id=%q name=%q status=%q content_type=%q list_ids=%v media_ids=%v archive=%v",
		recordID, o.Name, cm.Status, o.ContentType, o.ListIDs, o.MediaIDs, o.Archive)

	filteredListRecordIDs := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, o.ListRecordIDs)
	o.ListIDs, err = a.core.ResolveListIDs(nil, filteredListRecordIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}

	if c, err := a.validateCampaignFields(o); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	} else {
		o = c
	}

	out, err := a.core.UpdateCampaign(recordID, o.Campaign, o.ListIDs, o.MediaIDs)
	if err != nil {
		return err
	}
	out, err = a.syncCampaignDripAutomation(out, o.DripEnabled, existingDripMeta)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateCampaignStatus handles campaign status modification.
func (a *App) UpdateCampaignStatus(c echo.Context) error {
	// Get the campaign ID.
	recordID, err := a.resolveCampaignRouteID(c)
	if err != nil {
		return err
	}

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeManage, recordID, c); err != nil {
		return err
	}

	req := struct {
		Status string `json:"status"`
	}{}
	if err := c.Bind(&req); err != nil {
		return err
	}

	// Update the campaign status in the DB.
	a.log.Printf("change campaign status: request record_id=%q target_status=%q", recordID, req.Status)
	out, err := a.core.UpdateCampaignStatus(recordID, req.Status)
	if err != nil {
		a.log.Printf("change campaign status: failed record_id=%q target_status=%q error=%v", recordID, req.Status, err)
		return err
	}
	a.log.Printf("change campaign status: success record_id=%q new_status=%q", recordID, out.Status)

	// If the campaign is being stopped, send the signal to the manager to stop it in flight.
	if req.Status == models.CampaignStatusPaused || req.Status == models.CampaignStatusCancelled {
		a.log.Printf("change campaign status: stop signal record_id=%q rowid=%d target_status=%q", recordID, out.ID, req.Status)
		a.manager.StopCampaign(out.ID)
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateCampaignArchive handles campaign status modification.
func (a *App) UpdateCampaignArchive(c echo.Context) error {
	recordID, err := a.resolveCampaignRouteID(c)
	if err != nil {
		return err
	}

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeManage, recordID, c); err != nil {
		return err
	}

	req := struct {
		Archive     bool        `json:"archive"`
		TemplateID  string      `json:"archive_template_id"`
		Meta        models.JSON `json:"archive_meta"`
		ArchiveSlug string      `json:"archive_slug"`
	}{}
	if err := c.Bind(&req); err != nil {
		return err
	}

	if req.ArchiveSlug != "" {
		// Format the slug to be alpha-numeric-dash.
		s := strings.ToLower(req.ArchiveSlug)
		s = strings.TrimSpace(reSlug.ReplaceAllString(s, " "))
		s = regexpSpaces.ReplaceAllString(s, "-")
		req.ArchiveSlug = s
	}

	if err := a.core.UpdateCampaignArchive(recordID, req.Archive, req.TemplateID, req.Meta, req.ArchiveSlug); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{req})
}

// DeleteCampaign handles campaign deletion.
// Only scheduled campaigns that have not started yet can be deleted.
func (a *App) DeleteCampaign(c echo.Context) error {
	// Get the campaign ID.
	recordID, err := a.resolveCampaignRouteID(c)
	if err != nil {
		return err
	}

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeManage, recordID, c); err != nil {
		return err
	}

	// Delete the campaign from the DB.
	if err := a.core.DeleteCampaign(recordID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// DeleteCampaigns deletes multiple campaigns by IDs or by query.
func (a *App) DeleteCampaigns(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	var (
		hasAllPerm     = user.HasPerm(auth.PermCampaignsManageAll)
		permittedLists []int
		err            error
	)

	if !hasAllPerm {
		// Either the user has campaigns:manage_all permissions and can manage all campaigns,
		// or the campaigns are filtered by the lists the user has get|manage access to.
		permittedRecordIDs := []string{}
		hasAllPerm, permittedRecordIDs = user.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage)
		permittedLists, err = a.core.ResolveListIDs(nil, permittedRecordIDs)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
		}
	}

	var (
		recordIDs []string
		query     string
		all       bool
	)

	// Check for IDs in query params.
	if len(c.Request().URL.Query()["record_id"]) > 0 {
		recordIDs = getQueryStrings("record_id", c.Request().URL.Query())
	} else {
		// Check for query param.
		query = strings.TrimSpace(c.FormValue("query"))
		all = c.FormValue("all") == "true"
	}

	// Validate that either IDs or query is provided.
	if len(recordIDs) == 0 && (query == "" && !all) {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", "record_id or query required"))
	}

	// Delete the campaigns from the DB.
	if err := a.core.DeleteCampaigns(recordIDs, query, hasAllPerm, permittedLists); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// GetRunningCampaignStats returns stats of a given set of campaign IDs.
func (a *App) GetRunningCampaignStats(c echo.Context) error {
	// Get the running campaign stats from the DB.
	out, err := a.core.GetRunningCampaignStats()
	if err != nil {
		return err
	}

	if len(out) == 0 {
		return c.JSON(http.StatusOK, okResp{[]struct{}{}})
	}

	// Compute rate.
	for i, c := range out {
		if c.Started.Valid && c.UpdatedAt.Valid {
			diff := max(int(c.UpdatedAt.Time.Sub(c.Started.Time).Minutes()), 1)

			rate := c.Sent / diff
			if rate > c.Sent || rate > c.ToSend {
				rate = c.Sent
			}

			// Rate since the starting of the campaign.
			out[i].NetRate = rate

			// Realtime running rate over the last minute.
			out[i].Rate = a.manager.GetCampaignStats(c.ID).SendRate
		}
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// TestCampaign handles the sending of a campaign message to
// arbitrary subscribers for testing.
func (a *App) TestCampaign(c echo.Context) error {
	// Get the campaign ID.
	recordID, err := a.resolveCampaignRouteID(c)
	if err != nil {
		return err
	}

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeManage, recordID, c); err != nil {
		return err
	}

	// Get and validate fields.
	var req campReq
	if err := bindCampaignReq(c, &req); err != nil {
		return err
	}

	// Test requests from the UI don't post list IDs. Reuse the campaign's
	// saved list associations so regular validation and template rendering
	// continue to work for test sends.
	if len(req.ListIDs) == 0 && len(req.ListRecordIDs) == 0 {
		camp, err := a.core.GetCampaign(recordID, "", "")
		if err != nil {
			return err
		}

		req.ListRecordIDs = campaignListRecordIDs(camp.Lists)
		req.ListIDs, err = a.core.ResolveListIDs(nil, req.ListRecordIDs)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
		}
	}

	// Validate.
	if c, err := a.validateCampaignFieldsForTest(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	} else {
		req = c
	}
	if len(req.SubscriberEmails) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.noSubsToTest"))
	}

	// Sanitize subscriber e-mails.
	for i := range req.SubscriberEmails {
		req.SubscriberEmails[i] = strings.ToLower(strings.TrimSpace(req.SubscriberEmails[i]))
	}

	// Get the subscribers from the DB by their e-mails.
	subs, err := a.core.GetSubscribersByEmail(req.SubscriberEmails)
	if err != nil {
		return err
	}

	// Get the campaign from the DB for previewing.
	tplID := strings.TrimSpace(c.FormValue("template_id"))
	camp, err := a.core.GetCampaignForPreview(recordID, tplID)
	if err != nil {
		return err
	}

	// Override certain values from the DB with incoming values.
	camp.Name = req.Name
	camp.Subject = req.Subject
	camp.FromEmail = req.FromEmail
	camp.Body = req.Body
	camp.AltBody = req.AltBody
	camp.Messenger = req.Messenger
	camp.ContentType = req.ContentType
	camp.Headers = req.Headers
	camp.TemplateID = req.TemplateID
	for _, id := range req.MediaIDs {
		if id > 0 {
			camp.MediaIDs = append(camp.MediaIDs, int64(id))
		}
	}

	// Send the test messages.
	for _, s := range subs {
		sub := s

		if err := a.sendTestMessage(sub, &camp); err != nil {
			a.log.Printf("error sending test message: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				a.i18n.Ts("campaigns.errorSendTest", "error", err.Error()))
		}
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// GetCampaignViewAnalytics retrieves view counts for a campaign.
func (a *App) GetCampaignViewAnalytics(c echo.Context) error {
	recordIDs := getQueryStrings("id", c.Request().URL.Query())
	ids, err := a.core.ResolveCampaignIDs(nil, recordIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}

	if len(ids) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.missingFields", "name", "`id`"))
	}

	var (
		typ  = c.Param("type")
		from = c.QueryParams().Get("from")
		to   = c.QueryParams().Get("to")
	)
	if typ == "" {
		path := strings.Trim(c.Request().URL.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) > 0 {
			typ = parts[len(parts)-1]
		}
	}
	if !strHasLen(from, 10, 30) || !strHasLen(to, 10, 30) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("analytics.invalidDates"))
	}

	// Campaign link stats.
	if typ == "links" {
		out, err := a.core.GetCampaignAnalyticsLinks(ids, typ, from, to)
		if err != nil {
			return err
		}

		return c.JSON(http.StatusOK, okResp{out})
	}

	// Get the analytics numbers from the DB for the campaigns.
	out, err := a.core.GetCampaignAnalyticsCounts(ids, typ, from, to)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// sendTestMessage takes a campaign and a subscriber and sends out a sample campaign message.
func (a *App) sendTestMessage(sub models.Subscriber, camp *models.Campaign) error {
	if err := camp.CompileTemplate(a.manager.TemplateFuncs(camp)); err != nil {
		a.log.Printf("error compiling template: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			a.i18n.Ts("templates.errorCompiling", "error", err.Error()))
	}

	// Create a sample campaign message.
	msg, err := a.manager.NewCampaignMessage(camp, sub)
	if err != nil {
		a.log.Printf("error rendering message: %v", err)
		return echo.NewHTTPError(http.StatusNotFound, a.i18n.Ts("templates.errorRendering", "error", err.Error()))
	}

	return a.manager.PushCampaignMessage(msg)
}

func (a *App) defaultCampaignFromEmail(messenger string) string {
	settings, err := a.core.GetSettings()
	if err != nil {
		return a.cfg.FromEmail
	}

	enabled := make([]models.SMTPSettings, 0, len(settings.SMTP))
	for _, item := range settings.SMTP {
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}

	if messenger != "" && messenger != emailMsgr {
		for _, item := range enabled {
			if item.Name == messenger && item.DefaultFromEmail != "" {
				return item.DefaultFromEmail
			}
		}
	}

	if (messenger == "" || messenger == emailMsgr) && len(enabled) == 1 && enabled[0].DefaultFromEmail != "" {
		return enabled[0].DefaultFromEmail
	}

	return a.cfg.FromEmail
}

// validateCampaignFields validates incoming campaign field values.
func (a *App) validateCampaignFields(c campReq) (campReq, error) {
	if c.FromEmail == "" {
		c.FromEmail = a.defaultCampaignFromEmail(c.Messenger)
	} else {
		sanitized, err := a.sanitizeFromAddress(c.FromEmail)
		if err != nil {
			return c, err
		}
		c.FromEmail = sanitized
	}

	if !strHasLen(c.Name, 1, stdInputMaxLen) {
		return c, errors.New(a.i18n.T("campaigns.fieldInvalidName"))
	}

	// Larger char limit for subject as it can contain {{ go templating }} logic.
	if !strHasLen(c.Subject, 1, 5000) {
		return c, errors.New(a.i18n.T("campaigns.fieldInvalidSubject"))
	}

	// If no content-type is specified, default to richtext.
	if c.ContentType != models.CampaignContentTypeRichtext &&
		c.ContentType != models.CampaignContentTypeHTML &&
		c.ContentType != models.CampaignContentTypePlain &&
		c.ContentType != models.CampaignContentTypeVisual &&
		c.ContentType != models.CampaignContentTypeMarkdown {
		c.ContentType = models.CampaignContentTypeRichtext
	}

	if c.ContentType != models.CampaignContentTypeVisual {
		c.BodySource.Valid = false
	}

	// If there's a "send_at" date, it should be in the future.
	if c.SendAt.Valid {
		if c.SendAt.Time.Before(time.Now()) {
			return c, errors.New(a.i18n.T("campaigns.fieldInvalidSendAt"))
		}
	}

	if len(c.ListIDs) == 0 {
		return c, errors.New(a.i18n.T("campaigns.fieldInvalidListIDs"))
	}

	if !a.manager.HasMessenger(c.Messenger) {
		// If it's a specific SMTP, but it's no longer available (removed/disabled), fall back to general email messenger.
		if strings.HasPrefix(c.Messenger, "email-") {
			c.Messenger = "email"
		} else {
			return c, errors.New(a.i18n.Ts("campaigns.fieldInvalidMessenger", "name", c.Messenger))
		}
	}

	camp := models.Campaign{Body: c.Body, TemplateBody: tplTag}
	if err := c.CompileTemplate(a.manager.TemplateFuncs(&camp)); err != nil {
		return c, errors.New(a.i18n.Ts("campaigns.fieldInvalidBody", "error", err.Error()))
	}

	if len(c.Headers) == 0 {
		c.Headers = make([]map[string]string, 0)
	}

	// Validate and initialize attribs.
	if c.Attribs != nil {
		if _, err := json.Marshal(c.Attribs); err != nil {
			return c, errors.New(a.i18n.T("subscribers.invalidJSON"))
		}
	}

	batching := c.Batching.toModel()
	if batching.Enabled {
		if batching.BatchSize < 1 {
			return c, errors.New("batch size must be greater than 0")
		}
		if batching.RepeatValue < 1 {
			return c, errors.New("batch repeat value must be greater than 0")
		}
		if batching.RepeatUnit != "hours" && batching.RepeatUnit != "days" {
			return c, errors.New("batch repeat unit must be hours or days")
		}
	}

	c.Attribs = models.MergeCampaignBatching(c.Attribs, batching)

	if len(c.ArchiveMeta) == 0 {
		c.ArchiveMeta = json.RawMessage("{}")
	}

	if c.ArchiveSlug.String != "" {
		// Format the slug to be alpha-numeric-dash.
		s := strings.ToLower(c.ArchiveSlug.String)
		s = strings.TrimSpace(reSlug.ReplaceAllString(s, " "))
		s = regexpSpaces.ReplaceAllString(s, "-")

		c.ArchiveSlug = null.NewString(s, true)
	} else {
		// If there's no slug set, set it to NULL in the DB.
		c.ArchiveSlug.Valid = false
	}

	return c, nil
}

func (a *App) validateCampaignFieldsForTest(c campReq) (campReq, error) {
	if c.FromEmail == "" {
		c.FromEmail = a.defaultCampaignFromEmail(c.Messenger)
	} else {
		sanitized, err := a.sanitizeFromAddress(c.FromEmail)
		if err != nil {
			return c, err
		}
		c.FromEmail = sanitized
	}

	if !strHasLen(c.Name, 1, stdInputMaxLen) {
		return c, errors.New(a.i18n.T("campaigns.fieldInvalidName"))
	}

	if !strHasLen(c.Subject, 1, 5000) {
		return c, errors.New(a.i18n.T("campaigns.fieldInvalidSubject"))
	}

	if c.ContentType != models.CampaignContentTypeRichtext &&
		c.ContentType != models.CampaignContentTypeHTML &&
		c.ContentType != models.CampaignContentTypePlain &&
		c.ContentType != models.CampaignContentTypeVisual &&
		c.ContentType != models.CampaignContentTypeMarkdown {
		c.ContentType = models.CampaignContentTypeRichtext
	}

	if c.ContentType != models.CampaignContentTypeVisual {
		c.BodySource.Valid = false
	}

	if !a.manager.HasMessenger(c.Messenger) {
		if strings.HasPrefix(c.Messenger, "email-") {
			c.Messenger = "email"
		} else {
			return c, errors.New(a.i18n.Ts("campaigns.fieldInvalidMessenger", "name", c.Messenger))
		}
	}

	camp := models.Campaign{Body: c.Body, TemplateBody: tplTag}
	if err := c.CompileTemplate(a.manager.TemplateFuncs(&camp)); err != nil {
		return c, errors.New(a.i18n.Ts("campaigns.fieldInvalidBody", "error", err.Error()))
	}

	if len(c.Headers) == 0 {
		c.Headers = make([]map[string]string, 0)
	}

	if c.Attribs != nil {
		if _, err := json.Marshal(c.Attribs); err != nil {
			return c, errors.New(a.i18n.T("subscribers.invalidJSON"))
		}
	}

	c.Attribs = models.MergeCampaignBatching(c.Attribs, c.Batching.toModel())

	if len(c.ArchiveMeta) == 0 {
		c.ArchiveMeta = json.RawMessage("{}")
	}

	return c, nil
}

// makeOptinCampaignMessage makes a default opt-in campaign message body.
func (a *App) makeOptinCampaignMessage(o campReq) (campReq, error) {
	if len(o.ListIDs) == 0 {
		return o, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.fieldInvalidListIDs"))
	}

	// Fetch double opt-in lists from the given list IDs from the DB.
	lists, err := a.core.GetListsByOptin(o.ListIDs, models.ListOptinDouble)
	if err != nil {
		return o, err
	}

	// There are no double opt-in lists.
	if len(lists) == 0 {
		return o, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.noOptinLists"))
	}

	// Construct the opt-in URL with list IDs.
	listIDs := url.Values{}
	for _, l := range lists {
		listIDs.Add("l", l.UUID)
	}
	// optinURLFunc := template.URL("{{ OptinURL }}?" + listIDs.Encode())
	optinURLAttr := template.HTMLAttr(fmt.Sprintf(`href="{{ OptinURL }}%s"`, listIDs.Encode()))

	// Prepare sample opt-in message for the campaign.
	var b bytes.Buffer

	if err := notifs.Tpls.ExecuteTemplate(&b, "optin-campaign", struct {
		Lists        []models.List
		OptinURLAttr template.HTMLAttr
	}{lists, optinURLAttr}); err != nil {
		a.log.Printf("error compiling 'optin-campaign' template: %v", err)
		return o, echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("templates.errorCompiling", "error", err.Error()))
	}

	o.Body = b.String()
	return o, nil
}

// checkCampaignPerm checks if the user has get or manage access to the given campaign.
// Either the user has blanket get_all/manage_all permissions, or the campaign
// belongs to lists that the user has access to.
func (a *App) checkCampaignPerm(types auth.PermType, recordID string, c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	perm := auth.PermCampaignsGet
	if types&auth.PermTypeGet != 0 {
		// It's a get request and there's a blanket get all permission.
		if user.HasPerm(auth.PermCampaignsGetAll) {
			return nil
		}
	} else {
		// It's a manage request and there's a blanket manage_all permission.
		if user.HasPerm(auth.PermCampaignsManageAll) {
			return nil
		}

		perm = auth.PermCampaignsManage
	}

	// There are no *_all campaign permissions. Instead, check if the user access
	// blanket get_all/manage_all list permissions. If yes, then the user can access
	// all campaigns. If there are no *_all permissions, then ensure that the
	// campaign belongs to the lists that the user has access to.
	if hasAllPerm, permittedListRecordIDs := user.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage); !hasAllPerm {
		permittedListIDs, err := a.core.ResolveListIDs(nil, permittedListRecordIDs)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
		}
		if ok, err := a.core.CampaignHasLists(recordID, permittedListIDs); err != nil {
			return err
		} else if !ok {
			return echo.NewHTTPError(http.StatusForbidden,
				a.i18n.Ts("globals.messages.permissionDenied", "name", perm))
		}
	}

	return nil
}

// canEditCampaign returns true if a campaign is in a status where updating
// its properties is allowed.
func canEditCampaign(status string) bool {
	return status == models.CampaignStatusDraft ||
		status == models.CampaignStatusPaused ||
		status == models.CampaignStatusScheduled
}
