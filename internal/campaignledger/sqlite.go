// Package campaignledger implements per-campaign send deduplication using SQLite
// and PocketBase collections (campaign_send_ledger).
//
// Double-send prevention (same campaign, same subscriber):
//   - The collection enforces a unique (campaign_id, subscriber_id) pair: at most one ledger row per pair.
//   - NextPending only selects status='pending'; after a successful send, MarkSent sets status='sent', so that
//     recipient is never claimed again for that campaign.
//   - Backfill and InsertPendingIfEligible use INSERT OR IGNORE, so they cannot add a second row for the same pair.
//
// Audit trail: application code does not delete ledger rows when a message is sent; rows remain (typically with
// status='sent') so you can see what was delivered. FinalizeCampaignStats only updates aggregate counts on campaigns.
// A maintenance cleanup may later prune old sent rows after campaign counters are reconciled.
// Note: PocketBase relation fields may still cascade-delete ledger rows if a parent campaign or subscriber record
// is removed from the DB; this package does not delete rows as part of the send pipeline.
package campaignledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/jmoiron/sqlx"
	"github.com/pocketbase/pocketbase/tools/security"
)

const (
	tableName         = "campaign_send_ledger"
	ledgerInsertChunk = 5000
)

func ledgerMessageID(ledgerID string) string {
	ledgerID = strings.TrimSpace(ledgerID)
	if ledgerID == "" {
		return ""
	}
	return fmt.Sprintf("%s@listpocket.local", ledgerID)
}

func normalizeMessageID(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "<>")
	return strings.TrimSpace(raw)
}

