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
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/jmoiron/sqlx"
	"github.com/pocketbase/pocketbase/tools/security"
)

const tableName = "campaign_send_ledger"

// SubscriberRow matches cmd.manager_store sqliteStoreSubscriberRow for conversion to models.Subscriber.
type SubscriberRow struct {
	ID        int    `db:"id"`
	RecordID  string `db:"record_id"`
	CreatedAt string `db:"created_at"`
	UpdatedAt string `db:"updated_at"`
	UUID      string `db:"uuid"`
	Email     string `db:"email"`
	FirstName string `db:"first_name"`
	LastName  string `db:"last_name"`
	Name      string `db:"name"`
	Attribs   []byte `db:"attribs"`
	Status    string `db:"status"`
}

// BackfillIfEmpty inserts one ledger row per eligible (campaign, subscriber) pair when the
// ledger has no rows yet for this campaign. Returns true if a backfill ran.
func BackfillIfEmpty(db sqlx.ExtContext, campaignRowID int, campaignRecID, campaignType string) (bool, error) {
	ctx := context.Background()
	var n int
	if err := sqlx.GetContext(ctx, db, &n, `SELECT COUNT(1) FROM `+tableName+` WHERE campaign_id = ?`, campaignRecID); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}

	q := `
SELECT DISTINCT s.id
FROM campaign_lists cl
JOIN lists l ON l.id = cl.list_id
JOIN subscriber_lists sl ON sl.list_id = cl.list_id
JOIN subscribers s ON s.id = sl.subscriber_id
WHERE cl.campaign_id = ?
  AND s.status != 'blocklisted'
  AND (
    (? = 'optin' AND sl.status = 'unconfirmed' AND l.optin = 'double') OR
    (? != 'optin' AND (
      (l.optin = 'double' AND sl.status = 'confirmed') OR
      (l.optin != 'double' AND sl.status != 'unsubscribed')
    ))
  )`

	var subIDs []string
	if err := sqlx.SelectContext(ctx, db, &subIDs, q, campaignRecID, campaignType, campaignType); err != nil {
		return false, err
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	for _, sid := range subIDs {
		id := security.RandomString(15)
		_, err := db.ExecContext(ctx, `
INSERT OR IGNORE INTO `+tableName+` (id, campaign_id, subscriber_id, status, created, updated)
VALUES (?, ?, ?, 'pending', ?, ?)`, id, campaignRecID, sid, now, now)
		if err != nil {
			return false, err
		}
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

	var pendingTotal int
	if err := tx.GetContext(ctx, &pendingTotal, `
SELECT COUNT(1) FROM `+tableName+` WHERE campaign_id = ? AND status = 'pending'`, campaignRecID); err != nil {
		return nil, false, err
	}
	hasMore := pendingTotal > limit

	var rowIDs []int64
	if err := sqlx.SelectContext(ctx, tx, &rowIDs, `
SELECT rowid FROM `+tableName+`
WHERE campaign_id = ? AND status = 'pending'
ORDER BY created, id
LIMIT ?`, campaignRecID, limit); err != nil {
		return nil, false, err
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
  AND s.status != 'blocklisted'
  AND (
    (c.type = 'optin' AND sl.status = 'unconfirmed' AND l.optin = 'double') OR
    (c.type != 'optin' AND (
      (l.optin = 'double' AND sl.status = 'confirmed') OR
      (l.optin != 'double' AND sl.status != 'unsubscribed')
    ))
  )`

	var campaignIDs []string
	if err := sqlx.SelectContext(ctx, db, &campaignIDs, q, listRecordID, subscriberRecordID); err != nil {
		return err
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	for _, cid := range campaignIDs {
		id := security.RandomString(15)
		_, err := db.ExecContext(ctx, `
INSERT OR IGNORE INTO `+tableName+` (id, campaign_id, subscriber_id, status, created, updated)
VALUES (?, ?, ?, 'pending', ?, ?)`, id, cid, subscriberRecordID, now, now)
		if err != nil && !isUniqueViolation(err) {
			return err
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}
