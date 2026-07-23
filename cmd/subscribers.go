package main

// Subscriber HTTP routes and subscriber_record_id query/body fields use PocketBase
// record ids (SQLite subscribers.id), never numeric rowids.

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/compdani/list_pocket/internal/apperr"
	pbcore "github.com/pocketbase/pocketbase/core"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"

	"github.com/compdani/list_pocket/internal/auth"
	corepkg "github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/notifs"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/internal/subimporter"
	"github.com/compdani/list_pocket/internal/utils"
	"github.com/compdani/list_pocket/models"
)

const dummyUUID = models.DummyUUID

// subQueryReq is a "catch all" struct for reading various
// subscriber related requests.
type subQueryReq struct {
	Search              string          `json:"search"`
	Query               string          `json:"query"`
	Filters             json.RawMessage `json:"filters"`
	ListIDs             []int           `json:"list_ids"`
	ListRecordIDs       []string        `json:"list_record_ids"`
	TargetListIDs       []int           `json:"target_list_ids"`
	TargetListRecordIDs []string        `json:"target_list_record_ids"`
	SubscriberRecordIDs []string        `json:"subscriber_record_ids"`
	Action              string          `json:"action"`
	Status              string          `json:"status"`
	SubscriptionStatus  string          `json:"subscription_status"`
	All                 bool            `json:"all"`
}

func parseFiltersParam(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	return json.RawMessage(raw)
}

func hasSubscriberFilters(filters json.RawMessage) bool {
	f, err := corepkg.ParseSubscriberFilters(filters)
	return err == nil && f != nil
}

// subOptin contains the data that's passed to the double opt-in e-mail template.
type subOptin struct {
	models.Subscriber

	OptinURL string
	UnsubURL string
	Lists    []models.List
}

var (
	dummySubscriber = models.Subscriber{
		Base:      models.Base{RecordID: models.PreviewTrackingRecordID},
		Email:     "demo@listpocket.app",
		Phone:     "+15555550123",
		FirstName: "Demo",
		LastName:  "Subscriber",
		Name:      "Demo Subscriber",
		UUID:      dummyUUID,
		Attribs:   models.JSON{"city": "Bengaluru"},
	}
)

func (a *App) resolveSubscriberRouteID(re *pbcore.RequestEvent) (int, error) {
	rawID := strings.TrimSpace(pathParam(re, "id"))
	if rawID == "" {
		return 0, apperr.BadRequest("invalid ID")
	}

	ids, err := a.core.ResolveSubscriberIDs(nil, []string{rawID})
	if err != nil {
		return 0, apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if len(ids) != 1 || ids[0] < 1 {
		return 0, apperr.BadRequest("invalid ID")
	}

	return ids[0], nil
}

// GetSubscriber handles the retrieval of a single subscriber by ID.
func (a *App) GetSubscriber(re *pbcore.RequestEvent) error {
	user := auth.GetUserRE(re)

	// Check if the user has access to at least one of the lists on the subscriber.
	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}
	if err := a.hasSubPerm(user, []int{id}); err != nil {
		return err
	}

	// Fetch the subscriber from the DB.
	out, err := a.core.GetSubscriber(id, "", "")
	if err != nil {
		return err
	}

	return okJSON(re, out)
}

// GetSubscriberActivity handles campaign sends (ledger), views, and link clicks for the Activity tab.
func (a *App) GetSubscriberActivity(re *pbcore.RequestEvent) error {
	user := auth.GetUserRE(re)

	// Check if the user has access to at least one of the lists on the subscriber.
	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}
	if err := a.hasSubPerm(user, []int{id}); err != nil {
		return err
	}

	// Fetch the subscriber activity from the DB.
	out, err := a.core.GetSubscriberActivity(id)
	if err != nil {
		return err
	}

	return okJSON(re, out)
}

