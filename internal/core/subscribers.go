package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	pbcore "github.com/pocketbase/pocketbase/core"
)

var (
	allowedSubQueryTables = map[string]struct{}{
		"subscribers":       {},
		"lists":             {},
		"subscribers_lists": {},
		"campaigns":         {},
		"campaign_lists":    {},
		"campaign_views":    {},
		"links":             {},
		"link_clicks":       {},
		"bounces":           {},
	}
)

type sqliteSubscriberRow struct {
	ID        int             `db:"id"`
	CreatedAt string          `db:"created_at"`
	UpdatedAt string          `db:"updated_at"`
	UUID      string          `db:"uuid"`
	Email     string          `db:"email"`
	Name      string          `db:"name"`
	Attribs   []byte          `db:"attribs"`
	Status    string          `db:"status"`
}

type sqliteSubscriberActivityViewRow struct {
	ID           string `db:"id" json:"id"`
	UUID         string `db:"uuid" json:"uuid"`
	Name         string `db:"name" json:"name"`
	Subject      string `db:"subject" json:"subject"`
	ViewCount    int    `db:"view_count" json:"view_count"`
	LastViewedAt string `db:"last_viewed_at" json:"last_viewed_at"`
}

type sqliteSubscriberActivityClickRow struct {
	LinkID          string `db:"link_id" json:"link_id"`
	URL             string `db:"url" json:"url"`
	CampaignID      string `db:"campaign_id" json:"campaign_id"`
	CampaignUUID    string `db:"campaign_uuid" json:"campaign_uuid"`
	CampaignName    string `db:"campaign_name" json:"campaign_name"`
	CampaignSubject string `db:"campaign_subject" json:"campaign_subject"`
	ClickCount      int    `db:"click_count" json:"click_count"`
	LastClickedAt   string `db:"last_clicked_at" json:"last_clicked_at"`
}

func sqliteSubscriberRowsToModels(rows []sqliteSubscriberRow) models.Subscribers {
	out := make(models.Subscribers, 0, len(rows))
	for _, row := range rows {
		attribs := models.JSON{}
		if len(row.Attribs) > 0 && string(row.Attribs) != "null" {
			_ = json.Unmarshal(row.Attribs, &attribs)
		}

		out = append(out, models.Subscriber{
			Base: models.Base{
				ID:        row.ID,
				CreatedAt: parseNullTime(row.CreatedAt),
				UpdatedAt: parseNullTime(row.UpdatedAt),
			},
			UUID:    row.UUID,
			Email:   row.Email,
			Name:    row.Name,
			Attribs: attribs,
			Status:  row.Status,
		})
	}
	return out
}

func (c *Core) sqliteListRecordIDs(listIDs []int, listUUIDs []string) ([]string, error) {
	if len(listIDs) == 0 && len(listUUIDs) == 0 {
		return nil, nil
	}

	query := `SELECT id FROM lists WHERE `
	args := []any{}
	if len(listIDs) > 0 {
		query += `rowid IN (` + sqlitePlaceholders(len(listIDs)) + `)`
		for _, id := range listIDs {
			args = append(args, id)
		}
	} else {
		query += `uuid IN (` + sqlitePlaceholders(len(listUUIDs)) + `)`
		for _, id := range listUUIDs {
			args = append(args, id)
		}
	}

	var out []string
	if err := c.db.Select(&out, query, args...); err != nil {
		return nil, err
	}

	return out, nil
}

func (c *Core) sqliteSyncSubscriberLists(subscriberPBID string, listPBIDs []string, status string, deleteLists bool) error {
	if deleteLists {
		if len(listPBIDs) == 0 {
			if _, err := c.db.Exec(`DELETE FROM subscriber_lists WHERE subscriber_id = ?`, subscriberPBID); err != nil {
				return err
			}
		} else {
			args := []any{subscriberPBID}
			for _, id := range listPBIDs {
				args = append(args, id)
			}
			q := `DELETE FROM subscriber_lists WHERE subscriber_id = ? AND list_id NOT IN (` + sqlitePlaceholders(len(listPBIDs)) + `)`
			if _, err := c.db.Exec(q, args...); err != nil {
				return err
			}
		}
	}

	for _, listPBID := range listPBIDs {
		if _, err := c.db.Exec(`
			INSERT INTO subscriber_lists (subscriber_id, list_id, status)
			VALUES (?, ?, ?)
			ON CONFLICT (subscriber_id, list_id) DO UPDATE SET
				updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now'),
				status=excluded.status`,
			subscriberPBID, listPBID, status); err != nil {
			return err
		}
	}

	return nil
}