// SubscriberRow matches cmd.manager_store sqliteStoreSubscriberRow for conversion to models.Subscriber.
type SubscriberRow struct {
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

// RecoverStats describes the current ledger coverage for a campaign recovery.
type RecoverStats struct {
	Eligible      int   `json:"eligible"`
	LedgerTotal   int   `json:"ledger_total"`
	Sent          int   `json:"sent"`
	Pending       int   `json:"pending"`
	Inflight      int   `json:"inflight"`
	Missing       int   `json:"missing"`
	Inserted      int64 `json:"inserted,omitempty"`
	ResetInflight int64 `json:"reset_inflight,omitempty"`
}

// RecipientMembershipSQL filters list membership using email list status or SMS-specific
// sms_status (COALESCE with email status for rows before sms_status exists).
func RecipientMembershipSQL() string {
	return ` AND (
    (trim(COALESCE(c.messenger, '')) = '` + models.CampaignMessengerQuo + `' AND (
      (c.type = 'optin' AND COALESCE(sl.sms_status, sl.status) = 'unconfirmed' AND l.optin = 'double') OR
      ((c.type != 'optin' OR c.type IS NULL OR c.type = '') AND (
        (l.optin = 'double' AND COALESCE(sl.sms_status, sl.status) = 'confirmed') OR
        (l.optin != 'double' AND COALESCE(sl.sms_status, sl.status) != 'unsubscribed')
      ))
    ) AND trim(COALESCE(s.phone, '')) != '')
    OR
    (trim(COALESCE(c.messenger, '')) != '` + models.CampaignMessengerQuo + `' AND (
      (c.type = 'optin' AND sl.status = 'unconfirmed' AND l.optin = 'double') OR
      ((c.type != 'optin' OR c.type IS NULL OR c.type = '') AND (
        (l.optin = 'double' AND sl.status = 'confirmed') OR
        (l.optin != 'double' AND sl.status != 'unsubscribed')
      ))
    ))
  )`
}

func eligibleDistinctSQL(missingOnly bool) string {
	q := `
SELECT DISTINCT s.id
FROM campaign_lists cl
JOIN campaigns c ON c.id = cl.campaign_id
JOIN lists l ON l.id = cl.list_id
JOIN subscriber_lists sl ON sl.list_id = cl.list_id
JOIN subscribers s ON s.id = sl.subscriber_id
WHERE cl.campaign_id = ?
  AND s.status != 'blocklisted'`
	q += RecipientMembershipSQL()
	if missingOnly {
		q += `
  AND NOT EXISTS (
    SELECT 1 FROM ` + tableName + ` csl
    WHERE csl.campaign_id = c.id AND csl.subscriber_id = s.id
  )`
	}
	return q
}

func eligibleQueryArgs(db sqlx.ExtContext, campaignRecID string, missingOnly bool) (string, []any, error) {
	includeTags, excludeTags, err := campaignTagFilters(db, campaignRecID)
	if err != nil {
		return "", nil, err
	}
	tagClause, tagArgs := sqliteSubscriberTagFilterClause(includeTags, excludeTags)
	args := []any{campaignRecID}
	args = append(args, tagArgs...)
	return eligibleDistinctSQL(missingOnly) + tagClause, args, nil
}

func countEligibleSubscribers(db sqlx.ExtContext, campaignRecID string, missingOnly bool) (int, error) {
	q, args, err := eligibleQueryArgs(db, campaignRecID, missingOnly)
	if err != nil {
		return 0, err
	}
	var n int
	if err := sqlx.GetContext(context.Background(), db, &n, `SELECT COUNT(*) FROM (`+q+`)`, args...); err != nil {
		return 0, err
	}
	return n, nil
}

func insertEligiblePending(db sqlx.ExtContext, campaignRecID string) (int64, error) {
	ctx := context.Background()
	now := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	inner, innerArgs, err := eligibleQueryArgs(db, campaignRecID, true)
	if err != nil {
		return 0, err
	}

	q := `
INSERT OR IGNORE INTO ` + tableName + ` (id, campaign_id, subscriber_id, message_id, status, created, updated)
SELECT lid, ?, subscriber_id, lid || '@listpocket.local', 'pending', ?, ?
FROM (
  SELECT substr(lower(hex(randomblob(8))), 1, 15) AS lid, id AS subscriber_id
  FROM (
    ` + inner + `
    LIMIT ?
  )
)`

	var inserted int64
	for {
		args := make([]any, 0, 4+len(innerArgs))
		args = append(args, campaignRecID, now, now)
		args = append(args, innerArgs...)
		args = append(args, ledgerInsertChunk)
		res, err := db.ExecContext(ctx, q, args...)
		if err != nil {
			return inserted, err
		}
		n, _ := res.RowsAffected()
		inserted += n
		if n == 0 {
			break
		}
	}
	return inserted, nil
}

// RecoveryStats calculates campaign ledger coverage without changing data.
func RecoveryStats(db sqlx.ExtContext, campaignRecID string) (RecoverStats, error) {
	ctx := context.Background()
	out := RecoverStats{}

	eligible, err := countEligibleSubscribers(db, campaignRecID, false)
	if err != nil {
		return out, err
	}
	out.Eligible = eligible

	missing, err := countEligibleSubscribers(db, campaignRecID, true)
	if err != nil {
		return out, err
	}
	out.Missing = missing

	rows := []struct {
		Status string `db:"status"`
		Count  int    `db:"count"`
	}{}
	if err := sqlx.SelectContext(ctx, db, &rows, `
SELECT status, COUNT(1) AS count
FROM `+tableName+`
WHERE campaign_id = ?
GROUP BY status`, campaignRecID); err != nil {
		return out, err
	}
	for _, row := range rows {
		out.LedgerTotal += row.Count
		switch row.Status {
		case "sent":
			out.Sent = row.Count
		case "pending":
			out.Pending = row.Count
		case "inflight":
			out.Inflight = row.Count
		}
	}

	return out, nil
}

// RecoverMissing inserts pending ledger rows for currently eligible subscribers
// who are not yet represented in the ledger, resets stranded inflight rows, and
// reconciles campaign counters. Existing sent rows are preserved.
func RecoverMissing(db sqlx.ExtContext, campaignRowID int, campaignRecID string) (RecoverStats, error) {
	out, err := RecoveryStats(db, campaignRecID)
	if err != nil {
		return out, err
	}

	inserted, err := insertEligiblePending(db, campaignRecID)
	if err != nil {
		return out, err
	}
	out.Inserted = inserted

	reset, err := ResetInflight(db, campaignRecID)
	if err != nil {
		return out, err
	}
	out.ResetInflight = reset

	if err := FinalizeCampaignStats(db, campaignRowID, campaignRecID); err != nil {
		return out, err
	}

	updated, err := RecoveryStats(db, campaignRecID)
	if err != nil {
		return out, err
	}
	updated.Inserted = out.Inserted
	updated.ResetInflight = out.ResetInflight
	return updated, nil
}

// BackfillIfEmpty inserts one ledger row per eligible (campaign, subscriber) pair when the
// ledger has no rows yet for this campaign. Returns true if a backfill ran.
func BackfillIfEmpty(db sqlx.ExtContext, campaignRowID int, campaignRecID string) (bool, error) {
	ctx := context.Background()
	var n int
	if err := sqlx.GetContext(ctx, db, &n, `SELECT COUNT(1) FROM `+tableName+` WHERE campaign_id = ?`, campaignRecID); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}

	if _, err := insertEligiblePending(db, campaignRecID); err != nil {
		return false, err
	}

	if _, err := db.ExecContext(ctx, `
UPDATE campaigns
SET to_send = (SELECT COUNT(1) FROM `+tableName+` WHERE campaign_id = ?),
    updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
WHERE rowid = ?`, campaignRecID, campaignRowID); err != nil {
		return false, err
	}

	return true, nil
}