// GetSubscriberTimeline returns merged outbound and inbound timeline events for a subscriber.
func (a *App) GetSubscriberTimeline(re *pbcore.RequestEvent) error {
	user := auth.GetUserRE(re)

	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}
	if err := a.hasSubPerm(user, []int{id}); err != nil {
		return err
	}

	limit := 50
	if raw := strings.TrimSpace(queryParam(re, "limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			limit = v
		}
	}
	offset := 0
	if raw := strings.TrimSpace(queryParam(re, "offset")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			offset = v
		}
	}
	sortOrder := strings.TrimSpace(queryParam(re, "sort"))

	eventTypes := []string{}
	for _, raw := range re.Request.URL.Query()["event_type"] {
		if v := strings.TrimSpace(raw); v != "" {
			eventTypes = append(eventTypes, v)
		}
	}
	if len(eventTypes) == 0 {
		for _, raw := range strings.Split(strings.TrimSpace(queryParam(re, "event_types")), ",") {
			if v := strings.TrimSpace(raw); v != "" {
				eventTypes = append(eventTypes, v)
			}
		}
	}

	out, err := a.core.GetUnifiedContactTimeline(re.Request.Context(), corepkg.TimelineQueryParams{
		SubscriberID: id,
		Limit:        limit,
		Offset:       offset,
		SortOrder:    sortOrder,
		EventTypes:   eventTypes,
	})
	if err != nil {
		return err
	}

	return okJSON(re, out)
}

// QuerySubscribers handles querying subscribers based on an arbitrary SQL expression.
func (a *App) QuerySubscribers(re *pbcore.RequestEvent) error {
	// Get the authenticated user.
	user := auth.GetUserRE(re)

	// Filter list IDs by permission.
	listIDs, err := a.filterListQueryByPerm("list_record_id", re.Request.URL.Query(), user)
	if err != nil {
		return err
	}

	// Does the user have the subscribers:sql_query permission?
	query := formatSQLExp(re.Request.FormValue("query"))
	if query != "" {
		if !user.HasPerm(auth.PermSubscribersSqlQuery) {
			return apperr.Forbidden(a.i18n.Ts("globals.messages.permissionDenied", "name", auth.PermSubscribersSqlQuery))
		}
	}

	var (
		searchStr = strings.TrimSpace(re.Request.FormValue("search"))
		filters   = parseFiltersParam(re.Request.FormValue("filters"))
		subStatus = re.Request.FormValue("subscription_status")
		order     = re.Request.FormValue("order")
		orderBy   = re.Request.FormValue("order_by")
		pg        = a.pg.NewFromURL(re.Request.URL.Query())
	)
	a.log.Printf("query subscribers: username=%q role_id=%d search=%q query=%q has_filters=%v list_ids=%v sub_status=%q order_by=%q order=%q page=%d per_page=%d offset=%d limit=%d",
		user.Username, user.UserRoleID, searchStr, query, hasSubscriberFilters(filters), listIDs, subStatus, orderBy, order, pg.Page, pg.PerPage, pg.Offset, pg.Limit)

	// Query subscribers from the DB.
	res, total, err := a.core.QuerySubscribers(searchStr, query, filters, listIDs, subStatus, order, orderBy, pg.Offset, pg.Limit)
	if err != nil {
		a.log.Printf("query subscribers: failed username=%q error=%v", user.Username, err)
		return err
	}
	a.log.Printf("query subscribers: success username=%q total=%d results=%d", user.Username, total, len(res))

	out := models.PageResults{
		Query:   query,
		Search:  searchStr,
		Results: res,
		Total:   total,
		Page:    pg.Page,
		PerPage: pg.PerPage,
	}

	return okJSON(re, out)
}

