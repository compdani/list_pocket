package core

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
	pbcore "github.com/pocketbase/pocketbase/core"
)

type sqliteSubscriberRow struct {
	ID        int    `db:"id"`
	RecordID  string `db:"record_id"`
	CreatedAt string `db:"created_at"`
	UpdatedAt string `db:"updated_at"`
	UUID      string `db:"uuid"`
	Email     string `db:"email"`
	Phone     string `db:"phone"`
	FirstName string `db:"first_name"`
	LastName  string `db:"last_name"`
	Name      string `db:"name"`
	Attribs   []byte `db:"attribs"`
	Status    string `db:"status"`
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

type sqliteSubscriberCampaignSendRow struct {
	ID      string `db:"id" json:"id"`
	UUID    string `db:"uuid" json:"uuid"`
	Name    string `db:"name" json:"name"`
	Subject string `db:"subject" json:"subject"`
	Status  string `db:"status" json:"status"`
	Created string `db:"created" json:"created"`
	Updated string `db:"updated" json:"updated"`
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
				RecordID:  row.RecordID,
				CreatedAt: parseNullTime(row.CreatedAt),
				UpdatedAt: parseNullTime(row.UpdatedAt),
			},
			UUID:      row.UUID,
			Email:     row.Email,
			Phone:     row.Phone,
			FirstName: row.FirstName,
			LastName:  row.LastName,
			Name:      row.Name,
			Attribs:   attribs,
			Status:    row.Status,
		})
	}
	return out
}