// NextPending claims up to limit pending rows (pending → inflight) and returns those subscribers.
// hasMore is true if additional pending rows remain after this claim. Uses a transaction so
// overlapping NextSubscribers calls cannot claim the same recipients.
func NextPending(db *pbdb.DB, campaignRecID string, limit int) ([]SubscriberRow, bool, error) {
	ctx := context.Background()
	if limit < 1 {
		limit = 1
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()

	var rowIDs []int64
	if err := sqlx.SelectContext(ctx, tx, &rowIDs, `
SELECT rowid FROM `+tableName+`
WHERE campaign_id = ? AND status = 'pending'
ORDER BY created, id
LIMIT ?`, campaignRecID, limit+1); err != nil {
		return nil, false, err
	}
	hasMore := len(rowIDs) > limit
	if hasMore {
		rowIDs = rowIDs[:limit]
	}
	if len(rowIDs) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}

	ph := placeholders(len(rowIDs))
	args := make([]any, 0, len(rowIDs))
	for _, id := range rowIDs {
		args = append(args, id)
	}
	q := `UPDATE ` + tableName + ` SET status = 'inflight', updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now')) WHERE rowid IN (` + ph + `)`
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return nil, false, err
	}

	q2 := `
SELECT s.rowid AS id,
       s.id AS record_id,
       s.created AS created_at,
       s.updated AS updated_at,
       s.uuid,
       s.email,
       COALESCE(s.phone, '') AS phone,
       s.first_name,
       s.last_name,
       s.name,
       s.attribs,
       s.status
FROM ` + tableName + ` csl
JOIN subscribers s ON s.id = csl.subscriber_id
WHERE csl.campaign_id = ? AND csl.rowid IN (` + ph + `)`
	args2 := make([]any, 0, len(rowIDs)+1)
	args2 = append(args2, campaignRecID)
	for _, id := range rowIDs {
		args2 = append(args2, id)
	}
	var rows []SubscriberRow
	if err := sqlx.SelectContext(ctx, tx, &rows, q2, args2...); err != nil {
		return nil, false, err
	}

	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return rows, hasMore, nil
}

func placeholders(n int) string {
	if n < 1 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// RollbackInflight marks an inflight row back to pending after a failed send attempt.
func RollbackInflight(db sqlx.ExecerContext, campaignRecID, subscriberRecID string) error {
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
UPDATE `+tableName+`
SET status = 'pending', updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
WHERE campaign_id = ? AND subscriber_id = ? AND status = 'inflight'`, campaignRecID, subscriberRecID)
	return err
}

// ResetInflight rolls every inflight row for a campaign back to pending. Called when a
// pipe starts so rows stranded by a previous paused/cancelled/crashed run get picked up
// again, and when a pipe's cleanup routine reaches a naturally finished campaign so no
// subscriber is left permanently claimed as inflight. Returns the number of rows reset.
func ResetInflight(db sqlx.ExecerContext, campaignRecID string) (int64, error) {
	ctx := context.Background()
	res, err := db.ExecContext(ctx, `
UPDATE `+tableName+`
SET status = 'pending', updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
WHERE campaign_id = ? AND status = 'inflight'`, campaignRecID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// MarkInflightSent flips every inflight row for a campaign to sent. Used by the admin
// "resolve stranded inflight" flow when the operator is confident the messages did go
// out (e.g. Quo accepted them) but MarkSent never ran to close the ledger loop.
// Returns the number of rows updated.
func MarkInflightSent(db sqlx.ExecerContext, campaignRecID string) (int64, error) {
	ctx := context.Background()
	res, err := db.ExecContext(ctx, `
UPDATE `+tableName+`
SET status = 'sent', updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
WHERE campaign_id = ? AND status = 'inflight'`, campaignRecID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// MarkSent sets the ledger row to sent for this campaign + subscriber pair.
// Only rows in status inflight are updated, so a row already marked sent is never changed again.
func MarkSent(db sqlx.ExecerContext, campaignRecID, subscriberRecID string) error {
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
UPDATE `+tableName+`
SET status = 'sent', updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
WHERE campaign_id = ? AND subscriber_id = ? AND status = 'inflight'`, campaignRecID, subscriberRecID)
	return err
}