// ExportSubscribers handles querying subscribers based on an arbitrary SQL expression.
func (a *App) ExportSubscribers(re *pbcore.RequestEvent) error {
	// Get the authenticated user.
	user := auth.GetUserRE(re)

	// Filter list IDs by permission.
	listIDs, err := a.filterListQueryByPerm("list_record_id", re.Request.URL.Query(), user)
	if err != nil {
		return err
	}

	// Export only specific subscribers (PocketBase record ids).
	var subIDs []int
	if recordIDs := re.Request.URL.Query()["subscriber_record_id"]; len(recordIDs) > 0 {
		var err error
		subIDs, err = a.core.ResolveSubscriberIDs(nil, recordIDs)
		if err != nil {
			return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
		}
	}

	// Filter by subscription status
	subStatus := queryParam(re, "subscription_status")

	// Does the user have the subscribers:sql_query permission?
	var (
		searchStr = strings.TrimSpace(re.Request.FormValue("search"))
		query     = formatSQLExp(re.Request.FormValue("query"))
		filters   = parseFiltersParam(re.Request.FormValue("filters"))
	)
	if query != "" {
		if !user.HasPerm(auth.PermSubscribersSqlQuery) {
			return apperr.Forbidden(a.i18n.Ts("globals.messages.permissionDenied", "name", auth.PermSubscribersSqlQuery))
		}
	}

	// Get the batched export iterator.
	exp, err := a.core.ExportSubscribers(searchStr, query, filters, subIDs, listIDs, subStatus, a.cfg.DBBatchSize)
	if err != nil {
		return err
	}

	var (
		hdr = re.Response.Header()
		wr  = csv.NewWriter(re.Response)
	)

	hdr.Set("Content-Type", "application/octet-stream")
	hdr.Set("Content-type", "text/csv")
	hdr.Set("Content-Disposition", "attachment; filename="+"subscribers.csv")
	hdr.Set("Content-Transfer-Encoding", "binary")
	hdr.Set("Cache-Control", "no-cache")
	wr.Write([]string{"uuid", "email", "phone", "first_name", "last_name", "name", "attributes", "status", "created_at", "updated_at"})

loop:
	// Iterate in batches until there are no more subscribers to export.
	for {
		out, err := exp()
		if err != nil {
			return err
		}
		if len(out) == 0 {
			break
		}

		for _, r := range out {
			if err = wr.Write([]string{r.UUID, r.Email, r.Phone, r.FirstName, r.LastName, r.Name, r.Attribs, r.Status,
				r.CreatedAt.Time.String(), r.UpdatedAt.Time.String()}); err != nil {
				a.log.Printf("error streaming CSV export: %v", err)
				break loop
			}
		}

		// Flush CSV to stream after each batch.
		wr.Flush()
	}

	return nil
}

