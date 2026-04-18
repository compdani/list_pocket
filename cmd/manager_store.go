package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/campaignledger"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/events"
	"github.com/compdani/list_pocket/internal/manager"
	"github.com/compdani/list_pocket/internal/media"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
	null "gopkg.in/volatiletech/null.v6"
)

// store implements DataSource over the primary
// database.
type store struct {
	db      *pbdb.DB
	core    *core.Core
	media   media.Store
	log     *log.Logger
	events  *events.Events
	verbose bool
}

const sqliteBatchCursorAttribKey = "_sqlite_batch_cursor"

type sqliteBatchCursor struct {
	LastCreated string `json:"last_created"`
	LastID      string `json:"last_id"`
	MaxCreated  string `json:"max_created"`
	MaxID       string `json:"max_id"`
}

type sqliteStoreSubscriberRow struct {
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

func newManagerStore(db *pbdb.DB, c *core.Core, m media.Store, l *log.Logger, ev *events.Events, verbose bool) *store {
	return &store{
		db:      db,
		core:    c,
		media:   m,
		log:     l,
		events:  ev,
		verbose: verbose,
	}
}

func (s *store) publishCampaignStatsEvent(reason string, campaignID int) {
	if s.events == nil {
		return
	}

	if err := s.events.Publish(events.Event{
		Type: events.TypeCampaignStats,
		Data: map[string]any{
			"campaign_id": campaignID,
			"reason":      reason,
		},
	}); err != nil {
		s.log.Printf("error publishing campaign stats event: %v", err)
	}
}

func parseStoreNullTime(value string) null.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return null.Time{}
	}

	t, err := time.Parse("2006-01-02 15:04:05.000Z", value)
	if err != nil {
		t, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return null.Time{}
		}
	}

	return null.NewTime(t, true)
}

func sqliteParseCampaignAttribs(raw []byte) models.JSON {
	out := models.JSON{}
	if len(raw) == 0 || string(raw) == "null" {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		return models.JSON{}
	}
	return out
}