// SyncToSendFromLedger sets campaigns.to_send from the ledger row count when the ledger
// already has rows (e.g. subscriber_lists hooks ran before the first backfill).
func SyncToSendFromLedger(db sqlx.ExecerContext, campaignRowID int, campaignRecID string) error {
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
UPDATE campaigns SET
  to_send = (SELECT COUNT(1) FROM `+tableName+` WHERE campaign_id = ?),
  updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
WHERE rowid = ?
  AND EXISTS (SELECT 1 FROM `+tableName+` WHERE campaign_id = ? LIMIT 1)`, campaignRecID, campaignRowID, campaignRecID)
	return err
}

// FinalizeCampaignStats writes authoritative to_send and sent counts from the ledger
// into the campaigns row (by rowid).
func FinalizeCampaignStats(db sqlx.ExecerContext, campaignRowID int, campaignRecID string) error {
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
UPDATE campaigns SET
  to_send = (SELECT COUNT(1) FROM `+tableName+` WHERE campaign_id = ?),
  sent = (SELECT COUNT(1) FROM `+tableName+` WHERE campaign_id = ? AND status = 'sent'),
  updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
WHERE rowid = ?`, campaignRecID, campaignRecID, campaignRowID)
	return err
}

// CleanupSentOlderThan reconciles campaign counters from the ledger and then deletes old
// sent rows for completed campaigns.
//
// Only ledger rows that are:
//   - status = 'sent'
//   - older than olderThan (based on updated timestamp)
//   - attached to campaigns in status finished/cancelled
//
// are deleted.
//
// It returns (deletedRows, reconciledCampaigns, error).
func CleanupSentOlderThan(db *pbdb.DB, olderThan time.Time) (int, int, error) {
	ctx := context.Background()
	cutoff := olderThan.UTC().Format("2006-01-02 15:04:05.000Z")

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	var campaigns []struct {
		RowID int    `db:"rowid"`
		ID    string `db:"id"`
	}
	if err := sqlx.SelectContext(ctx, tx, &campaigns, `
SELECT DISTINCT c.rowid, c.id
FROM campaigns c
JOIN `+tableName+` l ON l.campaign_id = c.id
WHERE c.status IN ('finished', 'cancelled')
  AND l.status = 'sent'
  AND datetime(l.updated) <= datetime(?)`, cutoff); err != nil {
		return 0, 0, err
	}

	for _, c := range campaigns {
		if err := FinalizeCampaignStats(tx, c.RowID, c.ID); err != nil {
			return 0, 0, err
		}
	}

	res, err := tx.ExecContext(ctx, `
DELETE FROM `+tableName+`
WHERE status = 'sent'
  AND datetime(updated) <= datetime(?)
  AND campaign_id IN (
    SELECT id FROM campaigns WHERE status IN ('finished', 'cancelled')
  )`, cutoff)
	if err != nil {
		return 0, 0, err
	}

	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}

	return int(n), len(campaigns), nil
}

// InsertPendingIfEligible adds a ledger row when subscriber_lists gains an eligible membership
// for a list that belongs to an active campaign. Uses INSERT OR IGNORE for idempotency.
func InsertPendingIfEligible(db sqlx.ExtContext, listRecordID, subscriberRecordID string) error {
	ctx := context.Background()

	q := `
SELECT c.id
FROM campaigns c
JOIN campaign_lists cl ON cl.campaign_id = c.id AND cl.list_id = ?
JOIN lists l ON l.id = cl.list_id
JOIN subscriber_lists sl ON sl.list_id = cl.list_id AND sl.subscriber_id = ?
JOIN subscribers s ON s.id = sl.subscriber_id
WHERE c.status IN ('scheduled', 'running', 'paused')
  AND s.status != 'blocklisted'`
	q += RecipientMembershipSQL()

	var campaignIDs []string
	if err := sqlx.SelectContext(ctx, db, &campaignIDs, q, listRecordID, subscriberRecordID); err != nil {
		return err
	}

	filteredCampaignIDs := make([]string, 0, len(campaignIDs))
	for _, campaignID := range campaignIDs {
		includeTags, excludeTags, err := campaignTagFilters(db, campaignID)
		if err != nil {
			return err
		}
		matches, err := subscriberMatchesTagFilters(db, subscriberRecordID, includeTags, excludeTags)
		if err != nil {
			return err
		}
		if matches {
			filteredCampaignIDs = append(filteredCampaignIDs, campaignID)
		}
	}
	campaignIDs = filteredCampaignIDs

	now := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	for _, cid := range campaignIDs {
		id := security.RandomString(15)
		messageID := ledgerMessageID(id)
		_, err := db.ExecContext(ctx, `
INSERT OR IGNORE INTO `+tableName+` (id, campaign_id, subscriber_id, message_id, status, created, updated)
VALUES (?, ?, ?, ?, 'pending', ?, ?)`, id, cid, subscriberRecordID, messageID, now, now)
		if err != nil && !isUniqueViolation(err) {
			return err
		}
	}
	return nil
}