// CreateSubscriber handles the creation of a new subscriber.
func (a *App) CreateSubscriber(re *pbcore.RequestEvent) error {
	// Get the authenticated user.
	user := auth.GetUserRE(re)

	// Get and validate fields.
	var req subimporter.SubReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}

	// Validate fields.
	req, err := a.importer.ValidateFields(req)
	if err != nil {
		return apperr.BadRequest(err.Error())
	}

	// Filter lists against the current user's permitted lists.
	listIDs, err := a.core.ResolveListIDs(req.Lists, req.ListRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	filteredListRecordIDs := user.FilterListsByPerm(auth.PermTypeManage, req.ListRecordIDs)
	listIDs, err = a.core.ResolveListIDs(nil, filteredListRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	a.log.Printf("create subscriber: email=%q phone=%q first_name=%q last_name=%q name=%q requested_lists=%v permitted_lists=%v preconfirm=%v",
		req.Email, req.Phone, req.FirstName, req.LastName, req.Name, req.Lists, listIDs, req.PreconfirmSubs)

	// Insert the subscriber into the DB.
	sub, _, err := a.core.InsertSubscriber(req.Subscriber, listIDs, nil, req.PreconfirmSubs, false)
	if err != nil {
		a.log.Printf("create subscriber: insert failed email=%q error=%v", req.Email, err)
		return err
	}
	a.log.Printf("create subscriber: success email=%q subscriber_record_id=%q uuid=%q", sub.Email, sub.RecordID, sub.UUID)

	return okJSON(re, sub)
}

// UpdateSubscriber handles modification of a subscriber.
func (a *App) UpdateSubscriber(re *pbcore.RequestEvent) error {
	// Get the authenticated user.
	user := auth.GetUserRE(re)

	// Get and validate fields.
	req := struct {
		models.Subscriber
		Lists          []int    `json:"lists"`
		ListRecordIDs  []string `json:"list_record_ids"`
		PreconfirmSubs bool     `json:"preconfirm_subscriptions"`
	}{}
	if err := bindJSON(re, &req); err != nil {
		return err
	}

	// Sanitize and validate the email field.
	if em, err := a.importer.SanitizeEmail(req.Email); err != nil {
		return apperr.BadRequest(err.Error())
	} else {
		req.Email = em
	}
	req.Phone = utils.NormalizePhone(req.Phone)
	if req.Phone != "" && !strHasLen(req.Phone, 1, 64) {
		return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
	}

	req.Subscriber.NormalizeName()
	if req.FirstName != "" && !strHasLen(req.FirstName, 1, stdInputMaxLen) {
		return apperr.BadRequest(a.i18n.T("subscribers.invalidName"))
	}
	if req.LastName != "" && !strHasLen(req.LastName, 1, stdInputMaxLen) {
		return apperr.BadRequest(a.i18n.T("subscribers.invalidName"))
	}
	if req.Name != "" && !strHasLen(req.Name, 1, stdInputMaxLen) {
		return apperr.BadRequest(a.i18n.T("subscribers.invalidName"))
	}

	// Filter lists against the current user's permitted lists.
	listIDs, err := a.core.ResolveListIDs(req.Lists, req.ListRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	filteredListRecordIDs := user.FilterListsByPerm(auth.PermTypeManage, req.ListRecordIDs)
	listIDs, err = a.core.ResolveListIDs(nil, filteredListRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}

	// Update the subscriber in the DB by PocketBase record id from the path.
	recordID := strings.TrimSpace(pathParam(re, "id"))
	if recordID == "" {
		return apperr.BadRequest("invalid ID")
	}
	out, _, err := a.core.UpdateSubscriberWithLists(recordID, req.Subscriber, listIDs, nil, req.PreconfirmSubs, true, false)
	if err != nil {
		return err
	}

	return okJSON(re, out)
}

// SubscriberSendOptin sends an optin confirmation e-mail to a subscriber.
func (a *App) SubscriberSendOptin(re *pbcore.RequestEvent) error {
	// Fetch the subscriber.
	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}
	out, err := a.core.GetSubscriber(id, "", "")
	if err != nil {
		return err
	}

	// Trigger the opt-in confirmation e-mail hook.
	if _, err := a.fnOptinNotify(out, nil); err != nil {
		return apperr.Internal(a.i18n.T("subscribers.errorSendingOptin"))
	}

	return okJSON(re, true)
}

// SubscriberSMSOptOut marks a subscriber's phone as SMS-unsubscribed across
// every list they're on. Used both from the admin UI ("this phone is not
// SMS-reachable") and indirectly as the same action our STOP webhook takes.
func (a *App) SubscriberSMSOptOut(re *pbcore.RequestEvent) error {
	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}

	sub, err := a.core.GetSubscriber(id, "", "")
	if err != nil {
		return err
	}
	phone := strings.TrimSpace(sub.Phone)
	if phone == "" {
		return apperr.BadRequest("subscriber has no phone to opt out")
	}

	n, err := a.core.SMSOptOutSubscriberByPhone(phone)
	if err != nil {
		return apperr.Internal(err.Error())
	}

	return okJSON(re, map[string]any{
		"updated": n,
		"phone":   phone,
	})
}

// BlocklistSubscriber handles the blocklisting of a given subscriber.
func (a *App) BlocklistSubscriber(re *pbcore.RequestEvent) error {
	user := auth.GetUserRE(re)
	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}
	if err := a.hasSubPerm(user, []int{id}); err != nil {
		return err
	}
	recordID := strings.TrimSpace(pathParam(re, "id"))
	if err := a.core.BlocklistSubscribers([]string{recordID}); err != nil {
		return err
	}

	return okJSON(re, true)
}

// BlocklistSubscribers handles the blocklisting of one or more subscribers.
func (a *App) BlocklistSubscribers(re *pbcore.RequestEvent) error {
	var req subQueryReq
	if err := bindJSON(re, &req); err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	recordIDs := uniqueNonEmptyStrings(req.SubscriberRecordIDs)
	if len(recordIDs) == 0 {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", "subscriber_record_ids"))
	}

	// Update the subscribers in the DB.
	if err := a.core.BlocklistSubscribers(recordIDs); err != nil {
		return err
	}

	return okJSON(re, true)
}