func sqliteCampaignBatchCursor(raw []byte) sqliteBatchCursor {
	attribs := sqliteParseCampaignAttribs(raw)
	value, ok := attribs[sqliteBatchCursorAttribKey]
	if !ok {
		return sqliteBatchCursor{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return sqliteBatchCursor{}
	}
	out := sqliteBatchCursor{}
	if err := json.Unmarshal(encoded, &out); err != nil {
		return sqliteBatchCursor{}
	}
	return out
}

func sqliteSetCampaignBatchCursor(raw []byte, cursor sqliteBatchCursor) []byte {
	attribs := sqliteParseCampaignAttribs(raw)
	attribs[sqliteBatchCursorAttribKey] = cursor
	out, err := json.Marshal(attribs)
	if err != nil {
		return raw
	}
	return out
}

func sqliteAdvanceBatchCursor(cursor sqliteBatchCursor, rows []sqliteStoreSubscriberRow, count int) sqliteBatchCursor {
	if count < 1 || len(rows) == 0 {
		return cursor
	}
	if count > len(rows) {
		count = len(rows)
	}

	last := rows[count-1]
	cursor.LastCreated = last.CreatedAt
	cursor.LastID = strings.TrimSpace(last.RecordID)
	return cursor
}

func (s *store) sqliteCampaignRecordID(campID int) (string, error) {
	var recID string
	if err := s.db.Get(&recID, `SELECT id FROM campaigns WHERE rowid = ?`, campID); err != nil {
		return "", err
	}
	return recID, nil
}

func (s *store) sqliteSubscriberRecordID(subID int64) (string, error) {
	var recID string
	if err := s.db.Get(&recID, `SELECT id FROM subscribers WHERE rowid = ?`, subID); err != nil {
		return "", err
	}
	return recID, nil
}

// NextCampaigns retrieves active campaigns ready to be processed excluding
// campaigns that are also being processed. Additionally, it takes a map of campaignID:sentCount
// of campaigns that are being processed and updates them in the DB.
func (s *store) NextCampaigns(currentIDs []int64, sentCounts []int64) ([]*models.Campaign, error) {
	if s.verbose {
		s.log.Printf("manager store sqlite: next campaigns current_ids=%v sent_counts=%v", currentIDs, sentCounts)
	}
	return s.nextCampaignsSQLite(currentIDs, sentCounts)
}

// NextSubscribers retrieves a subset of subscribers of a given campaign.
// Since batches are processed sequentially, the retrieval is ordered by ID,
// and every batch takes the last ID of the last batch and fetches the next
// batch above that.
func (s *store) NextSubscribers(campID, limit int) ([]models.Subscriber, bool, error) {
	return s.nextSubscribersSQLite(campID, limit)
}

// GetCampaign fetches a campaign from the database.
func (s *store) GetCampaign(campID int) (*models.Campaign, error) {
	recordID, err := s.sqliteCampaignRecordID(campID)
	if err != nil {
		return nil, err
	}

	c, err := s.core.GetCampaign(recordID, "", "")
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateCampaignStatus updates a campaign's status.
func (s *store) UpdateCampaignStatus(campID int, status string) error {
	_, err := s.db.Exec(`
			UPDATE campaigns
			SET status=(CASE WHEN send_at IS NOT NULL AND send_at != '' AND ? = 'running' THEN 'scheduled' ELSE ? END),
			    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE rowid = ?`,
		status, status, campID)
	if err == nil {
		s.publishCampaignStatsEvent("status", campID)
	}
	return err
}

func (s *store) ScheduleCampaignBatch(campID int, sendAt time.Time) error {
	_, err := s.db.Exec(`
			UPDATE campaigns
			SET status = 'scheduled',
			    send_at = ?,
			    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE rowid = ?`,
		sendAt.UTC().Format("2006-01-02 15:04:05.000Z"), campID)
	if err == nil {
		s.publishCampaignStatsEvent("schedule-batch", campID)
	}
	return err
}

// UpdateCampaignCounts updates a campaign's status.
func (s *store) UpdateCampaignCounts(campID int, toSend int, sent int, lastSubID int) error {
	_, err := s.db.Exec(`
			UPDATE campaigns SET
				to_send=(CASE WHEN ? != 0 THEN ? ELSE to_send END),
				sent=sent+?,
				updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE rowid=?`,
		toSend, toSend, sent, campID)
	if err == nil {
		s.publishCampaignStatsEvent("counts", campID)
	}
	return err
}

// MarkCampaignLedgerSent records a successful delivery in campaign_send_ledger (SQLite only).
func (s *store) MarkCampaignLedgerSent(campaignID int, subscriberRecordID string) error {
	if strings.TrimSpace(subscriberRecordID) == "" {
		return nil
	}
	campaignRecID, err := s.sqliteCampaignRecordID(campaignID)
	if err != nil {
		return err
	}
	err = campaignledger.MarkSent(s.db, campaignRecID, subscriberRecordID)
	if err == nil {
		s.publishCampaignStatsEvent("ledger-sent", campaignID)
	}
	return err
}

// RollbackCampaignLedgerInflight resets inflight → pending after a failed delivery (SQLite only).
func (s *store) RollbackCampaignLedgerInflight(campaignID int, subscriberRecordID string) error {
	if strings.TrimSpace(subscriberRecordID) == "" {
		return nil
	}
	campaignRecID, err := s.sqliteCampaignRecordID(campaignID)
	if err != nil {
		return err
	}
	return campaignledger.RollbackInflight(s.db, campaignRecID, subscriberRecordID)
}

// ResetCampaignLedgerInflight rolls every inflight ledger row for the campaign back to
// pending. Called when a pipe starts or finishes so stranded rows from a previous run
// (paused/cancelled/crashed or dropped queue) are picked up again instead of being
// permanently claimed as inflight. Returns the number of rows reset.
func (s *store) ResetCampaignLedgerInflight(campaignID int) (int64, error) {
	campaignRecID, err := s.sqliteCampaignRecordID(campaignID)
	if err != nil {
		return 0, err
	}
	n, err := campaignledger.ResetInflight(s.db, campaignRecID)
	if err == nil && n > 0 {
		s.publishCampaignStatsEvent("ledger-reset-inflight", campaignID)
	}
	return n, err
}

// MarkSMSUnsendable opts the subscriber (matched by normalized phone) out of
// every list's SMS status, without touching email status. Used by the manager
// when Quo (or another SMS provider) returns a permanent per-recipient error
// such as "International Messaging Not Allowed".
func (s *store) MarkSMSUnsendable(phone string) (int64, error) {
	if strings.TrimSpace(phone) == "" {
		return 0, nil
	}
	return s.core.SMSOptOutSubscriberByPhone(phone)
}

// FinalizeCampaignLedgerStats writes final to_send and sent from the ledger into campaigns (SQLite only).
func (s *store) FinalizeCampaignLedgerStats(campaignID int) error {
	campaignRecID, err := s.sqliteCampaignRecordID(campaignID)
	if err != nil {
		return err
	}
	var n int
	if err := s.db.Get(&n, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ?`, campaignRecID); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	err = campaignledger.FinalizeCampaignStats(s.db, campaignID, campaignRecID)
	if err == nil {
		s.publishCampaignStatsEvent("finalize-ledger", campaignID)
	}
	return err
}

func (s *store) nextCampaignsSQLite(currentIDs []int64, sentCounts []int64) ([]*models.Campaign, error) {
	statsChanged := false

	for i, id := range currentIDs {
		if i >= len(sentCounts) || sentCounts[i] == 0 {
			continue
		}

		if _, err := s.db.Exec(`
			UPDATE campaigns
			SET sent = sent + ?,
			    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE rowid = ?`,
			sentCounts[i], id); err != nil {
			return nil, err
		}
		statsChanged = true
	}

	base := `
		SELECT c.rowid
		FROM campaigns c
		WHERE (
			c.status='running' OR
			(c.status='scheduled' AND c.send_at IS NOT NULL AND c.send_at != '' AND datetime('now') >= datetime(c.send_at))
		)
	`

	args := make([]any, 0, len(currentIDs))
	if len(currentIDs) > 0 {
		base += " AND c.rowid NOT IN (" + placeholders(len(currentIDs)) + ")"
		for _, id := range currentIDs {
			args = append(args, id)
		}
	}

	base += " ORDER BY c.rowid"

	var rowIDs []int
	if err := s.db.Select(&rowIDs, base, args...); err != nil {
		return nil, err
	}
	if s.verbose {
		s.log.Printf("manager store sqlite: runnable campaign rowids=%v", rowIDs)
	}

	campaigns := make([]*models.Campaign, 0, len(rowIDs))
	for _, rowID := range rowIDs {
		campaignRecID, err := s.sqliteCampaignRecordID(rowID)
		if err != nil {
			return nil, err
		}

		campaign, err := s.core.GetCampaign(campaignRecID, "", "")
		if err != nil {
			return nil, err
		}
		campaigns = append(campaigns, &campaign)
	}

	for _, c := range campaigns {
		campaignRecID, err := s.sqliteCampaignRecordID(c.ID)
		if err != nil {
			return nil, err
		}

		includeTags := normalizeCampaignFilterTags(c.IncludeTags)
		excludeTags := normalizeCampaignFilterTags(c.ExcludeTags)
		tagClause, tagArgs := sqliteSubscriberTagFilterClause(includeTags, excludeTags, "s")

		var meta struct {
			ToSend int `db:"to_send"`
		}
		metaQuery := `
			SELECT
				COUNT(DISTINCT s.rowid) AS to_send
			FROM campaign_lists cl
			JOIN campaigns c ON c.id = cl.campaign_id
			JOIN lists l ON l.id = cl.list_id
			JOIN subscriber_lists sl ON sl.list_id = cl.list_id
			JOIN subscribers s ON s.id = sl.subscriber_id
			WHERE cl.campaign_id = ?
			  AND s.status != 'blocklisted'
		`
		metaQuery += campaignledger.RecipientMembershipSQL()
		metaQuery += tagClause
		metaArgs := []any{campaignRecID}
		metaArgs = append(metaArgs, tagArgs...)
		if err := s.db.Get(&meta, metaQuery, metaArgs...); err != nil {
			return nil, err
		}
		cursor := sqliteBatchCursor{}
		cursorQuery := `
			SELECT
				COALESCE(MAX(created), '') AS max_created,
				COALESCE(MAX(id), '') AS max_id
			FROM (
				SELECT s.created, s.id
				FROM campaign_lists cl
				JOIN campaigns c ON c.id = cl.campaign_id
				JOIN lists l ON l.id = cl.list_id
				JOIN subscriber_lists sl ON sl.list_id = cl.list_id
				JOIN subscribers s ON s.id = sl.subscriber_id
				WHERE cl.campaign_id = ?
				  AND s.status != 'blocklisted'
				` + campaignledger.RecipientMembershipSQL() + `
				` + tagClause + `
				GROUP BY s.id
				ORDER BY s.created DESC, s.id DESC
				LIMIT 1
			) latest
		`
		cursorArgs := []any{campaignRecID}
		cursorArgs = append(cursorArgs, tagArgs...)
		if err := s.db.Get(&cursor, cursorQuery, cursorArgs...); err != nil {
			return nil, err
		}
		currentAttribs := sqliteCampaignBatchCursor(nil)
		if len(c.Attribs) > 0 {
			if raw, err := json.Marshal(c.Attribs); err == nil {
				currentAttribs = sqliteCampaignBatchCursor(raw)
			}
		}
		cursor.LastCreated = ""
		cursor.LastID = ""
		if cursor.MaxCreated == "" {
			cursor.MaxID = ""
		}
		if currentAttribs.LastCreated != "" || currentAttribs.LastID != "" || currentAttribs.MaxCreated != "" || currentAttribs.MaxID != "" {
			// reset any stale cursor values when campaign (re)starts.
		}
		var attribsRaw []byte
		if raw, err := json.Marshal(c.Attribs); err == nil {
			attribsRaw = raw
		}
		attribsRaw = sqliteSetCampaignBatchCursor(attribsRaw, cursor)
		if _, err := s.db.Exec(`
				UPDATE campaigns
				SET to_send = ?,
				    status = (CASE WHEN status != 'running' THEN 'running' ELSE status END),
				    attribs = ?,
				    started_at = (CASE WHEN started_at IS NULL THEN (strftime('%Y-%m-%d %H:%M:%fZ')) ELSE started_at END),
				    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
				WHERE rowid = ?`,
			meta.ToSend, string(attribsRaw), c.ID); err != nil {
			return nil, err
		}
		statsChanged = true
		c.ToSend = meta.ToSend

		var mediaIDs []int64
		if err := s.db.Select(&mediaIDs, `
			SELECT m.rowid
			FROM campaign_media cm
			JOIN media m ON m.id = cm.media_id
			WHERE cm.campaign_id = ? AND cm.media_id IS NOT NULL
			ORDER BY m.rowid`, campaignRecID); err != nil {
			return nil, err
		}
		c.MediaIDs = mediaIDs
	}

	if statsChanged {
		s.publishCampaignStatsEvent("scan", 0)
	}

	return campaigns, nil
}

func (s *store) nextSubscribersSQLite(campID, limit int) ([]models.Subscriber, bool, error) {
	campaignRecID, err := s.sqliteCampaignRecordID(campID)
	if err != nil {
		return nil, false, err
	}

	if _, err := campaignledger.BackfillIfEmpty(s.db, campID, campaignRecID); err != nil {
		return nil, false, err
	}
	if err := campaignledger.SyncToSendFromLedger(s.db, campID, campaignRecID); err != nil {
		return nil, false, err
	}

	rows, hasMore, err := campaignledger.NextPending(s.db, campaignRecID, limit)
	if err != nil {
		return nil, false, err
	}
	out := sqliteLedgerRowsToModels(rows)
	if s.verbose {
		s.log.Printf(
			"manager store sqlite: next subscribers (ledger) campaign_id=%d campaign_rec=%s fetched=%d limit=%d has_more=%v",
			campID,
			campaignRecID,
			len(out),
			limit,
			hasMore,
		)
	}
	return out, hasMore, nil
}

func sqliteLedgerRowsToModels(rows []campaignledger.SubscriberRow) []models.Subscriber {
	out := make([]models.Subscriber, 0, len(rows))
	for _, row := range rows {
		attribs := models.JSON{}
		if len(row.Attribs) > 0 && string(row.Attribs) != "null" {
			_ = json.Unmarshal(row.Attribs, &attribs)
		}
		sub := models.Subscriber{
			Base: models.Base{
				ID:        row.ID,
				RecordID:  row.RecordID,
				CreatedAt: parseStoreNullTime(row.CreatedAt),
				UpdatedAt: parseStoreNullTime(row.UpdatedAt),
			},
			UUID:      row.UUID,
			Email:     row.Email,
			Phone:     row.Phone,
			FirstName: row.FirstName,
			LastName:  row.LastName,
			Name:      row.Name,
			Attribs:   attribs,
			Status:    row.Status,
		}
		sub.NormalizeName()
		out = append(out, sub)
	}
	return out
}

func placeholders(n int) string {
	if n < 1 {
		return ""
	}

	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func normalizeCampaignFilterTags(tags []string) []string {
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

func sqliteSubscriberTagFilterClause(includeTags, excludeTags []string, subscriberAlias string) (string, []any) {
	if subscriberAlias == "" {
		subscriberAlias = "s"
	}
	args := []any{}
	var b strings.Builder

	if len(includeTags) > 0 {
		b.WriteString(`
			  AND EXISTS (
			    SELECT 1
			    FROM json_each(COALESCE(json_extract(` + subscriberAlias + `.attribs, '$.tags'), '[]')) jt
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
			    FROM json_each(COALESCE(json_extract(` + subscriberAlias + `.attribs, '$.tags'), '[]')) jt
			    WHERE lower(trim(CAST(jt.value AS TEXT))) IN (` + placeholders(len(excludeTags)) + `)
			  )`)
		for _, tag := range excludeTags {
			args = append(args, tag)
		}
	}

	return b.String(), args
}

// GetAttachment fetches a media attachment blob.
func (s *store) GetAttachment(mediaID int) (models.Attachment, error) {
	m, err := s.core.GetMedia(mediaID, "", "", s.media)
	if err != nil {
		return models.Attachment{}, err
	}

	b, err := s.media.GetBlob(m.URL)
	if err != nil {
		return models.Attachment{}, err
	}

	return models.Attachment{
		Name:    m.Filename,
		Content: b,
		Header:  manager.MakeAttachmentHeader(m.Filename, "base64", m.ContentType),
	}, nil
}

// CreateLink registers a URL for tracking clicks and returns the link's PocketBase record id (links.id).
func (s *store) CreateLink(url string) (string, error) {
	uu, err := uuid.NewV4()
	if err != nil {
		return "", err
	}

	var out string
	if err := s.db.Get(&out, `
			INSERT INTO links (uuid, url)
			VALUES (?, ?)
			ON CONFLICT(url) DO UPDATE SET url=excluded.url
			RETURNING id`,
		uu, url); err != nil {
		return "", err
	}
	return out, nil
}

func (s *store) CreateTransactionalMessage(msg models.TransactionalMessage) (models.TransactionalMessage, error) {
	dataJSON := msg.Data
	if dataJSON == nil {
		dataJSON = models.JSON{}
	}

	headersJSON := msg.Headers
	if headersJSON == nil {
		headersJSON = models.JSON{}
	}

	if strings.TrimSpace(msg.UUID) == "" {
		uu, err := uuid.NewV4()
		if err != nil {
			return models.TransactionalMessage{}, err
		}
		msg.UUID = uu.String()
	}

	subscriberID := null.String{}
	if val := strings.TrimSpace(msg.SubscriberID); val != "" {
		subscriberID = null.StringFrom(val)
	}

	templateID := null.String{}
	if val := strings.TrimSpace(msg.TemplateID); val != "" {
		templateID = null.StringFrom(val)
	}

	row := struct {
		ID              int    `db:"id"`
		RecordID        string `db:"record_id"`
		CreatedAt       string `db:"created_at"`
		UpdatedAt       string `db:"updated_at"`
		UUID            string `db:"uuid"`
		SubscriberID    string `db:"subscriber_record_id"`
		SubscriberEmail string `db:"subscriber_email"`
		TemplateID      string `db:"template_record_id"`
		FromEmail       string `db:"from_email"`
		Subject         string `db:"subject"`
		ContentType     string `db:"content_type"`
		Messenger       string `db:"messenger"`
		Status          string `db:"status"`
		Error           string `db:"error"`
		Body            string `db:"body"`
	}{}

	if err := s.db.Get(&row, `
		INSERT INTO transactional_messages (
			uuid, subscriber_id, to_email, template_id, from_email, subject,
			content_type, messenger, status, error, body, data, headers
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING rowid AS id, id AS record_id, created AS created_at, updated AS updated_at,
			uuid, subscriber_id AS subscriber_record_id, to_email AS subscriber_email,
			template_id AS template_record_id, from_email, subject, content_type, messenger, status, error, body
	`,
		msg.UUID,
		subscriberID,
		msg.SubscriberEmail,
		templateID,
		msg.FromEmail,
		msg.Subject,
		msg.ContentType,
		msg.Messenger,
		msg.Status,
		msg.Error,
		msg.Body,
		string(mustJSON(dataJSON)),
		string(mustJSON(headersJSON)),
	); err != nil {
		return models.TransactionalMessage{}, err
	}

	msg.Base = models.Base{
		ID:        row.ID,
		RecordID:  row.RecordID,
		CreatedAt: parseStoreNullTime(row.CreatedAt),
		UpdatedAt: parseStoreNullTime(row.UpdatedAt),
	}
	msg.UUID = row.UUID
	return msg, nil
}

func (s *store) UpdateTransactionalMessageStatus(recordID, status, errorMessage string, sent bool) error {
	if sent {
		_, err := s.db.Exec(`
			UPDATE transactional_messages
			SET status = ?, error = ?, sent_at = (strftime('%Y-%m-%d %H:%M:%fZ')), updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id = ?
		`, status, errorMessage, recordID)
		return err
	}

	_, err := s.db.Exec(`
		UPDATE transactional_messages
		SET status = ?, error = ?, updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE id = ?
	`, status, errorMessage, recordID)
	return err
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// RecordBounce records a bounce event and returns the bounce count.
func (s *store) RecordBounce(b models.Bounce) (int64, int, error) {
	subID := int64(0)
	if b.SubscriberUUID != "" {
		if err := s.db.Get(&subID, `SELECT id FROM subscribers WHERE uuid = ? LIMIT 1`, b.SubscriberUUID); err != nil {
			if err == sql.ErrNoRows {
				return 0, 0, nil
			}
			return 0, 0, err
		}
	} else if b.Email != "" {
		if err := s.db.Get(&subID, `SELECT id FROM subscribers WHERE email = ? LIMIT 1`, b.Email); err != nil {
			if err == sql.ErrNoRows {
				return 0, 0, nil
			}
			return 0, 0, err
		}
	}

	if subID == 0 {
		return 0, 0, nil
	}

	var campID sql.NullInt64
	if b.CampaignUUID != "" {
		_ = s.db.Get(&campID, `SELECT id FROM campaigns WHERE uuid = ? LIMIT 1`, b.CampaignUUID)
	}

	metaJSON := b.Meta
	if len(metaJSON) == 0 {
		metaJSON = json.RawMessage(`{}`)
	}

	if _, err := s.db.Exec(`
			INSERT INTO bounces (subscriber_id, campaign_id, type, source, meta, created)
			VALUES (?, ?, ?, ?, ?, (strftime('%Y-%m-%d %H:%M:%fZ')))`,
		subID, campID, b.Type, b.Source, string(metaJSON)); err != nil {
		return 0, 0, err
	}

	num := 0
	if err := s.db.Get(&num, `SELECT COUNT(*) FROM bounces WHERE subscriber_id = ? AND type = ?`, subID, b.Type); err != nil {
		return subID, 0, err
	}
	return subID, num, nil
}

// BlocklistSubscriber blocklists a subscriber permanently.
func (s *store) BlocklistSubscriber(id int64) error {
	recID, err := s.sqliteSubscriberRecordID(id)
	if err != nil {
		return err
	}

	pb := s.db.PocketBase()
	rec, err := pb.FindRecordById("subscribers", recID)
	if err != nil {
		return err
	}
	rec.Set("status", "blocklisted")
	if err := pb.Save(rec); err != nil {
		return err
	}

	_, err = s.db.Exec(`
			UPDATE subscriber_lists
			SET status='unsubscribed', updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id = ?`, recID)
	return err
}

// DeleteSubscriber deletes a subscriber from the DB.
func (s *store) DeleteSubscriber(id int64) error {
	recID, err := s.sqliteSubscriberRecordID(id)
	if err != nil {
		return err
	}
	pb := s.db.PocketBase()
	rec, err := pb.FindRecordById("subscribers", recID)
	if err != nil {
		return err
	}
	return pb.Delete(rec)
}