// GetMessageID returns the campaign_send_ledger message_id for the given campaign/subscriber pair.
// Legacy rows with empty message_id are lazily populated using a deterministic value derived from ledger id.
func GetMessageID(db sqlx.ExtContext, campaignRecID, subscriberRecID string) (string, error) {
	ctx := context.Background()
	campaignRecID = strings.TrimSpace(campaignRecID)
	subscriberRecID = strings.TrimSpace(subscriberRecID)
	if campaignRecID == "" || subscriberRecID == "" {
		return "", nil
	}

	row := struct {
		ID        string `db:"id"`
		MessageID string `db:"message_id"`
	}{}
	err := sqlx.GetContext(ctx, db, &row, `
SELECT id, COALESCE(message_id, '') AS message_id
FROM `+tableName+`
WHERE campaign_id = ? AND subscriber_id = ?
LIMIT 1`, campaignRecID, subscriberRecID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}

	messageID := normalizeMessageID(row.MessageID)
	if messageID != "" {
		return messageID, nil
	}

	messageID = ledgerMessageID(row.ID)
	if messageID == "" {
		return "", nil
	}
	_, err = db.ExecContext(ctx, `
UPDATE `+tableName+`
SET message_id = ?, updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
WHERE id = ? AND (message_id IS NULL OR trim(message_id) = '')`, messageID, row.ID)
	if err != nil {
		return "", err
	}

	return messageID, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}

func campaignTagFilters(db sqlx.ExtContext, campaignRecID string) ([]string, []string, error) {
	ctx := context.Background()
	row := struct {
		Include []byte `db:"include_tags"`
		Exclude []byte `db:"exclude_tags"`
	}{}
	if err := sqlx.GetContext(ctx, db, &row, `
SELECT
  COALESCE(c.include_tags, '[]') AS include_tags,
  COALESCE(c.exclude_tags, '[]') AS exclude_tags
FROM campaigns c
WHERE c.id = ?
LIMIT 1`, campaignRecID); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such column") {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return decodeTagFilterJSON(row.Include), decodeTagFilterJSON(row.Exclude), nil
}

func decodeTagFilterJSON(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var tags []string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil
	}
	return normalizeTagFilterSet(tags)
}

func normalizeTagFilterSet(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func sqliteSubscriberTagFilterClause(includeTags, excludeTags []string) (string, []any) {
	args := []any{}
	var b strings.Builder

	if len(includeTags) > 0 {
		b.WriteString(`
  AND EXISTS (
    SELECT 1
    FROM json_each(COALESCE(json_extract(s.attribs, '$.tags'), '[]')) jt
    WHERE lower(trim(CAST(jt.value AS TEXT))) IN (` + placeholders(len(includeTags)) + `)
  )`)
		for _, tag := range includeTags {
			args = append(args, tag)
		}
	}

	if len(excludeTags) > 0 {
		b.WriteString(`
  AND NOT EXISTS (
    SELECT 1
    FROM json_each(COALESCE(json_extract(s.attribs, '$.tags'), '[]')) jt
    WHERE lower(trim(CAST(jt.value AS TEXT))) IN (` + placeholders(len(excludeTags)) + `)
  )`)
		for _, tag := range excludeTags {
			args = append(args, tag)
		}
	}

	return b.String(), args
}

func subscriberMatchesTagFilters(db sqlx.ExtContext, subscriberRecID string, includeTags, excludeTags []string) (bool, error) {
	if len(includeTags) == 0 && len(excludeTags) == 0 {
		return true, nil
	}
	ctx := context.Background()

	args := []any{subscriberRecID}
	q := `
SELECT EXISTS (
  SELECT 1
  FROM subscribers s
  WHERE s.id = ?`

	includeClause, includeArgs := sqliteSubscriberTagFilterClause(includeTags, nil)
	excludeClause, excludeArgs := sqliteSubscriberTagFilterClause(nil, excludeTags)
	q += includeClause
	q += excludeClause
	q += `
)`
	args = append(args, includeArgs...)
	args = append(args, excludeArgs...)

	var matches bool
	if err := sqlx.GetContext(ctx, db, &matches, q, args...); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such function: json_extract") {
			return true, nil
		}
		return false, err
	}
	return matches, nil
}