// ManageSubscriberLists handles bulk addition or removal of subscribers
// from or to one or more target lists.
// It takes either a PocketBase record id in the URI, or subscriber_record_ids in the request body.
func (a *App) ManageSubscriberLists(re *pbcore.RequestEvent) error {
	// Get the authenticated user.
	user := auth.GetUserRE(re)

	var req subQueryReq
	if err := bindJSON(re, &req); err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}

	recordIDs := uniqueNonEmptyStrings(req.SubscriberRecordIDs)
	if pID := strings.TrimSpace(pathParam(re, "id")); pID != "" {
		recordIDs = uniqueNonEmptyStrings(append([]string{pID}, recordIDs...))
	}
	subIDs, err := a.core.ResolveSubscriberIDs(nil, recordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if len(subIDs) == 0 {
		return apperr.BadRequest(a.i18n.T("subscribers.errorNoIDs"))
	}
	targetListIDs, err := a.core.ResolveListIDs(req.TargetListIDs, req.TargetListRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if len(targetListIDs) == 0 {
		return apperr.BadRequest(a.i18n.T("subscribers.errorNoListsGiven"))
	}

	// Filter lists against the current user's permitted lists.
	listRecordIDs := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, req.TargetListRecordIDs)
	listIDs, err := a.core.ResolveListIDs(nil, listRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}

	// User doesn't have the required list permissions.
	if len(listIDs) == 0 {
		return apperr.Forbidden(a.i18n.Ts("globals.messages.permissionDenied", "name", "lists"))
	}

	// Run the action in the DB.
	switch req.Action {
	case "add":
		err = a.core.AddSubscriptions(subIDs, listIDs, req.Status)
	case "remove":
		err = a.core.DeleteSubscriptions(subIDs, listIDs)
	case "unsubscribe":
		err = a.core.UnsubscribeLists(subIDs, listIDs, nil)
	default:
		return apperr.BadRequest(a.i18n.T("subscribers.invalidAction"))
	}

	if err != nil {
		return err
	}

	return okJSON(re, true)
}

// DeleteSubscriber handles deletion of a single subscriber.
func (a *App) DeleteSubscriber(re *pbcore.RequestEvent) error {
	user := auth.GetUserRE(re)
	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}
	if err := a.hasSubPerm(user, []int{id}); err != nil {
		return err
	}
	recordID := strings.TrimSpace(pathParam(re, "id"))
	if err := a.core.DeleteSubscribers([]string{recordID}, nil); err != nil {
		return err
	}

	return okJSON(re, true)
}

// DeleteSubscribers handles bulk deletion of one or more subscribers.
func (a *App) DeleteSubscribers(re *pbcore.RequestEvent) error {
	recordIDs := uniqueNonEmptyStrings(re.Request.URL.Query()["subscriber_record_id"])
	if len(recordIDs) == 0 {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", "subscriber_record_id"))
	}

	if err := a.core.DeleteSubscribers(recordIDs, nil); err != nil {
		return err
	}

	return okJSON(re, true)
}

// DeleteSubscribersByQuery bulk deletes based on an
// arbitrary SQL expression.
func (a *App) DeleteSubscribersByQuery(re *pbcore.RequestEvent) error {
	// Get the authenticated user.
	user := auth.GetUserRE(re)

	var req subQueryReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}

	req.Search = strings.TrimSpace(req.Search)
	req.Query = formatSQLExp(req.Query)
	if req.All {
		// If the "all" flag is set, ignore any subquery that may be present.
		req.Search = ""
		req.Query = ""
		req.Filters = nil
	} else if req.Search == "" && req.Query == "" && !hasSubscriberFilters(req.Filters) {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "query"))
	}

	// Does the user have the subscribers:sql_query permission?
	if req.Query != "" {
		if !user.HasPerm(auth.PermSubscribersSqlQuery) {
			return apperr.Forbidden(a.i18n.Ts("globals.messages.permissionDenied", "name", auth.PermSubscribersSqlQuery))
		}
	}

	// Delete the subscribers from the DB.
	listIDs, err := a.core.ResolveListIDs(req.ListIDs, req.ListRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if err := a.core.DeleteSubscribersByQuery(req.Search, req.Query, req.Filters, listIDs, req.SubscriptionStatus); err != nil {
		return err
	}

	return okJSON(re, true)
}