func sqliteSubscriberRowsToExports(rows []sqliteSubscriberRow) []models.SubscriberExport {
	out := make([]models.SubscriberExport, 0, len(rows))
	for _, row := range rows {
		attribs := ""
		if len(row.Attribs) > 0 && string(row.Attribs) != "null" {
			attribs = string(row.Attribs)
		}
		out = append(out, models.SubscriberExport{
			Base: models.Base{
				ID:        row.ID,
				RecordID:  row.RecordID,
				CreatedAt: parseNullTime(row.CreatedAt),
				UpdatedAt: parseNullTime(row.UpdatedAt),
			},
			UUID:      row.UUID,
			Email:     row.Email,
			Phone:     row.Phone,
			FirstName: row.FirstName,
			LastName:  row.LastName,
			Name:      row.Name,
			Attribs:   attribs,
			Status:    row.Status,
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

func (c *Core) SQLiteListRecordIDs(listIDs []int, listUUIDs []string) ([]string, error) {
	return c.sqliteListRecordIDs(listIDs, listUUIDs)
}

func appendUniqueInts(dst []int, values []int) []int {
	seen := make(map[int]struct{}, len(dst)+len(values))
	for _, v := range dst {
		seen[v] = struct{}{}
	}
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		dst = append(dst, v)
	}
	return dst
}

func (c *Core) ResolveListIDs(listIDs []int, listRecordIDs []string) ([]int, error) {
	if len(listRecordIDs) == 0 {
		return appendUniqueInts([]int{}, listIDs), nil
	}

	query := `SELECT rowid FROM lists WHERE id IN (` + sqlitePlaceholders(len(listRecordIDs)) + `)`
	args := make([]any, 0, len(listRecordIDs))
	for _, id := range listRecordIDs {
		args = append(args, id)
	}

	var resolved []int
	if err := c.db.Select(&resolved, query, args...); err != nil {
		return nil, err
	}

	return appendUniqueInts(append([]int{}, listIDs...), resolved), nil
}

func (c *Core) ResolveSubscriberIDs(subIDs []int, subRecordIDs []string) ([]int, error) {
	if len(subRecordIDs) == 0 {
		return appendUniqueInts([]int{}, subIDs), nil
	}

	query := `SELECT rowid FROM subscribers WHERE id IN (` + sqlitePlaceholders(len(subRecordIDs)) + `)`
	args := make([]any, 0, len(subRecordIDs))
	for _, id := range subRecordIDs {
		args = append(args, id)
	}

	var resolved []int
	if err := c.db.Select(&resolved, query, args...); err != nil {
		return nil, err
	}

	return appendUniqueInts(append([]int{}, subIDs...), resolved), nil
}

func (c *Core) ResolveSubscriberRecordIDs(subIDs []int) ([]string, error) {
	if len(subIDs) == 0 {
		return nil, nil
	}

	query := `SELECT rowid AS row_id, id FROM subscribers WHERE rowid IN (` + sqlitePlaceholders(len(subIDs)) + `)`
	args := make([]any, 0, len(subIDs))
	for _, id := range subIDs {
		args = append(args, id)
	}

	var rows []struct {
		RowID int    `db:"row_id"`
		ID    string `db:"id"`
	}
	if err := c.db.Select(&rows, query, args...); err != nil {
		return nil, err
	}

	idMap := make(map[int]string, len(rows))
	for _, row := range rows {
		idMap[row.RowID] = row.ID
	}

	resolved := make([]string, 0, len(subIDs))
	for _, id := range subIDs {
		if recordID := idMap[id]; recordID != "" {
			resolved = append(resolved, recordID)
		}
	}

	return resolved, nil
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
			INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (subscriber_id, list_id) DO UPDATE SET
				updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now'),
				status=excluded.status,
				sms_status=excluded.sms_status`,
			subscriberPBID, listPBID, status, status); err != nil {
			return err
		}
	}

	return nil
}

// GetSubscriber fetches a subscriber by one of the given params.
func (c *Core) GetSubscriber(id int, uuid, email string) (models.Subscriber, error) {
	return c.getSubscriberSQLite(id, uuid, email)
}

// HasSubscriberLists checks if the given subscribers have at least one of the given lists.
func (c *Core) HasSubscriberLists(subIDs []int, listIDs []int) (map[int]bool, error) {
	return c.hasSubscriberListsSQLite(subIDs, listIDs)
}

// GetSubscribersByEmail fetches a subscriber by one of the given params.
func (c *Core) GetSubscribersByEmail(emails []string) (models.Subscribers, error) {
	return c.getSubscribersByEmailSQLite(emails)
}

// GetSubscribersByNormalizedPhones loads subscribers whose phone matches any of the given
// digit-only strings (same normalization as SMS opt-out).
func (c *Core) GetSubscribersByNormalizedPhones(digits []string) (models.Subscribers, error) {
	return c.getSubscribersByNormalizedPhonesSQLite(digits)
}

// QuerySubscribers queries and returns paginated subscrribers based on the given params including the total count.
func (c *Core) QuerySubscribers(searchStr, queryExp string, filters json.RawMessage, listIDs []int, subStatus string, order, orderBy string, offset, limit int) (models.Subscribers, int, error) {
	return c.querySubscribersSQLite(searchStr, queryExp, filters, listIDs, subStatus, order, orderBy, offset, limit)
}

// GetSubscriberLists returns a subscriber's lists based on the given conditions.
func (c *Core) GetSubscriberLists(subID int, uuid string, listIDs []int, listUUIDs []string, subStatus string, listType string) ([]models.List, error) {
	return c.getSubscriberListsSQLite(subID, uuid, listIDs, listUUIDs, subStatus, listType)
}

// GetSubscriberProfileForExport returns the subscriber's profile data as a JSON exportable.
// Get the subscriber's data. A single query that gets the profile, list subscriptions, campaign views,
// and link clicks. Names of private lists are replaced with "Private list".
func (c *Core) GetSubscriberProfileForExport(id int, uuid string) (models.SubscriberExportProfile, error) {
	return c.getSubscriberProfileForExportSQLite(id, uuid)
}

// GetSubscriberActivity returns the subscriber's campaign views and link clicks for the Activity tab.
func (c *Core) GetSubscriberActivity(id int) (models.SubscriberActivity, error) {
	return c.getSubscriberActivitySQLite(id)
}

// ExportSubscribers returns an iterator function that provides lists of subscribers based
// on the given criteria in an exportable form. The iterator function returned can be called
// repeatedly until there are nil subscribers. It's an iterator because exports can be extremely
// large and may have to be fetched in batches from the DB and streamed somewhere.
func (c *Core) ExportSubscribers(searchStr, query string, filters json.RawMessage, subIDs, listIDs []int, subStatus string, batchSize int) (func() ([]models.SubscriberExport, error), error) {
	return c.exportSubscribersSQLite(searchStr, query, filters, subIDs, listIDs, subStatus, batchSize)
}

// InsertSubscriber inserts a subscriber and returns the ID. The first bool indicates if
// it was a new subscriber, and the second bool indicates if the subscriber was sent an optin confirmation.
// bool = optinSent?
func (c *Core) InsertSubscriber(sub models.Subscriber, listIDs []int, listUUIDs []string, preconfirm, assertOptin bool) (models.Subscriber, bool, error) {
	uu, err := uuid.NewV4()
	if err != nil {
		c.log.Printf("error generating UUID: %v", err)
		return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
	}
	sub.UUID = uu.String()
	sub.NormalizeName()

	subStatus := models.SubscriptionStatusUnconfirmed
	if preconfirm {
		subStatus = models.SubscriptionStatusConfirmed
	}
	if sub.Status == "" {
		sub.Status = auth.UserStatusEnabled
	}

	if listIDs == nil {
		listIDs = []int{}
	}
	if listUUIDs == nil {
		listUUIDs = []string{}
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", "pocketbase is not initialized"))
	}

	collection, err := pb.FindCollectionByNameOrId("subscribers")
	if err != nil {
		return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
	}

	record := pbcore.NewRecord(collection)
	record.Set("uuid", sub.UUID)
	record.Set("email", sub.Email)
	record.Set("phone", strings.TrimSpace(sub.Phone))
	record.Set("first_name", sub.FirstName)
	record.Set("last_name", sub.LastName)
	record.Set("name", strings.TrimSpace(sub.Name))
	record.Set("status", sub.Status)
	record.Set("attribs", sub.Attribs)

	if err := pb.Save(record); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") && strings.Contains(strings.ToLower(err.Error()), "email") {
			return models.Subscriber{}, false, apperr.Conflict(c.i18n.T("subscribers.emailExists"))
		}
		c.log.Printf("error inserting subscriber: %v", err)
		return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
	}
	subscriberPBID := record.Id

	if len(listIDs) > 0 || len(listUUIDs) > 0 {
		listPBIDs, err := c.sqliteListRecordIDs(listIDs, listUUIDs)
		if err != nil {
			return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
		}
		status := subStatus
		if sub.Status == models.SubscriberStatusBlockListed {
			status = models.SubscriptionStatusUnsubscribed
		}
		if err := c.sqliteSyncSubscriberLists(subscriberPBID, listPBIDs, status, false); err != nil {
			return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
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

// UpdateSubscriber updates a subscriber's properties by PocketBase record id.
func (c *Core) UpdateSubscriber(recordID string, sub models.Subscriber) (models.Subscriber, error) {
	sub.NormalizeName()
	out, _, err := c.UpdateSubscriberWithLists(recordID, sub, nil, nil, false, false, false)
	return out, err
}

// UpdateSubscriberWithLists updates a subscriber's properties by PocketBase record id.
// If deleteLists is set to true, all existing subscriptions are deleted and only
// the ones provided are added or retained.
func (c *Core) UpdateSubscriberWithLists(recordID string, sub models.Subscriber, listIDs []int, listUUIDs []string, preconfirm, deleteLists, assertOptin bool) (models.Subscriber, bool, error) {
	sub.NormalizeName()
	subStatus := models.SubscriptionStatusUnconfirmed
	if preconfirm {
		subStatus = models.SubscriptionStatusConfirmed
	}

	recordID = strings.TrimSpace(recordID)
	if recordID == "" {
		return models.Subscriber{}, false, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.subscriber}"))
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", "pocketbase is not initialized"))
	}

	rec, err := pb.FindRecordById("subscribers", recordID)
	if err != nil {
		c.log.Printf("error updating subscriber: %v", err)
		return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
	}

	rec.Set("email", sub.Email)
	rec.Set("phone", strings.TrimSpace(sub.Phone))
	rec.Set("first_name", sub.FirstName)
	rec.Set("last_name", sub.LastName)
	rec.Set("name", strings.TrimSpace(sub.Name))
	rec.Set("status", sub.Status)
	rec.Set("attribs", sub.Attribs)
	if err := pb.Save(rec); err != nil {
		c.log.Printf("error updating subscriber: %v", err)
		return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
	}

	listPBIDs, err := c.sqliteListRecordIDs(listIDs, listUUIDs)
	if err != nil {
		return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
	}

	status := subStatus
	if sub.Status == models.SubscriberStatusBlockListed {
		status = models.SubscriptionStatusUnsubscribed
	}
	if err := c.sqliteSyncSubscriberLists(recordID, listPBIDs, status, deleteLists); err != nil {
		c.log.Printf("error updating subscriber: %v", err)
		return models.Subscriber{}, false, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
	}

	out, err := c.GetSubscriber(0, recordID, "")
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

// BlocklistSubscribers blocklists subscribers by PocketBase record id (subscribers.id TEXT).
func (c *Core) BlocklistSubscribers(recordIDs []string) error {
	if len(recordIDs) == 0 {
		return nil
	}

	args := make([]any, 0, len(recordIDs))
	for _, id := range recordIDs {
		args = append(args, id)
	}

	q := `UPDATE subscribers
		SET status='blocklisted', updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE id IN (` + sqlitePlaceholders(len(recordIDs)) + `)`
	if _, err := c.db.Exec(q, args...); err != nil {
		c.log.Printf("error blocklisting subscribers: %v", err)
		return apperr.Internal(c.i18n.Ts("subscribers.errorBlocklisting", "error", err.Error()))
	}

	q = `UPDATE subscriber_lists
		SET status='unsubscribed', updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE subscriber_id IN (` + sqlitePlaceholders(len(recordIDs)) + `)`
	if _, err := c.db.Exec(q, args...); err != nil {
		c.log.Printf("error blocklisting subscribers: %v", err)
		return apperr.Internal(c.i18n.Ts("subscribers.errorBlocklisting", "error", err.Error()))
	}

	return nil
}

// BlocklistSubscribersByQuery blocklists the given list of subscribers.
func (c *Core) BlocklistSubscribersByQuery(searchStr, queryExp string, filters json.RawMessage, listIDs []int, subStatus string) error {
	ids, err := c.findSubscriberIDsSQLite(searchStr, queryExp, filters, listIDs, subStatus, 0, 0)
	if err != nil {
		c.log.Printf("error blocklisting subscribers: %v", err)
		return apperr.Internal(c.i18n.Ts("subscribers.errorBlocklisting", "error", dbErr(err)))
	}

	for i := 0; i < len(ids); i += 400 {
		end := i + 400
		if end > len(ids) {
			end = len(ids)
		}
		recIDs, err := c.ResolveSubscriberRecordIDs(ids[i:end])
		if err != nil {
			return err
		}
		if err := c.BlocklistSubscribers(recIDs); err != nil {
			return err
		}
	}
	return nil
}

// DeleteSubscribers deletes subscribers by PocketBase record id (subscribers.id) and/or by uuid (public flows).
func (c *Core) DeleteSubscribers(recordIDs []string, uuids []string) error {
	if recordIDs == nil {
		recordIDs = []string{}
	}
	if uuids == nil {
		uuids = []string{}
	}

	if len(recordIDs) == 0 && len(uuids) == 0 {
		return nil
	}

	clauses := make([]string, 0, 2)
	args := make([]any, 0, len(recordIDs)+len(uuids))

	if len(recordIDs) > 0 {
		clauses = append(clauses, `id IN (`+sqlitePlaceholders(len(recordIDs))+`)`)
		for _, id := range recordIDs {
			args = append(args, id)
		}
	}
	if len(uuids) > 0 {
		clauses = append(clauses, `uuid IN (`+sqlitePlaceholders(len(uuids))+`)`)
		for _, u := range uuids {
			args = append(args, u)
		}
	}

	q := `DELETE FROM subscribers WHERE ` + strings.Join(clauses, " OR ")
	if _, err := c.db.Exec(q, args...); err != nil {
		c.log.Printf("error deleting subscribers: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	return nil
}

// DeleteSubscribersByQuery deletes subscribers by a given arbitrary query expression.
func (c *Core) DeleteSubscribersByQuery(searchStr, queryExp string, filters json.RawMessage, listIDs []int, subStatus string) error {
	ids, err := c.findSubscriberIDsSQLite(searchStr, queryExp, filters, listIDs, subStatus, 0, 0)
	if err != nil {
		c.log.Printf("error deleting subscribers: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	for i := 0; i < len(ids); i += 400 {
		end := i + 400
		if end > len(ids) {
			end = len(ids)
		}
		recIDs, err := c.ResolveSubscriberRecordIDs(ids[i:end])
		if err != nil {
			return err
		}
		if err := c.DeleteSubscribers(recIDs, nil); err != nil {
			return err
		}
	}
	return nil
}

func isPlaceholderCampaignURL(camp string) bool {
	return strings.EqualFold(strings.TrimSpace(camp), models.DummyUUID) || strings.TrimSpace(camp) == models.PreviewTrackingRecordID
}

// unsubscribeSubscriberAllPublicLists unsubscribes a subscriber from every public list
// (used when the unsubscribe URL has no real campaign id, e.g. opt-in notifications).
func (c *Core) unsubscribeSubscriberAllPublicLists(subRecID string, blocklist bool) error {
	var messenger string
	if err := c.db.Get(&messenger, `
		SELECT COALESCE(c.messenger, '') FROM campaigns c
		INNER JOIN campaign_lists cl ON cl.campaign_id = c.id
		INNER JOIN subscriber_lists sl ON sl.list_id = cl.list_id AND sl.subscriber_id = ?
		LIMIT 1`, subRecID); err != nil {
		if err != sql.ErrNoRows {
			c.log.Printf("error unsubscribing: %v", err)
			return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
		}
		messenger = ""
	}
	smsChannel := models.IsTextMessenger(messenger)

	tx, err := c.db.Beginx()
	if err != nil {
		c.log.Printf("error unsubscribing: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}
	defer tx.Rollback()

	if blocklist {
		if _, err := tx.Exec(`UPDATE subscribers
			SET status = 'blocklisted',
			    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id = ?`, subRecID); err != nil {
			c.log.Printf("error unsubscribing: %v", err)
			return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
		}

		var listUpd string
		if smsChannel {
			listUpd = `UPDATE subscriber_lists
			SET sms_status = 'unsubscribed',
			    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id = ?
			  AND list_id IN (SELECT id FROM lists WHERE type = ?)
			  AND COALESCE(sms_status, status) != 'unsubscribed'`
		} else {
			listUpd = `UPDATE subscriber_lists
			SET status = 'unsubscribed',
			    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id = ?
			  AND list_id IN (SELECT id FROM lists WHERE type = ?)
			  AND status != 'unsubscribed'`
		}
		if _, err := tx.Exec(listUpd, subRecID, models.ListTypePublic); err != nil {
			c.log.Printf("error unsubscribing: %v", err)
			return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
		}

		if err := tx.Commit(); err != nil {
			c.log.Printf("error unsubscribing: %v", err)
			return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
		}
		return nil
	}

	var res sql.Result
	if smsChannel {
		res, err = tx.Exec(`UPDATE subscriber_lists
		SET sms_status = 'unsubscribed',
		    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE subscriber_id = ?
		  AND COALESCE(sms_status, status) != 'unsubscribed'
		  AND list_id IN (SELECT id FROM lists WHERE type = ?)`, subRecID, models.ListTypePublic)
	} else {
		res, err = tx.Exec(`UPDATE subscriber_lists
		SET status = 'unsubscribed',
		    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE subscriber_id = ?
		  AND status != 'unsubscribed'
		  AND list_id IN (SELECT id FROM lists WHERE type = ?)`, subRecID, models.ListTypePublic)
	}
	if err != nil {
		c.log.Printf("error unsubscribing: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	_ = res

	if err := tx.Commit(); err != nil {
		c.log.Printf("error unsubscribing: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	return nil
}

// UnsubscribeByCampaign unsubscribes a given subscriber from lists in a given campaign.
func (c *Core) UnsubscribeByCampaign(subUUID, campUUID string, blocklist bool) error {
	var (
		subRecID  string
		campRecID string
	)

	if err := c.db.Get(&subRecID, `SELECT id FROM subscribers WHERE uuid = ? OR id = ?`, subUUID, subUUID); err != nil {
		c.log.Printf("error unsubscribing: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	if err := c.db.Get(&campRecID, `SELECT id FROM campaigns WHERE uuid = ? OR id = ?`, campUUID, campUUID); err != nil {
		if err == sql.ErrNoRows && isPlaceholderCampaignURL(campUUID) {
			return c.unsubscribeSubscriberAllPublicLists(subRecID, blocklist)
		}
		c.log.Printf("error unsubscribing: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	var messenger string
	if err := c.db.Get(&messenger, `SELECT COALESCE(messenger, '') FROM campaigns WHERE id = ?`, campRecID); err != nil {
		c.log.Printf("error unsubscribing: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}
	smsChannel := models.IsTextMessenger(messenger)

	tx, err := c.db.Beginx()
	if err != nil {
		c.log.Printf("error unsubscribing: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}
	defer tx.Rollback()

	var hasCampaignSubscriptions bool
	membershipActive := `sl.status != 'unsubscribed'`
	if smsChannel {
		membershipActive = `COALESCE(sl.sms_status, sl.status) != 'unsubscribed'`
	}
	if err := tx.Get(&hasCampaignSubscriptions, `
		SELECT EXISTS(
			SELECT 1
			FROM subscriber_lists sl
			INNER JOIN campaign_lists cl ON cl.list_id = sl.list_id
			WHERE sl.subscriber_id = ?
			  AND cl.campaign_id = ?
			  AND `+membershipActive+`
		)`, subRecID, campRecID); err != nil {
		c.log.Printf("error unsubscribing: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	if blocklist {
		if _, err := tx.Exec(`UPDATE subscribers
			SET status = 'blocklisted',
			    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id = ?`, subRecID); err != nil {
			c.log.Printf("error unsubscribing: %v", err)
			return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
		}

		var listUpd string
		if smsChannel {
			listUpd = `UPDATE subscriber_lists
			SET sms_status = 'unsubscribed',
			    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id = ?
			  AND COALESCE(sms_status, status) != 'unsubscribed'`
		} else {
			listUpd = `UPDATE subscriber_lists
			SET status = 'unsubscribed',
			    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id = ?
			  AND status != 'unsubscribed'`
		}
		if _, err := tx.Exec(listUpd, subRecID); err != nil {
			c.log.Printf("error unsubscribing: %v", err)
			return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
		}

		if hasCampaignSubscriptions {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO campaign_unsubscribes (campaign_id, subscriber_id, created, updated)
				VALUES (?, ?, strftime('%Y-%m-%d %H:%M:%fZ'), strftime('%Y-%m-%d %H:%M:%fZ'))`, campRecID, subRecID); err != nil {
				c.log.Printf("error recording campaign unsubscribe: %v", err)
				return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
			}
		}

		if err := tx.Commit(); err != nil {
			c.log.Printf("error unsubscribing: %v", err)
			return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
		}
		return nil
	}

	var res sql.Result
	if smsChannel {
		res, err = tx.Exec(`UPDATE subscriber_lists
		SET sms_status = 'unsubscribed',
		    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE subscriber_id = ?
		  AND COALESCE(sms_status, status) != 'unsubscribed'
		  AND list_id IN (
		    SELECT list_id
		    FROM campaign_lists
		    WHERE campaign_id = ?
		  )`, subRecID, campRecID)
	} else {
		res, err = tx.Exec(`UPDATE subscriber_lists
		SET status = 'unsubscribed',
		    updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE subscriber_id = ?
		  AND status != 'unsubscribed'
		  AND list_id IN (
		    SELECT list_id
		    FROM campaign_lists
		    WHERE campaign_id = ?
		  )`, subRecID, campRecID)
	}
	if err != nil {
		c.log.Printf("error unsubscribing: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	if hasCampaignSubscriptions {
		if n, _ := res.RowsAffected(); n > 0 {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO campaign_unsubscribes (campaign_id, subscriber_id, created, updated)
				VALUES (?, ?, strftime('%Y-%m-%d %H:%M:%fZ'), strftime('%Y-%m-%d %H:%M:%fZ'))`, campRecID, subRecID); err != nil {
				c.log.Printf("error recording campaign unsubscribe: %v", err)
				return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
			}
		}
	}

	if err := tx.Commit(); err != nil {
		c.log.Printf("error unsubscribing: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	return nil
}

// ConfirmOptionSubscription confirms a subscriber's optin subscription.
func (c *Core) ConfirmOptionSubscription(subUUID string, listUUIDs []string, meta models.JSON) error {
	if meta == nil {
		meta = models.JSON{}
	}

	if len(listUUIDs) == 0 {
		return nil
	}

	metaJSON, _ := json.Marshal(meta)
	metaStr := string(metaJSON)
	execArgs := []any{metaStr, metaStr, subUUID, subUUID}
	for _, listUUID := range listUUIDs {
		execArgs = append(execArgs, listUUID)
	}

	if _, err := c.db.Exec(`UPDATE subscriber_lists
		SET status='confirmed',
		    meta=(CASE
		        WHEN json_valid(COALESCE(meta, '{}')) AND json_valid(?)
		            THEN json_patch(COALESCE(meta, '{}'), ?)
		        ELSE COALESCE(meta, '{}')
		    END),
		    updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
		WHERE subscriber_id = (SELECT id FROM subscribers WHERE uuid = ? OR id = ?)
		  AND list_id IN (
		      SELECT id FROM lists WHERE uuid IN (`+sqlitePlaceholders(len(listUUIDs))+`)
		  )`, execArgs...); err != nil {
		c.log.Printf("error confirming subscription: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	return nil
}

// DeleteSubscriberBounces deletes the given list of subscribers.
func (c *Core) DeleteSubscriberBounces(id int, uuid string) error {
	var subRecID string
	var err error
	if uuid != "" {
		err = c.db.Get(&subRecID, `SELECT id FROM subscribers WHERE uuid = ?`, uuid)
	} else {
		err = c.db.Get(&subRecID, `SELECT id FROM subscribers WHERE rowid = ?`, id)
	}
	if err != nil && err != sql.ErrNoRows {
		c.log.Printf("error deleting bounces: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.bounces}", "error", dbErr(err)))
	}
	if subRecID == "" {
		return nil
	}
	if _, err := c.db.Exec(`DELETE FROM bounces WHERE subscriber_id = ?`, subRecID); err != nil {
		c.log.Printf("error deleting bounces: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.bounces}", "error", dbErr(err)))
	}

	return nil
}

func (c *Core) getSubscriberProfileForExportSQLite(id int, uuid string) (models.SubscriberExportProfile, error) {
	query := `SELECT rowid AS id, uuid, email, name, attribs, status, created AS created_at, updated AS updated_at FROM subscribers WHERE `
	args := []any{}
	if id > 0 {
		query += `rowid = ?`
		args = append(args, id)
	} else if uuid != "" && models.IsRFC4122UUID(uuid) {
		query += `uuid = ?`
		args = append(args, uuid)
	} else if uuid != "" {
		query += `id = ?`
		args = append(args, uuid)
	} else {
		return models.SubscriberExportProfile{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.subscriber}"))
	}
	query += ` LIMIT 1`

	prof := map[string]any{}
	row := c.db.QueryRowx(query, args...)
	if err := row.MapScan(prof); err != nil {
		if err == sql.ErrNoRows {
			return models.SubscriberExportProfile{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.subscriber}"))
		}
		c.log.Printf("error fetching subscriber export data: %v", err)
		return models.SubscriberExportProfile{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	sid, ok := prof["id"].(int64)
	if !ok || sid == 0 {
		return models.SubscriberExportProfile{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.subscriber}"))
	}

	var subs []map[string]any
	if err := c.db.Select(&subs, `
		SELECT
			sl.status AS subscription_status,
			COALESCE(NULLIF(TRIM(sl.sms_status), ''), sl.status) AS subscription_sms_status,
			(CASE WHEN l.type = 'private' THEN 'Private list' ELSE l.name END) AS name,
			l.type,
			sl.created AS created_at
		FROM subscriber_lists sl
		LEFT JOIN lists l ON l.id = sl.list_id
		WHERE sl.subscriber_id = ?`, sid); err != nil {
		c.log.Printf("error fetching subscriber subscriptions: %v", err)
		return models.SubscriberExportProfile{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	var views []map[string]any
	if err := c.db.Select(&views, `
		SELECT c.subject AS campaign, COUNT(cv.subscriber_id) AS views
		FROM campaign_views cv
		LEFT JOIN campaigns c ON c.id = cv.campaign_id
		WHERE cv.subscriber_id = ?
		  AND COALESCE(cv.is_suspected_privacy_open, 0) = 0
		GROUP BY c.id, c.subject
		ORDER BY c.id`, sid); err != nil {
		c.log.Printf("error fetching subscriber campaign views: %v", err)
		return models.SubscriberExportProfile{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
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
		return models.SubscriberExportProfile{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
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
	var sends []sqliteSubscriberCampaignSendRow
	if err := c.db.Select(&sends, `
		SELECT
			c.id,
			c.uuid,
			c.name,
			c.subject,
			l.status,
			COALESCE(l.created, '') AS created,
			COALESCE(l.updated, '') AS updated
		FROM campaign_send_ledger l
		INNER JOIN subscribers s ON l.subscriber_id = s.id
		INNER JOIN campaigns c ON l.campaign_id = c.id
		WHERE s.rowid = ?
		ORDER BY l.updated DESC, l.created DESC`, id); err != nil {
		c.log.Printf("error fetching subscriber campaign sends: %v", err)
		return models.SubscriberActivity{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "activity", "error", err.Error()))
	}

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
		  AND COALESCE(cv.is_suspected_privacy_open, 0) = 0
		GROUP BY c.id, c.uuid, c.name, c.subject
		ORDER BY last_viewed_at DESC`, id); err != nil {
		c.log.Printf("error fetching subscriber activity views: %v", err)
		return models.SubscriberActivity{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "activity", "error", err.Error()))
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
		return models.SubscriberActivity{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "activity", "error", err.Error()))
	}

	sendsJSON, _ := json.Marshal(sends)
	viewsJSON, _ := json.Marshal(views)
	clicksJSON, _ := json.Marshal(clicks)

	return models.SubscriberActivity{
		CampaignSends: sendsJSON,
		CampaignViews: viewsJSON,
		LinkClicks:    clicksJSON,
	}, nil
}