// GetSubscriber fetches a subscriber by one of the given params.
func (c *Core) GetSubscriber(id int, uuid, email string) (models.Subscriber, error) {
	if c.isSQLite() {
		return c.getSubscriberSQLite(id, uuid, email)
	}

	var uu any
	if uuid != "" {
		uu = uuid
	}

	var out models.Subscribers
	if err := c.q.GetSubscriber.Select(&out, id, uu, email); err != nil {
		c.log.Printf("error fetching subscriber: %v", err)
		return models.Subscriber{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching",
				"name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
	}
	if len(out) == 0 {
		return models.Subscriber{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name",
				fmt.Sprintf("{globals.terms.subscriber} (%d: %s%s)", id, uuid, email)))
	}
	if err := out.LoadLists(c.q.GetSubscriberListsLazy); err != nil {
		c.log.Printf("error loading subscriber lists: %v", err)
		return models.Subscriber{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching",
				"name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	return out[0], nil
}

// HasSubscriberLists checks if the given subscribers have at least one of the given lists.
func (c *Core) HasSubscriberLists(subIDs []int, listIDs []int) (map[int]bool, error) {
	if c.isSQLite() {
		return c.hasSubscriberListsSQLite(subIDs, listIDs)
	}

	res := []struct {
		SubID int  `db:"subscriber_id"`
		Has   bool `db:"has"`
	}{}

	if err := c.q.HasSubscriberLists.Select(&res, pq.Array(subIDs), pq.Array(listIDs)); err != nil {
		c.log.Printf("error fetching subscriber: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
	}

	out := make(map[int]bool, len(res))
	for _, r := range res {
		out[r.SubID] = r.Has
	}

	return out, nil
}

// GetSubscribersByEmail fetches a subscriber by one of the given params.
func (c *Core) GetSubscribersByEmail(emails []string) (models.Subscribers, error) {
	if c.isSQLite() {
		return c.getSubscribersByEmailSQLite(emails)
	}

	var out models.Subscribers

	if err := c.q.GetSubscribersByEmails.Select(&out, pq.Array(emails)); err != nil {
		c.log.Printf("error fetching subscriber: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
	}
	if len(out) == 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("campaigns.noKnownSubsToTest"))
	}

	if err := out.LoadLists(c.q.GetSubscriberListsLazy); err != nil {
		c.log.Printf("error loading subscriber lists: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// QuerySubscribers queries and returns paginated subscrribers based on the given params including the total count.
func (c *Core) QuerySubscribers(searchStr, queryExp string, listIDs []int, subStatus string, order, orderBy string, offset, limit int) (models.Subscribers, int, error) {
	if c.isSQLite() {
		return c.querySubscribersSQLite(searchStr, queryExp, listIDs, subStatus, order, orderBy, offset, limit)
	}

	// Sort params.
	if !strSliceContains(orderBy, subQuerySortFields) {
		orderBy = "subscribers.id"
	}
	if order != SortAsc && order != SortDesc {
		order = SortDesc
	}

	// Required for pq.Array()
	if listIDs == nil {
		listIDs = []int{}
	}

	// There's an arbitrary query condition.
	cond := "TRUE"
	if queryExp != "" {
		cond = queryExp
	}

	// stmt is the raw SQL query.
	stmt := strings.ReplaceAll(c.q.QuerySubscribers, "%query%", cond)
	stmt = strings.ReplaceAll(stmt, "%order%", orderBy+" "+order)

	// Validate the tables used in the query.
	if err := validateQueryTables(c.db, stmt, allowedSubQueryTables); err != nil {
		c.log.Printf("error validating query tables: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("subscribers.errorPreparingQuery", "error", err.Error()))
	}

	// Create a readonly transaction that just does COUNT() to obtain the count of results
	// and to ensure that the arbitrary query is indeed readonly.
	total, err := c.getSubscriberCount(searchStr, cond, subStatus, listIDs)
	if err != nil {
		c.log.Printf("error getting subscriber count: %v", err)
		return nil, 0, err
	}

	// No results.
	if total == 0 {
		return models.Subscribers{}, 0, nil
	}

	tx, err := c.db.BeginTxx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		c.log.Printf("error preparing subscriber query: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("subscribers.errorPreparingQuery", "error", pqErrMsg(err)))
	}
	defer tx.Rollback()

	var out models.Subscribers
	if err := tx.Select(&out, stmt, pq.Array(listIDs), subStatus, searchStr, offset, limit); err != nil {
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	// Lazy load lists for each subscriber.
	if err := out.LoadLists(c.q.GetSubscriberListsLazy); err != nil {
		c.log.Printf("error fetching subscriber lists: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return out, total, nil
}

// GetSubscriberLists returns a subscriber's lists based on the given conditions.
func (c *Core) GetSubscriberLists(subID int, uuid string, listIDs []int, listUUIDs []string, subStatus string, listType string) ([]models.List, error) {
	if c.isSQLite() {
		return c.getSubscriberListsSQLite(subID, uuid, listIDs, listUUIDs, subStatus, listType)
	}

	if listIDs == nil {
		listIDs = []int{}
	}
	if listUUIDs == nil {
		listUUIDs = []string{}
	}

	var uu any
	if uuid != "" {
		uu = uuid
	}

	// Fetch double opt-in lists from the given list IDs.
	// Get the list of subscription lists where the subscriber hasn't confirmed.
	out := []models.List{}
	if err := c.q.GetSubscriberLists.Select(&out, subID, uu, pq.Array(listIDs), pq.Array(listUUIDs), subStatus, listType); err != nil {
		c.log.Printf("error fetching lists for opt-in: %s", pqErrMsg(err))
		return nil, err
	}

	return out, nil
}

// GetSubscriberProfileForExport returns the subscriber's profile data as a JSON exportable.
// Get the subscriber's data. A single query that gets the profile, list subscriptions, campaign views,
// and link clicks. Names of private lists are replaced with "Private list".
func (c *Core) GetSubscriberProfileForExport(id int, uuid string) (models.SubscriberExportProfile, error) {
	if c.isSQLite() {
		return c.getSubscriberProfileForExportSQLite(id, uuid)
	}

	var uu any
	if uuid != "" {
		uu = uuid
	}

	var out models.SubscriberExportProfile
	if err := c.q.ExportSubscriberData.Get(&out, id, uu); err != nil {
		c.log.Printf("error fetching subscriber export data: %v", err)

		return models.SubscriberExportProfile{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	return out, nil
}

// GetSubscriberActivity returns the subscriber's campaign views and link clicks for the Activity tab.
func (c *Core) GetSubscriberActivity(id int) (models.SubscriberActivity, error) {
	if c.isSQLite() {
		return c.getSubscriberActivitySQLite(id)
	}

	var out models.SubscriberActivity
	if err := c.q.GetSubscriberActivity.Get(&out, id); err != nil {
		c.log.Printf("error fetching subscriber activity: %v", err)

		return models.SubscriberActivity{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "activity", "error", err.Error()))
	}

	return out, nil
}

// ExportSubscribers returns an iterator function that provides lists of subscribers based
// on the given criteria in an exportable form. The iterator function returned can be called
// repeatedly until there are nil subscribers. It's an iterator because exports can be extremely
// large and may have to be fetched in batches from the DB and streamed somewhere.
func (c *Core) ExportSubscribers(searchStr, query string, subIDs, listIDs []int, subStatus string, batchSize int) (func() ([]models.SubscriberExport, error), error) {
	if c.isSQLite() {
		return c.exportSubscribersSQLite(searchStr, query, subIDs, listIDs, subStatus, batchSize)
	}

	if subIDs == nil {
		subIDs = []int{}
	}
	if listIDs == nil {
		listIDs = []int{}
	}

	// There's an arbitrary query condition.
	cond := "TRUE"
	if query != "" {
		cond = query
	}

	stmt := strings.ReplaceAll(c.q.QuerySubscribersForExport, "%query%", cond)

	// Create a readonly transaction that just does COUNT() to obtain the count of results
	// and to ensure that the arbitrary query is indeed readonly.
	if _, err := c.getSubscriberCount(searchStr, cond, subStatus, listIDs); err != nil {
		c.log.Printf("error getting subscriber count: %v", err)
		return nil, err
	}

	// Prepare the actual query statement.
	tx, err := c.db.Preparex(stmt)
	if err != nil {
		c.log.Printf("error preparing subscriber query: %v", err)
		return nil, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("subscribers.errorPreparingQuery", "error", pqErrMsg(err)))
	}

	id := 0
	return func() ([]models.SubscriberExport, error) {
		var out []models.SubscriberExport
		if err := tx.Select(&out, pq.Array(listIDs), id, pq.Array(subIDs), subStatus, searchStr, batchSize); err != nil {
			c.log.Printf("error exporting subscribers by query: %v", err)
			return nil, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}
		if len(out) == 0 {
			return nil, nil
		}

		id = out[len(out)-1].ID
		return out, nil
	}, nil
}

// InsertSubscriber inserts a subscriber and returns the ID. The first bool indicates if
// it was a new subscriber, and the second bool indicates if the subscriber was sent an optin confirmation.
// bool = optinSent?
func (c *Core) InsertSubscriber(sub models.Subscriber, listIDs []int, listUUIDs []string, preconfirm, assertOptin bool) (models.Subscriber, bool, error) {
	uu, err := uuid.NewV4()
	if err != nil {
		c.log.Printf("error generating UUID: %v", err)
		return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
	}
	sub.UUID = uu.String()

	subStatus := models.SubscriptionStatusUnconfirmed
	if preconfirm {
		subStatus = models.SubscriptionStatusConfirmed
	}
	if sub.Status == "" {
		sub.Status = auth.UserStatusEnabled
	}

	// For pq.Array()
	if listIDs == nil {
		listIDs = []int{}
	}
	if listUUIDs == nil {
		listUUIDs = []string{}
	}

	if c.isSQLite() {
		pb := c.db.PocketBase()
		if pb == nil {
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", "pocketbase is not initialized"))
		}

		collection, err := pb.FindCollectionByNameOrId("subscribers")
		if err != nil {
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
		}

		record := pbcore.NewRecord(collection)
		record.Set("uuid", sub.UUID)
		record.Set("email", sub.Email)
		record.Set("name", strings.TrimSpace(sub.Name))
		record.Set("status", sub.Status)
		record.Set("attribs", sub.Attribs)

		if err := pb.Save(record); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") && strings.Contains(strings.ToLower(err.Error()), "email") {
				return models.Subscriber{}, false, echo.NewHTTPError(http.StatusConflict, c.i18n.T("subscribers.emailExists"))
			}
			c.log.Printf("error inserting subscriber: %v", err)
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
		}
		subscriberPBID := record.Id

		if len(listIDs) > 0 || len(listUUIDs) > 0 {
			listPBIDs, err := c.sqliteListRecordIDs(listIDs, listUUIDs)
			if err != nil {
				return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
					c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
			}
			status := subStatus
			if sub.Status == models.SubscriberStatusBlockListed {
				status = models.SubscriptionStatusUnsubscribed
			}
			if err := c.sqliteSyncSubscriberLists(subscriberPBID, listPBIDs, status, false); err != nil {
				return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
					c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
			}
		}

		out, err := c.GetSubscriber(0, sub.UUID, sub.Email)
		if err != nil {
			return models.Subscriber{}, false, err
		}

		hasOptin := false
		if !preconfirm && c.consts.SendOptinConfirmation {
			num, err := c.h.SendOptinConfirmation(out, listIDs)
			if assertOptin && err != nil {
				return out, hasOptin, err
			}
			hasOptin = num > 0
		}

		return out, hasOptin, nil
	}

	if err = c.q.InsertSubscriber.Get(&sub.ID,
		sub.UUID,
		sub.Email,
		strings.TrimSpace(sub.Name),
		sub.Status,
		sub.Attribs,
		pq.Array(listIDs),
		pq.Array(listUUIDs),
		subStatus); err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Constraint == "subscribers_email_key" {
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusConflict, c.i18n.T("subscribers.emailExists"))
		} else {
			// return sub.Subscriber, errSubscriberExists
			c.log.Printf("error inserting subscriber: %v", err)
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
		}
	}

	// Fetch the subscriber's full data. If the subscriber already existed and wasn't
	// created, the id will be empty. Fetch the details by e-mail then.
	out, err := c.GetSubscriber(sub.ID, "", sub.Email)
	if err != nil {
		return models.Subscriber{}, false, err
	}

	hasOptin := false
	if !preconfirm && c.consts.SendOptinConfirmation {
		// Send a confirmation e-mail (if there are any double opt-in lists).
		num, err := c.h.SendOptinConfirmation(out, listIDs)
		if assertOptin && err != nil {
			return out, hasOptin, err
		}

		hasOptin = num > 0
	}

	return out, hasOptin, nil
}

// UpdateSubscriber updates a subscriber's properties.
func (c *Core) UpdateSubscriber(id int, sub models.Subscriber) (models.Subscriber, error) {
	if c.isSQLite() {
		out, _, err := c.UpdateSubscriberWithLists(id, sub, nil, nil, false, false, false)
		return out, err
	}

	// Format raw JSON attributes.
	attribs := []byte("{}")
	if len(sub.Attribs) > 0 {
		if b, err := json.Marshal(sub.Attribs); err != nil {
			return models.Subscriber{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating",
					"name", "{globals.terms.subscriber}", "error", err.Error()))
		} else {
			attribs = b
		}
	}

	_, err := c.q.UpdateSubscriber.Exec(id,
		sub.Email,
		strings.TrimSpace(sub.Name),
		sub.Status,
		json.RawMessage(attribs),
	)
	if err != nil {
		c.log.Printf("error updating subscriber: %v", err)
		return models.Subscriber{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
	}

	out, err := c.GetSubscriber(sub.ID, "", sub.Email)
	if err != nil {
		return models.Subscriber{}, err
	}

	return out, nil
}

// UpdateSubscriberWithLists updates a subscriber's properties.
// If deleteLists is set to true, all existing subscriptions are deleted and only
// the ones provided are added or retained.
func (c *Core) UpdateSubscriberWithLists(id int, sub models.Subscriber, listIDs []int, listUUIDs []string, preconfirm, deleteLists, assertOptin bool) (models.Subscriber, bool, error) {
	subStatus := models.SubscriptionStatusUnconfirmed
	if preconfirm {
		subStatus = models.SubscriptionStatusConfirmed
	}

	if c.isSQLite() {
		pb := c.db.PocketBase()
		if pb == nil {
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", "pocketbase is not initialized"))
		}

		var recID string
		if err := c.db.Get(&recID, `SELECT id FROM subscribers WHERE rowid = ?`, id); err != nil {
			c.log.Printf("error updating subscriber: %v", err)
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
		}

		rec, err := pb.FindRecordById("subscribers", recID)
		if err != nil {
			c.log.Printf("error updating subscriber: %v", err)
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
		}

		rec.Set("email", sub.Email)
		rec.Set("name", strings.TrimSpace(sub.Name))
		rec.Set("status", sub.Status)
		rec.Set("attribs", sub.Attribs)
		if err := pb.Save(rec); err != nil {
			c.log.Printf("error updating subscriber: %v", err)
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
		}

		listPBIDs, err := c.sqliteListRecordIDs(listIDs, listUUIDs)
		if err != nil {
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
		}

		status := subStatus
		if sub.Status == models.SubscriberStatusBlockListed {
			status = models.SubscriptionStatusUnsubscribed
		}
		if err := c.sqliteSyncSubscriberLists(recID, listPBIDs, status, deleteLists); err != nil {
			c.log.Printf("error updating subscriber: %v", err)
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
		}

		out, err := c.GetSubscriber(id, "", sub.Email)
		if err != nil {
			return models.Subscriber{}, false, err
		}

		hasOptin := false
		if !preconfirm && c.consts.SendOptinConfirmation && len(listIDs) > 0 {
			num, err := c.h.SendOptinConfirmation(out, listIDs)
			if assertOptin && err != nil {
				return out, hasOptin, err
			}
			hasOptin = num > 0
		}

		return out, hasOptin, nil
	}

	// Format raw JSON attributes.
	attribs := []byte("{}")
	if len(sub.Attribs) > 0 {
		if b, err := json.Marshal(sub.Attribs); err != nil {
			return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating",
					"name", "{globals.terms.subscriber}", "error", err.Error()))
		} else {
			attribs = b
		}
	}

	_, err := c.q.UpdateSubscriberWithLists.Exec(id,
		sub.Email,
		strings.TrimSpace(sub.Name),
		sub.Status,
		json.RawMessage(attribs),
		pq.Array(listIDs),
		pq.Array(listUUIDs),
		subStatus,
		deleteLists)
	if err != nil {
		c.log.Printf("error updating subscriber: %v", err)
		return models.Subscriber{}, false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
	}

	out, err := c.GetSubscriber(sub.ID, "", sub.Email)
	if err != nil {
		return models.Subscriber{}, false, err
	}

	hasOptin := false
	if !preconfirm && c.consts.SendOptinConfirmation {
		// Send a confirmation e-mail (if there are any double opt-in lists).
		num, err := c.h.SendOptinConfirmation(out, listIDs)
		if assertOptin && err != nil {
			return out, hasOptin, err
		}
		hasOptin = num > 0
	}

	return out, hasOptin, nil
}

// BlocklistSubscribers blocklists the given list of subscribers.
func (c *Core) BlocklistSubscribers(subIDs []int) error {
	if c.isSQLite() {
		if len(subIDs) == 0 {
			return nil
		}

		args := make([]any, 0, len(subIDs))
		for _, id := range subIDs {
			args = append(args, id)
		}

		q := `UPDATE subscribers
			SET status='blocklisted', updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id IN (` + sqlitePlaceholders(len(subIDs)) + `)`
		if _, err := c.db.Exec(q, args...); err != nil {
			c.log.Printf("error blocklisting subscribers: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("subscribers.errorBlocklisting", "error", err.Error()))
		}

		q = `UPDATE subscriber_lists
			SET status='unsubscribed', updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id IN (` + sqlitePlaceholders(len(subIDs)) + `)`
		if _, err := c.db.Exec(q, args...); err != nil {
			c.log.Printf("error blocklisting subscribers: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("subscribers.errorBlocklisting", "error", err.Error()))
		}

		return nil
	}

	if _, err := c.q.BlocklistSubscribers.Exec(pq.Array(subIDs)); err != nil {
		c.log.Printf("error blocklisting subscribers: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("subscribers.errorBlocklisting", "error", err.Error()))
	}

	return nil
}

// BlocklistSubscribersByQuery blocklists the given list of subscribers.
func (c *Core) BlocklistSubscribersByQuery(searchStr, queryExp string, listIDs []int, subStatus string) error {
	if c.isSQLite() {
		ids, err := c.findSubscriberIDsSQLite(searchStr, queryExp, listIDs, subStatus, 0, 0)
		if err != nil {
			c.log.Printf("error blocklisting subscribers: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("subscribers.errorBlocklisting", "error", pqErrMsg(err)))
		}

		for i := 0; i < len(ids); i += 400 {
			end := i + 400
			if end > len(ids) {
				end = len(ids)
			}
			if err := c.BlocklistSubscribers(ids[i:end]); err != nil {
				return err
			}
		}
		return nil
	}

	if err := c.q.ExecSubQueryTpl(searchStr, sanitizeSQLExp(queryExp), c.q.BlocklistSubscribersByQuery, listIDs, c.db, subStatus); err != nil {
		c.log.Printf("error blocklisting subscribers: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("subscribers.errorBlocklisting", "error", pqErrMsg(err)))
	}

	return nil
}

// DeleteSubscribers deletes the given list of subscribers.
func (c *Core) DeleteSubscribers(subIDs []int, subUUIDs []string) error {
	if subIDs == nil {
		subIDs = []int{}
	}
	if subUUIDs == nil {
		subUUIDs = []string{}
	}

	if c.isSQLite() {
		if len(subIDs) == 0 && len(subUUIDs) == 0 {
			return nil
		}

		clauses := make([]string, 0, 2)
		args := make([]any, 0, len(subIDs)+len(subUUIDs))

		if len(subIDs) > 0 {
			clauses = append(clauses, `id IN (`+sqlitePlaceholders(len(subIDs))+`)`)
			for _, id := range subIDs {
				args = append(args, id)
			}
		}
		if len(subUUIDs) > 0 {
			clauses = append(clauses, `uuid IN (`+sqlitePlaceholders(len(subUUIDs))+`)`)
			for _, u := range subUUIDs {
				args = append(args, u)
			}
		}

		q := `DELETE FROM subscribers WHERE ` + strings.Join(clauses, " OR ")
		if _, err := c.db.Exec(q, args...); err != nil {
			c.log.Printf("error deleting subscribers: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}
		return nil
	}

	if _, err := c.q.DeleteSubscribers.Exec(pq.Array(subIDs), pq.Array(subUUIDs)); err != nil {
		c.log.Printf("error deleting subscribers: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return nil
}

// DeleteSubscribersByQuery deletes subscribers by a given arbitrary query expression.
func (c *Core) DeleteSubscribersByQuery(searchStr, queryExp string, listIDs []int, subStatus string) error {
	if c.isSQLite() {
		ids, err := c.findSubscriberIDsSQLite(searchStr, queryExp, listIDs, subStatus, 0, 0)
		if err != nil {
			c.log.Printf("error deleting subscribers: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}

		for i := 0; i < len(ids); i += 400 {
			end := i + 400
			if end > len(ids) {
				end = len(ids)
			}
			if err := c.DeleteSubscribers(ids[i:end], nil); err != nil {
				return err
			}
		}
		return nil
	}

	err := c.q.ExecSubQueryTpl(searchStr, sanitizeSQLExp(queryExp), c.q.DeleteSubscribersByQuery, listIDs, c.db, subStatus)
	if err != nil {
		c.log.Printf("error deleting subscribers: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return err
}

// UnsubscribeByCampaign unsubscribes a given subscriber from lists in a given campaign.
func (c *Core) UnsubscribeByCampaign(subUUID, campUUID string, blocklist bool) error {
	if c.isSQLite() {
		var (
			subRecID  string
			campRecID string
		)

		if err := c.db.Get(&subRecID, `SELECT id FROM subscribers WHERE uuid = ?`, subUUID); err != nil {
			c.log.Printf("error unsubscribing: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}

		if err := c.db.Get(&campRecID, `SELECT id FROM campaigns WHERE uuid = ?`, campUUID); err != nil {
			c.log.Printf("error unsubscribing: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}

		if blocklist {
			if _, err := c.db.Exec(`UPDATE subscribers
				SET status = 'blocklisted',
				    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
				WHERE id = ?`, subRecID); err != nil {
				c.log.Printf("error unsubscribing: %v", err)
				return echo.NewHTTPError(http.StatusInternalServerError,
					c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
			}

			if _, err := c.db.Exec(`UPDATE subscriber_lists
				SET status = 'unsubscribed',
				    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
				WHERE subscriber_id = ?
				  AND status != 'unsubscribed'`, subRecID); err != nil {
				c.log.Printf("error unsubscribing: %v", err)
				return echo.NewHTTPError(http.StatusInternalServerError,
					c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
			}

			return nil
		}

		if _, err := c.db.Exec(`UPDATE subscriber_lists
			SET status = 'unsubscribed',
			    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id = ?
			  AND status != 'unsubscribed'
			  AND list_id IN (
			    SELECT list_id
			    FROM campaign_lists
			    WHERE campaign_id = ?
			  )`, subRecID, campRecID); err != nil {
			c.log.Printf("error unsubscribing: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}

		return nil
	}

	if _, err := c.q.UnsubscribeByCampaign.Exec(campUUID, subUUID, blocklist); err != nil {
		c.log.Printf("error unsubscribing: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return nil
}

// ConfirmOptionSubscription confirms a subscriber's optin subscription.
func (c *Core) ConfirmOptionSubscription(subUUID string, listUUIDs []string, meta models.JSON) error {
	if meta == nil {
		meta = models.JSON{}
	}

	if _, err := c.q.ConfirmSubscriptionOptin.Exec(subUUID, pq.Array(listUUIDs), meta); err != nil {
		c.log.Printf("error confirming subscription: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return nil
}

// DeleteSubscriberBounces deletes the given list of subscribers.
func (c *Core) DeleteSubscriberBounces(id int, uuid string) error {
	if c.isSQLite() {
		var subID int
		var err error
		if uuid != "" {
			err = c.db.Get(&subID, `SELECT id FROM subscribers WHERE uuid = ?`, uuid)
		} else {
			subID = id
		}
		if err != nil && err != sql.ErrNoRows {
			c.log.Printf("error deleting bounces: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.bounces}", "error", pqErrMsg(err)))
		}
		if subID == 0 {
			return nil
		}
		if _, err := c.db.Exec(`DELETE FROM bounces WHERE subscriber_id = ?`, subID); err != nil {
			c.log.Printf("error deleting bounces: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.bounces}", "error", pqErrMsg(err)))
		}
		return nil
	}

	var uu any
	if uuid != "" {
		uu = uuid
	}

	if _, err := c.q.DeleteBouncesBySubscriber.Exec(id, uu); err != nil {
		c.log.Printf("error deleting bounces: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.bounces}", "error", pqErrMsg(err)))
	}

	return nil
}

func (c *Core) getSubscriberProfileForExportSQLite(id int, uuid string) (models.SubscriberExportProfile, error) {
	query := `SELECT id, uuid, email, name, attribs, status, created_at, updated_at FROM subscribers WHERE `
	args := []any{}
	if id > 0 {
		query += `id = ?`
		args = append(args, id)
	} else {
		query += `uuid = ?`
		args = append(args, uuid)
	}
	query += ` LIMIT 1`

	prof := map[string]any{}
	row := c.db.QueryRowx(query, args...)
	if err := row.MapScan(prof); err != nil {
		if err == sql.ErrNoRows {
			return models.SubscriberExportProfile{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.subscriber}"))
		}
		c.log.Printf("error fetching subscriber export data: %v", err)
		return models.SubscriberExportProfile{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	sid, ok := prof["id"].(int64)
	if !ok || sid == 0 {
		return models.SubscriberExportProfile{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.subscriber}"))
	}

	var subs []map[string]any
	if err := c.db.Select(&subs, `
		SELECT
			sl.status AS subscription_status,
			(CASE WHEN l.type = 'private' THEN 'Private list' ELSE l.name END) AS name,
			l.type,
			sl.created_at
		FROM subscriber_lists sl
		LEFT JOIN lists l ON l.id = sl.list_id
		WHERE sl.subscriber_id = ?`, sid); err != nil {
		c.log.Printf("error fetching subscriber subscriptions: %v", err)
		return models.SubscriberExportProfile{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	var views []map[string]any
	if err := c.db.Select(&views, `
		SELECT c.subject AS campaign, COUNT(cv.subscriber_id) AS views
		FROM campaign_views cv
		LEFT JOIN campaigns c ON c.id = cv.campaign_id
		WHERE cv.subscriber_id = ?
		GROUP BY c.id, c.subject
		ORDER BY c.id`, sid); err != nil {
		c.log.Printf("error fetching subscriber campaign views: %v", err)
		return models.SubscriberExportProfile{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	var clicks []map[string]any
	if err := c.db.Select(&clicks, `
		SELECT l.url, COUNT(lc.subscriber_id) AS clicks
		FROM link_clicks lc
		LEFT JOIN links l ON l.id = lc.link_id
		WHERE lc.subscriber_id = ?
		GROUP BY l.id, l.url
		ORDER BY l.id`, sid); err != nil {
		c.log.Printf("error fetching subscriber link clicks: %v", err)
		return models.SubscriberExportProfile{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	profJSON, _ := json.Marshal(prof)
	subsJSON, _ := json.Marshal(subs)
	viewsJSON, _ := json.Marshal(views)
	clicksJSON, _ := json.Marshal(clicks)

	email, _ := prof["email"].(string)
	return models.SubscriberExportProfile{
		Email:         email,
		Profile:       profJSON,
		Subscriptions: subsJSON,
		CampaignViews: viewsJSON,
		LinkClicks:    clicksJSON,
	}, nil
}

func (c *Core) getSubscriberActivitySQLite(id int) (models.SubscriberActivity, error) {
	var views []sqliteSubscriberActivityViewRow
	if err := c.db.Select(&views, `
		SELECT
			c.id,
			c.uuid,
			c.name,
			c.subject,
			COUNT(*) AS view_count,
			MAX(cv.created) AS last_viewed_at
		FROM campaign_views cv
		LEFT JOIN campaigns c ON c.id = cv.campaign_id
		LEFT JOIN subscribers s ON s.id = cv.subscriber_id
		WHERE s.rowid = ?
		GROUP BY c.id, c.uuid, c.name, c.subject
		ORDER BY last_viewed_at DESC`, id); err != nil {
		c.log.Printf("error fetching subscriber activity views: %v", err)
		return models.SubscriberActivity{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "activity", "error", err.Error()))
	}

	var clicks []sqliteSubscriberActivityClickRow
	if err := c.db.Select(&clicks, `
		SELECT
			l.id AS link_id,
			l.url,
			c.id AS campaign_id,
			c.uuid AS campaign_uuid,
			c.name AS campaign_name,
			c.subject AS campaign_subject,
			COUNT(*) AS click_count,
			MAX(lc.created) AS last_clicked_at
		FROM link_clicks lc
		LEFT JOIN links l ON l.id = lc.link_id
		LEFT JOIN campaigns c ON c.id = lc.campaign_id
		LEFT JOIN subscribers s ON s.id = lc.subscriber_id
		WHERE s.rowid = ?
		GROUP BY l.id, l.url, c.id, c.uuid, c.name, c.subject
		ORDER BY last_clicked_at DESC`, id); err != nil {
		c.log.Printf("error fetching subscriber activity clicks: %v", err)
		return models.SubscriberActivity{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "activity", "error", err.Error()))
	}

	viewsJSON, _ := json.Marshal(views)
	clicksJSON, _ := json.Marshal(clicks)

	return models.SubscriberActivity{
		CampaignViews: viewsJSON,
		LinkClicks:    clicksJSON,
	}, nil
}

// DeleteOrphanSubscribers deletes orphan subscriber records (subscribers without lists).
func (c *Core) DeleteOrphanSubscribers() (int, error) {
	res, err := c.q.DeleteOrphanSubscribers.Exec()
	if err != nil {
		c.log.Printf("error deleting orphan subscribers: %v", err)
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteBlocklistedSubscribers deletes blocklisted subscribers.
func (c *Core) DeleteBlocklistedSubscribers() (int, error) {
	res, err := c.q.DeleteBlocklistedSubscribers.Exec()
	if err != nil {
		c.log.Printf("error deleting blocklisted subscribers: %v", err)
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	n, _ := res.RowsAffected()
	return int(n), nil
}

func (c *Core) getSubscriberCount(searchStr, queryExp, subStatus string, listIDs []int) (int, error) {
	if c.isSQLite() {
		whereSQL, args := c.subscriberFilterSQLite(searchStr, queryExp, listIDs, subStatus)
		total := 0
		if err := c.db.Get(&total, `SELECT COUNT(*) FROM subscribers WHERE `+whereSQL, args...); err != nil {
			return 0, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}

		return total, nil
	}

	// If there's no condition, it's a "get all" call which can probably be optionally pulled from cache.
	if queryExp == "" {
		_ = c.refreshCache(matListSubStats, false)

		total := 0
		if err := c.q.QuerySubscribersCountAll.Get(&total, pq.Array(listIDs), subStatus); err != nil {
			return 0, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}

		return total, nil
	}

	// Create a readonly transaction that just does COUNT() to obtain the count of results
	// and to ensure that the arbitrary query is indeed readonly.
	stmt := strings.ReplaceAll(c.q.QuerySubscribersCount, "%query%", queryExp)
	tx, err := c.db.BeginTxx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		c.log.Printf("error preparing subscriber query: %v", err)
		return 0, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("subscribers.errorPreparingQuery", "error", pqErrMsg(err)))
	}
	defer tx.Rollback()

	// Execute the readonly query and get the count of results.
	total := 0
	if err := tx.Get(&total, stmt, pq.Array(listIDs), subStatus, searchStr); err != nil {
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return total, nil
}

func (c *Core) getSubscriberSQLite(id int, uuid, email string) (models.Subscriber, error) {
	q := `SELECT rowid AS id, created AS created_at, updated AS updated_at, uuid, email, name, attribs, status FROM subscribers WHERE `
	args := []any{}
	switch {
	case id > 0:
		q += `rowid = ?`
		args = append(args, id)
	case uuid != "":
		q += `uuid = ?`
		args = append(args, uuid)
	case email != "":
		q += `email = ?`
		args = append(args, email)
	default:
		return models.Subscriber{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.subscriber}"))
	}
	q += ` LIMIT 1`

	var rows []sqliteSubscriberRow
	if err := c.db.Select(&rows, q, args...); err != nil {
		if err == sql.ErrNoRows {
			return models.Subscriber{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name",
					fmt.Sprintf("{globals.terms.subscriber} (%d: %s%s)", id, uuid, email)))
		}
		c.log.Printf("error fetching subscriber: %v", err)
		return models.Subscriber{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
	}
	if len(rows) == 0 {
		return models.Subscriber{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name",
				fmt.Sprintf("{globals.terms.subscriber} (%d: %s%s)", id, uuid, email)))
	}

	subs := sqliteSubscriberRowsToModels(rows)
	if err := c.loadSubscriberListsSQLite(subs); err != nil {
		c.log.Printf("error loading subscriber lists: %v", err)
		return models.Subscriber{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	return subs[0], nil
}

func (c *Core) hasSubscriberListsSQLite(subIDs, listIDs []int) (map[int]bool, error) {
	res := make(map[int]bool)
	if len(subIDs) == 0 {
		return res, nil
	}

	for _, id := range subIDs {
		res[id] = false
	}
	if len(listIDs) == 0 {
		return res, nil
	}

	q := `
		SELECT s.id AS subscriber_id,
			CASE
				WHEN EXISTS (
					SELECT 1 FROM subscriber_lists sl
					WHERE sl.subscriber_id = s.id
					  AND sl.list_id IN (` + sqlitePlaceholders(len(listIDs)) + `)
				) THEN 1 ELSE 0
			END AS has
		FROM subscribers s
		WHERE s.id IN (` + sqlitePlaceholders(len(subIDs)) + `)
	`
	args := make([]any, 0, len(listIDs)+len(subIDs))
	for _, id := range listIDs {
		args = append(args, id)
	}
	for _, id := range subIDs {
		args = append(args, id)
	}

	rows := []struct {
		SubID int  `db:"subscriber_id"`
		Has   bool `db:"has"`
	}{}
	if err := c.db.Select(&rows, q, args...); err != nil {
		c.log.Printf("error fetching subscriber: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
	}

	for _, r := range rows {
		res[r.SubID] = r.Has
	}
	return res, nil
}

func (c *Core) getSubscribersByEmailSQLite(emails []string) (models.Subscribers, error) {
	if len(emails) == 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("campaigns.noKnownSubsToTest"))
	}

	args := make([]any, 0, len(emails))
	for _, e := range emails {
		args = append(args, e)
	}

	var rows []sqliteSubscriberRow
	q := `SELECT rowid AS id, created AS created_at, updated AS updated_at, uuid, email, name, attribs, status FROM subscribers WHERE email IN (` + sqlitePlaceholders(len(emails)) + `) ORDER BY rowid`
	if err := c.db.Select(&rows, q, args...); err != nil {
		c.log.Printf("error fetching subscriber: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscriber}", "error", pqErrMsg(err)))
	}
	if len(rows) == 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("campaigns.noKnownSubsToTest"))
	}
	out := sqliteSubscriberRowsToModels(rows)

	if err := c.loadSubscriberListsSQLite(out); err != nil {
		c.log.Printf("error loading subscriber lists: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}
	return out, nil
}

func (c *Core) querySubscribersSQLite(searchStr, queryExp string, listIDs []int, subStatus string, order, orderBy string, offset, limit int) (models.Subscribers, int, error) {
	if order != SortAsc && order != SortDesc {
		order = SortDesc
	}

	orderMap := map[string]string{
		"email":      "subscribers.email",
		"status":     "subscribers.status",
		"name":       "subscribers.name",
		"created_at": "subscribers.created",
		"updated_at": "subscribers.updated",
	}
	sortCol, ok := orderMap[orderBy]
	if !ok {
		sortCol = "subscribers.rowid"
	}

	whereSQL, args := c.subscriberFilterSQLite(searchStr, queryExp, listIDs, subStatus)
	total := 0
	if err := c.db.Get(&total, `SELECT COUNT(*) FROM subscribers WHERE `+whereSQL, args...); err != nil {
		c.log.Printf("error getting subscriber count: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}
	if total == 0 {
		return models.Subscribers{}, 0, nil
	}

	q := `SELECT subscribers.rowid AS id, subscribers.created AS created_at, subscribers.updated AS updated_at,
		subscribers.uuid, subscribers.email, subscribers.name, subscribers.attribs, subscribers.status
		FROM subscribers WHERE ` + whereSQL + ` ORDER BY ` + sortCol + ` ` + order
	if limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	} else if offset > 0 {
		q += ` LIMIT -1 OFFSET ?`
		args = append(args, offset)
	}

	var rows []sqliteSubscriberRow
	c.log.Printf("query subscribers sqlite: where=%q args=%v total=%d order_by=%q order=%q", whereSQL, args, total, sortCol, order)
	if err := c.db.Select(&rows, q, args...); err != nil {
		c.log.Printf("error querying subscribers: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}
	out := sqliteSubscriberRowsToModels(rows)
	if err := c.loadSubscriberListsSQLite(out); err != nil {
		c.log.Printf("error fetching subscriber lists: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return out, total, nil
}

func (c *Core) getSubscriberListsSQLite(subID int, uuid string, listIDs []int, listUUIDs []string, subStatus string, listType string) ([]models.List, error) {
	if listIDs == nil {
		listIDs = []int{}
	}
	if listUUIDs == nil {
		listUUIDs = []string{}
	}

	sid := subID
	if sid == 0 && uuid != "" {
		if err := c.db.Get(&sid, `SELECT rowid FROM subscribers WHERE uuid = ?`, uuid); err != nil {
			if err == sql.ErrNoRows {
				return []models.List{}, nil
			}
			c.log.Printf("error fetching lists for opt-in: %s", pqErrMsg(err))
			return nil, err
		}
	}
	if sid == 0 {
		return []models.List{}, nil
	}

	q := `
		SELECT
			l.rowid AS id, l.uuid, l.name, l.type, l.optin, l.status, l.tags, l.description,
			s.rowid AS subscriber_id,
			sl.status AS subscription_status
		FROM lists l
		LEFT JOIN subscriber_lists sl ON l.id = sl.list_id
		LEFT JOIN subscribers s ON s.id = sl.subscriber_id
		WHERE s.rowid = ?
	`
	args := []any{sid}

	if len(listIDs) > 0 {
		q += ` AND l.id IN (` + sqlitePlaceholders(len(listIDs)) + `)`
		for _, id := range listIDs {
			args = append(args, id)
		}
	} else if len(listUUIDs) > 0 {
		q += ` AND l.uuid IN (` + sqlitePlaceholders(len(listUUIDs)) + `)`
		for _, u := range listUUIDs {
			args = append(args, u)
		}
	}
	if subStatus != "" {
		q += ` AND sl.status = ?`
		args = append(args, subStatus)
	}
	if listType != "" {
		q += ` AND l.optin = ?`
		args = append(args, listType)
	}
	q += ` ORDER BY l.id`

	rows := []struct {
		ID                 int    `db:"id"`
		UUID               string `db:"uuid"`
		Name               string `db:"name"`
		Type               string `db:"type"`
		Optin              string `db:"optin"`
		Status             string `db:"status"`
		Tags               string `db:"tags"`
		Description        string `db:"description"`
		SubscriberID       int    `db:"subscriber_id"`
		SubscriptionStatus string `db:"subscription_status"`
	}{}

	if err := c.db.Select(&rows, q, args...); err != nil {
		c.log.Printf("error fetching lists for opt-in: %s", pqErrMsg(err))
		return nil, err
	}

	out := make([]models.List, 0, len(rows))
	for _, r := range rows {
		tags := pq.StringArray{}
		if strings.TrimSpace(r.Tags) != "" {
			var parsed []string
			if err := json.Unmarshal([]byte(r.Tags), &parsed); err == nil {
				tags = pq.StringArray(parsed)
			}
		}

		out = append(out, models.List{
			Base:               models.Base{ID: r.ID},
			UUID:               r.UUID,
			Name:               r.Name,
			Type:               r.Type,
			Optin:              r.Optin,
			Status:             r.Status,
			Tags:               tags,
			Description:        r.Description,
			SubscriberID:       r.SubscriberID,
			SubscriptionStatus: r.SubscriptionStatus,
		})
	}

	return out, nil
}

func (c *Core) exportSubscribersSQLite(searchStr, query string, subIDs, listIDs []int, subStatus string, batchSize int) (func() ([]models.SubscriberExport, error), error) {
	if subIDs == nil {
		subIDs = []int{}
	}
	if listIDs == nil {
		listIDs = []int{}
	}
	cond := "TRUE"
	if query != "" {
		cond = query
	}

	if _, err := c.getSubscriberCount(searchStr, cond, subStatus, listIDs); err != nil {
		c.log.Printf("error getting subscriber count: %v", err)
		return nil, err
	}

	id := 0
	return func() ([]models.SubscriberExport, error) {
		whereSQL, args := c.subscriberFilterSQLite(searchStr, cond, listIDs, subStatus)
		whereSQL = `subscribers.id > ? AND ` + whereSQL
		args = append([]any{id}, args...)

		if len(subIDs) > 0 {
			whereSQL += ` AND subscribers.id IN (` + sqlitePlaceholders(len(subIDs)) + `)`
			for _, sid := range subIDs {
				args = append(args, sid)
			}
		}

		q := `
			SELECT subscribers.id, subscribers.uuid, subscribers.email, subscribers.name,
			       subscribers.status, subscribers.attribs, subscribers.created_at, subscribers.updated_at
			FROM subscribers
			WHERE ` + whereSQL + `
			ORDER BY subscribers.id ASC`
		if batchSize > 0 {
			q += ` LIMIT ?`
			args = append(args, batchSize)
		}

		var out []models.SubscriberExport
		if err := c.db.Select(&out, q, args...); err != nil {
			c.log.Printf("error exporting subscribers by query: %v", err)
			return nil, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}
		if len(out) == 0 {
			return nil, nil
		}

		id = out[len(out)-1].ID
		return out, nil
	}, nil
}

func (c *Core) loadSubscriberListsSQLite(subs models.Subscribers) error {
	for i := range subs {
		lists, err := c.getSubscriberListsSQLite(subs[i].ID, "", nil, nil, "", "")
		if err != nil {
			return err
		}
		if b, err := json.Marshal(lists); err == nil {
			subs[i].Lists = b
		} else {
			return err
		}
	}
	return nil
}

func (c *Core) findSubscriberIDsSQLite(searchStr, queryExp string, listIDs []int, subStatus string, offset, limit int) ([]int, error) {
	whereSQL, args := c.subscriberFilterSQLite(searchStr, queryExp, listIDs, subStatus)
	q := `SELECT subscribers.rowid AS id FROM subscribers WHERE ` + whereSQL + ` ORDER BY subscribers.rowid ASC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
		if offset > 0 {
			q += ` OFFSET ?`
			args = append(args, offset)
		}
	} else if offset > 0 {
		q += ` LIMIT -1 OFFSET ?`
		args = append(args, offset)
	}

	out := []int{}
	if err := c.db.Select(&out, q, args...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Core) subscriberFilterSQLite(searchStr, queryExp string, listIDs []int, subStatus string) (string, []any) {
	where := []string{"1=1"}
	args := []any{}

	if len(listIDs) > 0 {
		clause := `EXISTS (
			SELECT 1 FROM subscriber_lists sl
			JOIN lists l ON l.id = sl.list_id
			WHERE sl.subscriber_id = subscribers.id
			  AND l.rowid IN (` + sqlitePlaceholders(len(listIDs)) + `)`
		for _, id := range listIDs {
			args = append(args, id)
		}
		if subStatus != "" {
			clause += ` AND sl.status = ?`
			args = append(args, subStatus)
		}
		clause += `)`
		where = append(where, clause)
	}

	if searchStr != "" {
		like := "%" + searchStr + "%"
		where = append(where, `(subscribers.name LIKE ? COLLATE NOCASE OR subscribers.email LIKE ? COLLATE NOCASE)`)
		args = append(args, like, like)
	}

	exp := strings.TrimSpace(sanitizeSQLExp(queryExp))
	if exp != "" && exp != "TRUE" {
		where = append(where, "("+exp+")")
	}

	return strings.Join(where, " AND "), args
}

// validateQueryTables checks if the query accesses only allowed tables.
func validateQueryTables(db *pbdb.DB, query string, allowedTables map[string]struct{}) error {
	// Get the EXPLAIN (FORMAT JSON) output.
	tx, err := db.BeginTxx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var plan string
	if err = tx.QueryRow("EXPLAIN (FORMAT JSON) "+query, nil, models.SubscriberStatusEnabled, "", 0, 10).Scan(&plan); err != nil {
		return err
	}

	// Extract all relation names from the JSON plan.
	tables, err := getTablesFromQueryPlan(plan)
	if err != nil {
		return fmt.Errorf("error getting tables from query: %v", err)
	}

	// Validate against allowed tables.
	for _, table := range tables {
		if _, ok := allowedTables[table]; !ok {
			return fmt.Errorf("table '%s' is not allowed", table)
		}
	}

	return nil
}

// getTablesFromQueryPlan parses the EXPLAIN JSON to find all "Relation Name" entries.
func getTablesFromQueryPlan(explainJSON string) ([]string, error) {
	var plans []map[string]any
	if err := json.Unmarshal([]byte(explainJSON), &plans); err != nil {
		return nil, err
	}

	// Collect table names in `tables` recursively.
	tables := make(map[string]struct{})
	for _, plan := range plans {
		traverseQueryPlan(plan, tables)
	}

	result := make([]string, 0, len(tables))
	for table := range tables {
		result = append(result, table)
	}
	return result, nil
}

func traverseQueryPlan(node map[string]any, tables map[string]struct{}) {
	if relName, ok := node["Relation Name"].(string); ok {
		tables[relName] = struct{}{}
	}

	// Recursively check nested plans (e.g., subqueries, CTEs).
	for _, v := range node {
		switch v := v.(type) {
		case map[string]any:
			traverseQueryPlan(v, tables)
		case []any:
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					traverseQueryPlan(m, tables)
				}
			}
		}
	}
}