// BlocklistSubscribersByQuery bulk blocklists subscribers
// based on an arbitrary SQL expression.
func (a *App) BlocklistSubscribersByQuery(re *pbcore.RequestEvent) error {
	// Get the authenticated user.
	user := auth.GetUserRE(re)

	var req subQueryReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}

	req.Search = strings.TrimSpace(req.Search)
	req.Query = formatSQLExp(req.Query)
	if req.All {
		// If the "all" flag is set, ignore any subquery that may be present.
		req.Search = ""
		req.Query = ""
		req.Filters = nil
	} else if req.Search == "" && req.Query == "" && !hasSubscriberFilters(req.Filters) {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "query"))
	}
	// Does the user have the subscribers:sql_query permission?
	if req.Query != "" {
		if !user.HasPerm(auth.PermSubscribersSqlQuery) {
			return apperr.Forbidden(a.i18n.Ts("globals.messages.permissionDenied", "name", auth.PermSubscribersSqlQuery))
		}
	}

	// Update the subscribers in the DB.
	listIDs, err := a.core.ResolveListIDs(req.ListIDs, req.ListRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if err := a.core.BlocklistSubscribersByQuery(req.Search, req.Query, req.Filters, listIDs, req.SubscriptionStatus); err != nil {
		return err
	}

	return okJSON(re, true)
}

// ManageSubscriberListsByQuery bulk adds/removes/unsubscribes subscribers
// from one or more lists based on an arbitrary SQL expression.
func (a *App) ManageSubscriberListsByQuery(re *pbcore.RequestEvent) error {
	// Get the authenticated user.
	user := auth.GetUserRE(re)

	var req subQueryReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}

	req.Search = strings.TrimSpace(req.Search)
	req.Query = formatSQLExp(req.Query)

	// Does the user have the subscribers:sql_query permission?
	if req.Query != "" {
		if !user.HasPerm(auth.PermSubscribersSqlQuery) {
			return apperr.Forbidden(a.i18n.Ts("globals.messages.permissionDenied", "name", auth.PermSubscribersSqlQuery))
		}
	}

	resolvedTargetListIDs, err := a.core.ResolveListIDs(req.TargetListIDs, req.TargetListRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if len(resolvedTargetListIDs) == 0 {
		return apperr.BadRequest(a.i18n.T("subscribers.errorNoListsGiven"))
	}

	// Filter lists against the current user's permitted lists.
	sourceListRecordIDs := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, req.ListRecordIDs)
	targetListRecordIDs := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, req.TargetListRecordIDs)
	sourceListIDs, err := a.core.ResolveListIDs(nil, sourceListRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	targetListIDs, err := a.core.ResolveListIDs(nil, targetListRecordIDs)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}

	// Run the action in the DB.
	affected := 0
	switch req.Action {
	case "add":
		affected, err = a.core.AddSubscriptionsByQuery(req.Search, req.Query, req.Filters, sourceListIDs, targetListIDs, req.Status, req.SubscriptionStatus)
	case "remove":
		affected, err = a.core.DeleteSubscriptionsByQuery(req.Search, req.Query, req.Filters, sourceListIDs, targetListIDs, req.SubscriptionStatus)
	case "unsubscribe":
		affected, err = a.core.UnsubscribeListsByQuery(req.Search, req.Query, req.Filters, sourceListIDs, targetListIDs, req.SubscriptionStatus)
	default:
		return apperr.BadRequest(a.i18n.T("subscribers.invalidAction"))
	}

	if err != nil {
		return err
	}

	return okJSON(re, map[string]any{
		"ok":             true,
		"affected_count": affected,
	})
}

// DeleteSubscriberBounces deletes all the bounces on a subscriber.
func (a *App) DeleteSubscriberBounces(re *pbcore.RequestEvent) error {
	// Delete the bounces from the DB.
	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}
	if err := a.core.DeleteSubscriberBounces(id, ""); err != nil {
		return err
	}

	return okJSON(re, true)
}