// DeleteOrphanSubscribers deletes orphan subscriber records (subscribers without lists).
func (c *Core) DeleteOrphanSubscribers() (int, error) {
	res, err := c.db.Exec(`DELETE FROM subscribers WHERE NOT EXISTS (SELECT 1 FROM subscriber_lists sl WHERE sl.subscriber_id = subscribers.id)`)
	if err != nil {
		c.log.Printf("error deleting orphan subscribers: %v", err)
		return 0, apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteBlocklistedSubscribers deletes blocklisted subscribers.
func (c *Core) DeleteBlocklistedSubscribers() (int, error) {
	res, err := c.db.Exec(`DELETE FROM subscribers WHERE status = 'blocklisted'`)
	if err != nil {
		c.log.Printf("error deleting blocklisted subscribers: %v", err)
		return 0, apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	n, _ := res.RowsAffected()
	return int(n), nil
}

func (c *Core) getSubscriberCount(searchStr, queryExp string, filters json.RawMessage, subStatus string, listIDs []int) (int, error) {
	whereSQL, args, err := c.subscriberFilterSQLite(searchStr, queryExp, filters, listIDs, subStatus)
	if err != nil {
		return 0, err
	}
	total := 0
	if err := c.db.Get(&total, `SELECT COUNT(*) FROM subscribers WHERE `+whereSQL, args...); err != nil {
		return 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}

	return total, nil
}

func (c *Core) getSubscriberSQLite(id int, uuid, email string) (models.Subscriber, error) {
	q := `SELECT rowid AS id, id AS record_id, created AS created_at, updated AS updated_at, uuid, email, phone, first_name, last_name, name, attribs, status FROM subscribers WHERE `
	args := []any{}
	switch {
	case id > 0:
		q += `rowid = ?`
		args = append(args, id)
	case uuid != "":
		if models.IsRFC4122UUID(uuid) {
			q += `uuid = ?`
		} else {
			q += `id = ?`
		}
		args = append(args, uuid)
	case email != "":
		q += `email = ?`
		args = append(args, email)
	default:
		return models.Subscriber{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.subscriber}"))
	}
	q += ` LIMIT 1`

	var rows []sqliteSubscriberRow
	if err := c.db.Select(&rows, q, args...); err != nil {
		if err == sql.ErrNoRows {
			return models.Subscriber{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name",
				fmt.Sprintf("{globals.terms.subscriber} (%d: %s%s)", id, uuid, email)))
		}
		c.log.Printf("error fetching subscriber: %v", err)
		return models.Subscriber{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
	}
	if len(rows) == 0 {
		return models.Subscriber{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name",
			fmt.Sprintf("{globals.terms.subscriber} (%d: %s%s)", id, uuid, email)))
	}

	subs := sqliteSubscriberRowsToModels(rows)
	if err := c.loadSubscriberListsSQLite(subs); err != nil {
		c.log.Printf("error loading subscriber lists: %v", err)
		return models.Subscriber{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", dbErr(err)))
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
		SELECT s.rowid AS subscriber_id,
			CASE
				WHEN EXISTS (
					SELECT 1 FROM subscriber_lists sl
					WHERE sl.subscriber_id = s.id
					  AND sl.list_id IN (` + sqlitePlaceholders(len(listIDs)) + `)
				) THEN 1 ELSE 0
			END AS has
		FROM subscribers s
		WHERE s.rowid IN (` + sqlitePlaceholders(len(subIDs)) + `)
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
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
	}

	for _, r := range rows {
		res[r.SubID] = r.Has
	}
	return res, nil
}

func (c *Core) getSubscribersByEmailSQLite(emails []string) (models.Subscribers, error) {
	if len(emails) == 0 {
		return nil, apperr.BadRequest(c.i18n.T("campaigns.noKnownSubsToTest"))
	}

	args := make([]any, 0, len(emails))
	for _, e := range emails {
		args = append(args, e)
	}

	var rows []sqliteSubscriberRow
	q := `SELECT rowid AS id, id AS record_id, created AS created_at, updated AS updated_at, uuid, email, phone, first_name, last_name, name, attribs, status FROM subscribers WHERE email IN (` + sqlitePlaceholders(len(emails)) + `) ORDER BY rowid`
	if err := c.db.Select(&rows, q, args...); err != nil {
		c.log.Printf("error fetching subscriber: %v", err)
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
	}
	if len(rows) == 0 {
		return nil, apperr.BadRequest(c.i18n.T("campaigns.noKnownSubsToTest"))
	}
	out := sqliteSubscriberRowsToModels(rows)

	if err := c.loadSubscriberListsSQLite(out); err != nil {
		c.log.Printf("error loading subscriber lists: %v", err)
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", dbErr(err)))
	}
	return out, nil
}

func (c *Core) getSubscribersByNormalizedPhonesSQLite(digits []string) (models.Subscribers, error) {
	if len(digits) == 0 {
		return nil, apperr.BadRequest(c.i18n.T("campaigns.noKnownSubsToTest"))
	}
	phoneExpr := `replace(replace(replace(replace(replace(replace(COALESCE(phone, ''), '+', ''), ' ', ''), '-', ''), '(', ''), ')', ''), '.', '')`
	args := make([]any, 0, len(digits))
	for _, d := range digits {
		if strings.TrimSpace(d) == "" {
			continue
		}
		args = append(args, d)
	}
	if len(args) == 0 {
		return nil, apperr.BadRequest(c.i18n.T("campaigns.noKnownSubsToTest"))
	}
	var rows []sqliteSubscriberRow
	q := `SELECT rowid AS id, id AS record_id, created AS created_at, updated AS updated_at, uuid, email, phone, first_name, last_name, name, attribs, status FROM subscribers WHERE ` + phoneExpr + ` IN (` + sqlitePlaceholders(len(args)) + `) ORDER BY rowid`
	if err := c.db.Select(&rows, q, args...); err != nil {
		c.log.Printf("error fetching subscriber: %v", err)
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscriber}", "error", dbErr(err)))
	}
	if len(rows) == 0 {
		return nil, apperr.BadRequest(c.i18n.T("campaigns.noKnownSubsToTest"))
	}
	out := sqliteSubscriberRowsToModels(rows)
	if err := c.loadSubscriberListsSQLite(out); err != nil {
		c.log.Printf("error loading subscriber lists: %v", err)
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", dbErr(err)))
	}
	return out, nil
}

func (c *Core) querySubscribersSQLite(searchStr, queryExp string, filters json.RawMessage, listIDs []int, subStatus string, order, orderBy string, offset, limit int) (models.Subscribers, int, error) {
	if order != SortAsc && order != SortDesc {
		order = SortDesc
	}

	orderMap := map[string]string{
		"email":      "subscribers.email",
		"status":     "subscribers.status",
		"name":       "subscribers.name",
		"created_at": "subscribers.created",
		"updated_at": "subscribers.updated",
		"id":         "subscribers.id",
	}
	sortCol, ok := orderMap[orderBy]
	if !ok {
		sortCol = "subscribers.updated"
	}

	whereSQL, args, err := c.subscriberFilterSQLite(searchStr, queryExp, filters, listIDs, subStatus)
	if err != nil {
		return nil, 0, err
	}
	total := 0
	if err := c.db.Get(&total, `SELECT COUNT(*) FROM subscribers WHERE `+whereSQL, args...); err != nil {
		c.log.Printf("error getting subscriber count: %v", err)
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}
	if total == 0 {
		return models.Subscribers{}, 0, nil
	}

	q := `SELECT subscribers.rowid AS id, subscribers.id AS record_id, subscribers.created AS created_at, subscribers.updated AS updated_at,
		subscribers.uuid, subscribers.email, subscribers.phone, subscribers.first_name, subscribers.last_name, subscribers.name, subscribers.attribs, subscribers.status
		FROM subscribers WHERE ` + whereSQL + ` ORDER BY ` + sortCol + ` ` + order
	if limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	} else if offset > 0 {
		q += ` LIMIT -1 OFFSET ?`
		args = append(args, offset)
	}

	var rows []sqliteSubscriberRow
	c.log.Printf("query subscribers sqlite: total=%d order_by=%q order=%q", total, sortCol, order)
	if err := c.db.Select(&rows, q, args...); err != nil {
		c.log.Printf("error querying subscribers: %v", err)
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
	}
	out := sqliteSubscriberRowsToModels(rows)
	if err := c.loadSubscriberListsSQLite(out); err != nil {
		c.log.Printf("error fetching subscriber lists: %v", err)
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
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
		sel := `SELECT rowid FROM subscribers WHERE uuid = ?`
		if !models.IsRFC4122UUID(uuid) {
			sel = `SELECT rowid FROM subscribers WHERE id = ?`
		}
		if err := c.db.Get(&sid, sel, uuid); err != nil {
			if err == sql.ErrNoRows {
				return []models.List{}, nil
			}
			c.log.Printf("error fetching lists for opt-in: %s", dbErr(err))
			return nil, err
		}
	}
	if sid == 0 {
		return []models.List{}, nil
	}

	q := `
		SELECT
			l.rowid AS id, l.id AS record_id, l.uuid, l.name, l.type, l.optin, l.status, l.tags, l.description,
			s.rowid AS subscriber_id,
			sl.status AS subscription_status,
			COALESCE(NULLIF(TRIM(sl.sms_status), ''), sl.status) AS subscription_sms_status
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
		q += ` AND l.type = ?`
		args = append(args, listType)
	}
	q += ` ORDER BY l.id`

	rows := []struct {
		ID                    int    `db:"id"`
		RecordID              string `db:"record_id"`
		UUID                  string `db:"uuid"`
		Name                  string `db:"name"`
		Type                  string `db:"type"`
		Optin                 string `db:"optin"`
		Status                string `db:"status"`
		Tags                  string `db:"tags"`
		Description           string `db:"description"`
		SubscriberID          int    `db:"subscriber_id"`
		SubscriptionStatus    string `db:"subscription_status"`
		SubscriptionSmsStatus string `db:"subscription_sms_status"`
	}{}

	if err := c.db.Select(&rows, q, args...); err != nil {
		c.log.Printf("error fetching lists for opt-in: %s", dbErr(err))
		return nil, err
	}

	out := make([]models.List, 0, len(rows))
	for _, r := range rows {
		tags := []string{}
		if strings.TrimSpace(r.Tags) != "" {
			var parsed []string
			if err := json.Unmarshal([]byte(r.Tags), &parsed); err == nil {
				tags = parsed
			}
		}

		out = append(out, models.List{
			Base:                  models.Base{ID: r.ID, RecordID: r.RecordID},
			UUID:                  r.UUID,
			Name:                  r.Name,
			Type:                  r.Type,
			Optin:                 r.Optin,
			Status:                r.Status,
			Tags:                  tags,
			Description:           r.Description,
			SubscriberID:          r.SubscriberID,
			SubscriptionStatus:    r.SubscriptionStatus,
			SubscriptionSmsStatus: r.SubscriptionSmsStatus,
		})
	}

	return out, nil
}

func (c *Core) exportSubscribersSQLite(searchStr, query string, filters json.RawMessage, subIDs, listIDs []int, subStatus string, batchSize int) (func() ([]models.SubscriberExport, error), error) {
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

	if _, err := c.getSubscriberCount(searchStr, cond, filters, subStatus, listIDs); err != nil {
		c.log.Printf("error getting subscriber count: %v", err)
		return nil, err
	}

	id := 0
	return func() ([]models.SubscriberExport, error) {
		whereSQL, args, err := c.subscriberFilterSQLite(searchStr, cond, filters, listIDs, subStatus)
		if err != nil {
			return nil, err
		}
		whereSQL = `subscribers.rowid > ? AND ` + whereSQL
		args = append([]any{id}, args...)

		if len(subIDs) > 0 {
			whereSQL += ` AND subscribers.rowid IN (` + sqlitePlaceholders(len(subIDs)) + `)`
			for _, sid := range subIDs {
				args = append(args, sid)
			}
		}

		q := `
			SELECT subscribers.rowid AS id, subscribers.id AS record_id, subscribers.uuid, subscribers.email, subscribers.phone, subscribers.name,
			       subscribers.first_name, subscribers.last_name,
			       subscribers.status, subscribers.attribs,
			       subscribers.created AS created_at,
			       subscribers.updated AS updated_at
			FROM subscribers
			WHERE ` + whereSQL + `
			ORDER BY subscribers.rowid ASC`
		if batchSize > 0 {
			q += ` LIMIT ?`
			args = append(args, batchSize)
		}

		var rows []sqliteSubscriberRow
		if err := c.db.Select(&rows, q, args...); err != nil {
			c.log.Printf("error exporting subscribers by query: %v", err)
			return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", dbErr(err)))
		}
		if len(rows) == 0 {
			return nil, nil
		}

		out := sqliteSubscriberRowsToExports(rows)
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

func (c *Core) findSubscriberIDsSQLite(searchStr, queryExp string, filters json.RawMessage, listIDs []int, subStatus string, offset, limit int) ([]int, error) {
	whereSQL, args, err := c.subscriberFilterSQLite(searchStr, queryExp, filters, listIDs, subStatus)
	if err != nil {
		return nil, err
	}
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

func (c *Core) subscriberFilterSQLite(searchStr, queryExp string, filters json.RawMessage, listIDs []int, subStatus string) (string, []any, error) {
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
		where = append(where, `(subscribers.name LIKE ? COLLATE NOCASE OR subscribers.email LIKE ? COLLATE NOCASE OR subscribers.phone LIKE ? COLLATE NOCASE)`)
		args = append(args, like, like, like)
	}

	filterSQL, filterArgs, err := c.CompileSubscriberFilters(filters)
	if err != nil {
		return "", nil, err
	}
	if filterSQL != "" {
		where = append(where, filterSQL)
		args = append(args, filterArgs...)
	}

	exp := strings.TrimSpace(sanitizeSQLExp(queryExp))
	if exp != "" && exp != "TRUE" {
		where = append(where, "("+exp+")")
	}

	return strings.Join(where, " AND "), args, nil
}