// ExportSubscriberData pulls the subscriber's profile,
// list subscriptions, campaign views and clicks and produces
// a JSON report. This is a privacy feature and depends on the
// configuration in a.Constants.Privacy.
func (a *App) ExportSubscriberData(re *pbcore.RequestEvent) error {
	// Get the subscriber's data. A single query that gets the profile,
	// list subscriptions, campaign views, and link clicks. Names of
	// private lists are replaced with "Private list".
	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}
	_, b, err := a.exportSubscriberData(id, "", a.cfg.Privacy.Exportable)
	if err != nil {
		a.log.Printf("error exporting subscriber data: %s", err)
		return apperr.Internal(a.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	// Set headers to force the browser to prompt for download.
	re.Response.Header().Set("Cache-Control", "no-cache")
	re.Response.Header().Set("Content-Disposition", `attachment; filename="data.json"`)
	return writeBlob(re, http.StatusOK, "application/json", b)
}

// exportSubscriberData collates the data of a subscriber including profile,
// subscriptions, campaign_views, link_clicks (if they're enabled in the config)
// and returns a formatted, indented JSON payload. id is the internal rowid
// resolved from the route; subUUID is used when exporting by uuid instead.
func (a *App) exportSubscriberData(id int, subUUID string, exportables map[string]bool) (models.SubscriberExportProfile, []byte, error) {
	data, err := a.core.GetSubscriberProfileForExport(id, subUUID)
	if err != nil {
		return data, nil, err
	}

	// Filter out the non-exportable items.
	if _, ok := exportables["profile"]; !ok {
		data.Profile = nil
	}
	if _, ok := exportables["subscriptions"]; !ok {
		data.Subscriptions = nil
	}
	if _, ok := exportables["campaign_views"]; !ok {
		data.CampaignViews = nil
	}
	if _, ok := exportables["link_clicks"]; !ok {
		data.LinkClicks = nil
	}

	// Marshal the data into an indented payload.
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		a.log.Printf("error marshalling subscriber export data: %v", err)
		return data, nil, err
	}

	return data, b, nil
}

// hasSubPerm checks whether the current user has permission to access the given list
// of subscriber IDs.
func (a *App) hasSubPerm(u auth.User, subIDs []int) error {
	allPerm, listRecordIDs := u.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage)

	// User has blanket get_all|manage_all permission.
	if allPerm {
		return nil
	}

	// Check whether the subscribers have the list IDs permitted to the user.
	listIDs, err := a.core.ResolveListIDs(nil, listRecordIDs)
	if err != nil {
		return err
	}
	res, err := a.core.HasSubscriberLists(subIDs, listIDs)
	if err != nil {
		return err
	}

	var denied []int
	for id, has := range res {
		if !has {
			denied = append(denied, id)
		}
	}
	if len(denied) == 0 {
		return nil
	}
	recIDs, err := a.core.ResolveSubscriberRecordIDs(denied)
	if err != nil {
		return err
	}
	name := "{globals.terms.subscriber}"
	if len(recIDs) > 0 {
		name = fmt.Sprintf("subscriber: %s", strings.Join(recIDs, ", "))
	}
	return apperr.Forbidden(a.i18n.Ts("globals.messages.permissionDenied", "name", name))
}

// filterListQueryByPerm filters the list IDs in the query params and returns the list IDs to which the user has access.
func (a *App) filterListQueryByPerm(param string, qp url.Values, user auth.User) ([]int, error) {
	var (
		listIDs []int
		err     error
	)
	recordParam := param

	// Primordial super admin and users with blanket subscriber access should never
	// be forced into a list-scoped fallback filter.
	if user.UserRoleID == auth.SuperAdminRoleID || user.HasPerm(auth.PermSubscribersGetAll) {
		if qp.Has(recordParam) {
			ids, err := a.core.ResolveListIDs(nil, qp[recordParam])
			if err != nil {
				return nil, apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
			}
			recordIDs, err := a.core.ResolveListRecordIDs(ids)
			if err != nil {
				return nil, apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
			}
			filteredRecordIDs := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, recordIDs)
			filtered, err := a.core.ResolveListIDs(nil, filteredRecordIDs)
			if err != nil {
				return nil, apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
			}
			if len(filtered) == 0 && qp.Has(recordParam) {
				return []int{-1}, nil
			}
			return filtered, nil
		}

		return nil, nil
	}

	// If there are incoming list query params, filter them by permission.
	if qp.Has(recordParam) {
		ids, err := a.core.ResolveListIDs(nil, qp[recordParam])
		if err != nil {
			return nil, apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
		}
		recordIDs, err := a.core.ResolveListRecordIDs(ids)
		if err != nil {
			return nil, apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
		}
		filteredRecordIDs := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, recordIDs)
		listIDs, err = a.core.ResolveListIDs(nil, filteredRecordIDs)
		if err != nil {
			return nil, apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
		}
	}

	// There are no incoming params. If the user doesn't have permission to get all subscribers,
	// filter by the lists they have access to.
	if len(listIDs) == 0 {
		if _, ok := user.PermissionsMap[auth.PermSubscribersGetAll]; !ok {
			if len(user.GetListIDs) > 0 {
				listIDs, err = a.core.ResolveListIDs(nil, user.GetListIDs)
				if err != nil {
					return nil, apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
				}
			} else {
				// User doesn't have access to any lists.
				listIDs = []int{-1}
			}
		}
	}

	return listIDs, nil
}

// formatSQLExp does basic sanitisation on arbitrary
// SQL query expressions coming from the frontend.
func formatSQLExp(q string) string {
	q = strings.TrimSpace(q)
	if len(q) == 0 {
		return ""
	}

	// Remove semicolon suffix.
	if q[len(q)-1] == ';' {
		q = q[:len(q)-1]
	}
	return q
}

// makeOptinNotifyHook returns an enclosed callback that sends optin confirmation e-mails.
// This is plugged into the 'core' package to send optin confirmations when a new subscriber is
// created via `core.CreateSubscriber()`.
func makeOptinNotifyHook(unsubHeader bool, u *UrlConfig, db *pbdb.DB, i *i18n.I18n) func(sub models.Subscriber, listIDs []int) (int, error) {
	return func(sub models.Subscriber, listIDs []int) (int, error) {
		// Fetch double opt-in lists from the given list IDs.
		// Get the list of subscription lists where the subscriber hasn't confirmed.
		var lists []models.List
		query := `
				SELECT
					l.rowid AS id,
					l.uuid,
					l.name,
					l.type,
					l.optin,
					l.status,
					l.tags,
					l.description,
					s.rowid AS subscriber_id,
					sl.status AS subscription_status,
					COALESCE(NULLIF(TRIM(sl.sms_status), ''), sl.status) AS subscription_sms_status
				FROM lists l
				LEFT JOIN subscriber_lists sl ON l.id = sl.list_id
				LEFT JOIN subscribers s ON s.id = sl.subscriber_id
				WHERE s.rowid = ?
				  AND sl.status = ?
				  AND l.optin = ?
			`
		args := []any{sub.ID, models.SubscriptionStatusUnconfirmed, models.ListOptinDouble}
		if len(listIDs) > 0 {
			query += ` AND l.rowid IN (` + strings.TrimSuffix(strings.Repeat("?,", len(listIDs)), ",") + `)`
			for _, id := range listIDs {
				args = append(args, id)
			}
		}
		query += ` ORDER BY l.rowid`

		if err := db.Select(&lists, query, args...); err != nil {
			lo.Printf("error fetching lists for opt-in: %s", err)
			return 0, err
		}

		// None.
		if len(lists) == 0 {
			return 0, nil
		}

		var (
			out      = subOptin{Subscriber: sub, Lists: lists}
			qListIDs = url.Values{}
		)

		// Construct the opt-in URL with list IDs.
		for _, l := range out.Lists {
			qListIDs.Add("l", l.UUID)
		}
		out.OptinURL = fmt.Sprintf(u.OptinURL, sub.RecordID, qListIDs.Encode())
		out.UnsubURL = fmt.Sprintf(u.UnsubURL, models.PreviewTrackingRecordID, sub.RecordID)

		// Unsub headers.
		hdr := textproto.MIMEHeader{}
		hdr.Set(models.EmailHeaderSubscriberUUID, sub.RecordID)

		// Attach List-Unsubscribe headers?
		if unsubHeader {
			unsubURL := fmt.Sprintf(u.UnsubURL, models.PreviewTrackingRecordID, sub.RecordID)
			hdr.Set("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
			hdr.Set("List-Unsubscribe", `<`+unsubURL+`>`)
		}

		// Send the e-mail.
		if err := notifs.Notify([]string{sub.Email}, i.T("subscribers.optinSubject"), notifs.TplSubscriberOptin, out, hdr); err != nil {
			lo.Printf("error sending opt-in e-mail for subscriber %d (%s): %s", sub.ID, sub.UUID, err)
			return 0, err
		}

		return len(lists), nil
	}
}
