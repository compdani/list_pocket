package core

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"

	"github.com/compdani/list_pocket/internal/phoneutil"
	"github.com/compdani/list_pocket/models"
	pbcore "github.com/pocketbase/pocketbase/core"
)

func (c *Core) GetInboundSMSEventsBySubscriber(ctx context.Context, subscriberID int, limit int, offset int) ([]models.InboundSMSEvent, int, error) {
	_ = ctx
	if subscriberID <= 0 {
		return []models.InboundSMSEvent{}, 0, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	subRecID, err := c.subscriberRecordIDByRowID(subscriberID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []models.InboundSMSEvent{}, 0, nil
		}
		return nil, 0, err
	}

	var total int
	if err := c.db.Get(&total, `SELECT COUNT(*) FROM inbound_sms_events WHERE subscriber_id = ?`, subRecID); err != nil {
		c.log.Printf("error counting inbound sms events: %v", err)
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "inbound sms events", "error", dbErr(err)))
	}
	if total == 0 {
		return []models.InboundSMSEvent{}, 0, nil
	}

	rows := []inboundSMSRow{}
	if err := c.db.Select(&rows, `
		SELECT
			rowid AS id,
			id AS record_id,
			created AS created_at,
			updated AS updated_at,
			subscriber_id,
			list_id,
			phone_number,
			provider_id,
			provider_msg_id,
			from_number,
			message_body,
			received_at,
			is_stop_keyword,
			match_score,
			raw_payload,
			processed_at,
			sender_hash
		FROM inbound_sms_events
		WHERE subscriber_id = ?
		ORDER BY received_at DESC, rowid DESC
		LIMIT ? OFFSET ?
	`, subRecID, limit, offset); err != nil {
		c.log.Printf("error querying inbound sms events: %v", err)
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "inbound sms events", "error", dbErr(err)))
	}

	out := make([]models.InboundSMSEvent, 0, len(rows))
	for _, r := range rows {
		e, mapErr := mapInboundSMSRow(r)
		if mapErr != nil {
			c.log.Printf("error mapping inbound sms event row: %v", mapErr)
			continue
		}
		out = append(out, e)
	}

	return out, total, nil
}

// GetInboundSMSEventByProviderID checks for existing inbound SMS event (idempotency check)
// during webhook processing to prevent duplicate ingestion.
//
// Implementation strategy (Phase 3):
// - Query inbound_sms_events where provider_id = providerID AND provider_msg_id = providerMsgID AND received_at = receivedAt
// - Fallback to sender_hash based dedup if exact match not found
// - Return event if found, nil if not found, error if query fails
//
// Used in Phase 3 for idempotency check before persisting.
func (c *Core) GetInboundSMSEventByProviderID(ctx context.Context, providerID string, providerMsgID string, receivedAt time.Time) (*models.InboundSMSEvent, error) {
	_ = ctx
	providerID = strings.TrimSpace(providerID)
	providerMsgID = strings.TrimSpace(providerMsgID)
	if providerID == "" || providerMsgID == "" {
		return nil, nil
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	var row inboundSMSRow
	err := c.db.Get(&row, `
		SELECT
			rowid AS id,
			id AS record_id,
			created AS created_at,
			updated AS updated_at,
			subscriber_id,
			list_id,
			phone_number,
			provider_id,
			provider_msg_id,
			from_number,
			message_body,
			received_at,
			is_stop_keyword,
			match_score,
			raw_payload,
			processed_at,
			sender_hash
		FROM inbound_sms_events
		WHERE provider_id = ?
		  AND provider_msg_id = ?
		  AND strftime('%s', received_at) = strftime('%s', ?)
		ORDER BY rowid DESC
		LIMIT 1
	`, providerID, providerMsgID, receivedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	e, err := mapInboundSMSRow(row)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CreateInboundSMSEvent persists a new inbound SMS event to the database
// Called during Phase 3 Quo webhook processing.
//
// Implementation strategy (Phase 3):
// - Perform idempotency check via GetInboundSMSEventByProviderID
// - If exists, return early (already processed)
// - If not exists, persist new InboundSMSEvent record
// - Trigger downstream processing (e.g., STOP keyword handling, timeline updates)
//
// Returns the created event ID and any errors.
func (c *Core) CreateInboundSMSEvent(ctx context.Context, event *models.InboundSMSEvent) (string, error) {
	_ = ctx
	if event == nil {
		return "", errors.New("inbound sms event cannot be nil")
	}

	now := time.Now().UTC()
	if event.ReceivedAt.IsZero() {
		event.ReceivedAt = now
	}
	if event.ProcessedAt.IsZero() {
		event.ProcessedAt = now
	}
	event.ProviderID = strings.TrimSpace(event.ProviderID)
	if event.ProviderID == "" {
		event.ProviderID = "quo"
	}
	event.ProviderMsgID = strings.TrimSpace(event.ProviderMsgID)
	event.FromNumber = strings.TrimSpace(event.FromNumber)
	event.PhoneNumber = phoneutil.NormalizeDigits(event.PhoneNumber)
	if event.PhoneNumber == "" {
		event.PhoneNumber = phoneutil.NormalizeDigits(event.FromNumber)
	}
	event.SenderHash = strings.TrimSpace(event.SenderHash)
	if event.SenderHash == "" {
		event.SenderHash = hashInboundSMSSender(event.FromNumber, event.PhoneNumber)
	}

	if existing, err := c.GetInboundSMSEventByProviderID(ctx, event.ProviderID, event.ProviderMsgID, event.ReceivedAt); err == nil && existing != nil {
		return existing.RecordID, nil
	} else if err != nil {
		return "", err
	}

	if existingID, err := c.getInboundSMSEventRecordIDBySenderHash(event.ProviderID, event.SenderHash, event.ReceivedAt); err != nil {
		return "", err
	} else if existingID != "" {
		return existingID, nil
	}

	if event.MatchScore == "" {
		event.MatchScore = "unmatched"
	}

	if event.SubscriberID == nil {
		if subRowID, score, err := c.MatchSubscriberByInboundSMS(ctx, event.FromNumber, nil); err == nil && subRowID > 0 {
			if recID, convErr := c.subscriberRecordIDByRowID(subRowID); convErr == nil {
				event.SubscriberID = &recID
				event.MatchScore = score
				if event.ListID == nil {
					if listRecID, inferErr := c.inferSingleListRecordIDForSubscriberRow(subRowID); inferErr == nil && listRecID != "" {
						event.ListID = &listRecID
					}
				}
			}
		} else if err != nil {
			c.log.Printf("inbound sms match warning: %v", err)
		}
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return "", fmt.Errorf("pocketbase is not initialized")
	}
	collection, err := pb.FindCollectionByNameOrId("inbound_sms_events")
	if err != nil {
		return "", err
	}
	rec := pbcore.NewRecord(collection)
	if event.SubscriberID != nil {
		rec.Set("subscriber_id", *event.SubscriberID)
	}
	if event.ListID != nil {
		rec.Set("list_id", *event.ListID)
	}
	rec.Set("phone_number", event.PhoneNumber)
	rec.Set("provider_id", event.ProviderID)
	rec.Set("provider_msg_id", event.ProviderMsgID)
	rec.Set("from_number", event.FromNumber)
	rec.Set("message_body", event.MessageBody)
	rec.Set("received_at", event.ReceivedAt.UTC().Format(time.RFC3339Nano))
	rec.Set("is_stop_keyword", event.IsStopKeyword)
	rec.Set("match_score", event.MatchScore)
	if len(event.RawPayload) > 0 {
		rec.Set("raw_payload", map[string]any(event.RawPayload))
	}
	rec.Set("processed_at", event.ProcessedAt.UTC().Format(time.RFC3339Nano))
	rec.Set("sender_hash", event.SenderHash)

	if err := pb.Save(rec); err != nil {
		if isLikelyDuplicateError(err) {
			if existingID, lookErr := c.getInboundSMSEventRecordIDBySenderHash(event.ProviderID, event.SenderHash, event.ReceivedAt); lookErr == nil && existingID != "" {
				return existingID, nil
			}
			if existing, lookErr := c.GetInboundSMSEventByProviderID(ctx, event.ProviderID, event.ProviderMsgID, event.ReceivedAt); lookErr == nil && existing != nil {
				return existing.RecordID, nil
			}
		}
		return "", err
	}

	return rec.Id, nil
}

// GetInboundEmailRepliesBySubscriber returns all inbound email replies for a subscriber, ordered by received_at DESC
// Used by Phase 4 webhook handler and timeline assembly in Phase 5.
//
// Implementation strategy (Phase 4):
// - Query inbound_email_replies where subscriber_id = params.SubscriberID
// - Order by received_at DESC, rowid DESC
// - Apply offset/limit pagination
// - Return events as []InboundEmailReplyEvent
//
// Note: This is defined as an interface; implementation in Phase 4.
func (c *Core) MatchSubscriberByInboundSMS(ctx context.Context, phoneNumber string, listID *int) (int, string, error) {
	_ = ctx
	digits := phoneutil.NormalizeDigits(phoneNumber)
	if digits == "" {
		return 0, "unmatched", nil
	}

	const normalizePhoneSQL = `REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(COALESCE(phone, ''), '+', ''), ' ', ''), '-', ''), '(', ''), ')', ''), '.', '')`
	listClause := ""
	listArgs := []any{}
	if listID != nil && *listID > 0 {
		listClause = `
		AND EXISTS (
			SELECT 1
			FROM subscriber_lists sl
			JOIN lists l ON l.id = sl.list_id
			WHERE sl.subscriber_id = s.id AND l.rowid = ?
		)
		`
		listArgs = append(listArgs, *listID)
	}

	exactArgs := []any{digits}
	exactArgs = append(exactArgs, listArgs...)
	exactRows := []struct {
		SubscriberID int `db:"subscriber_id"`
	}{}
	if err := c.db.Select(&exactRows, `
		SELECT s.rowid AS subscriber_id
		FROM subscribers s
		WHERE `+normalizePhoneSQL+` = ?
		`+listClause+`
		ORDER BY s.rowid ASC
		LIMIT 2
	`, exactArgs...); err != nil {
		return 0, "unmatched", err
	}
	if len(exactRows) > 0 {
		if len(exactRows) > 1 {
			c.log.Printf("inbound sms match: multiple exact phone matches for %q; using rowid=%d", digits, exactRows[0].SubscriberID)
		}
		return exactRows[0].SubscriberID, "exact", nil
	}

	if len(digits) < 10 {
		return 0, "unmatched", nil
	}

	fallbackArgs := []any{digits, digits}
	fallbackArgs = append(fallbackArgs, listArgs...)
	fallbackRows := []struct {
		SubscriberID int `db:"subscriber_id"`
	}{}
	if err := c.db.Select(&fallbackRows, `
		SELECT s.rowid AS subscriber_id
		FROM subscribers s
		WHERE LENGTH(`+normalizePhoneSQL+`) >= 10
		  AND LENGTH(?) >= 10
		  AND SUBSTR(`+normalizePhoneSQL+`, -10) = SUBSTR(?, -10)
		`+listClause+`
		ORDER BY s.rowid ASC
		LIMIT 2
	`, fallbackArgs...); err != nil {
		return 0, "unmatched", err
	}
	if len(fallbackRows) > 0 {
		if len(fallbackRows) > 1 {
			c.log.Printf("inbound sms match: multiple fallback phone matches for %q; using rowid=%d", digits, fallbackRows[0].SubscriberID)
		}
		return fallbackRows[0].SubscriberID, "fallback_10digit", nil
	}

	return 0, "unmatched", nil
}

// MatchSubscriberByInboundEmail attempts to link an inbound email to a subscriber
// using exact email address matching.
//
// Returns:
// - subscriber ID (if match found)
// - match_score ("exact_email", "unmatched")
// - error if query fails
//
// Implementation strategy (Phase 4):
// - Query subscribers where email = from_address (case-insensitive, trimmed)
// - Return first match found
// - Return "unmatched" if no matches
//
// Note: Strict matching only (no fuzzy/heuristic matching in this phase)
type inboundSMSRow struct {
	ID             int            `db:"id"`
	RecordID       string         `db:"record_id"`
	CreatedAtRaw   sql.NullString `db:"created_at"`
	UpdatedAtRaw   sql.NullString `db:"updated_at"`
	SubscriberID   sql.NullString `db:"subscriber_id"`
	ListID         sql.NullString `db:"list_id"`
	PhoneNumber    string         `db:"phone_number"`
	ProviderID     string         `db:"provider_id"`
	ProviderMsgID  string         `db:"provider_msg_id"`
	FromNumber     string         `db:"from_number"`
	MessageBody    string         `db:"message_body"`
	ReceivedAtRaw  sql.NullString `db:"received_at"`
	IsStopKeyword  bool           `db:"is_stop_keyword"`
	MatchScore     string         `db:"match_score"`
	RawPayloadRaw  any            `db:"raw_payload"`
	ProcessedAtRaw sql.NullString `db:"processed_at"`
	SenderHash     string         `db:"sender_hash"`
}

func mapInboundSMSRow(r inboundSMSRow) (models.InboundSMSEvent, error) {
	createdAt, err := parseSQLiteDateTime(r.CreatedAtRaw.String)
	if err != nil && strings.TrimSpace(r.CreatedAtRaw.String) != "" {
		return models.InboundSMSEvent{}, err
	}
	updatedAt, err := parseSQLiteDateTime(r.UpdatedAtRaw.String)
	if err != nil && strings.TrimSpace(r.UpdatedAtRaw.String) != "" {
		return models.InboundSMSEvent{}, err
	}
	receivedAt, err := parseSQLiteDateTime(r.ReceivedAtRaw.String)
	if err != nil {
		return models.InboundSMSEvent{}, err
	}
	processedAt, err := parseSQLiteDateTime(r.ProcessedAtRaw.String)
	if err != nil {
		return models.InboundSMSEvent{}, err
	}
	payload, err := decodeJSONFieldToModelJSON(r.RawPayloadRaw)
	if err != nil {
		return models.InboundSMSEvent{}, err
	}

	out := models.InboundSMSEvent{
		Base: models.Base{
			ID:       r.ID,
			RecordID: r.RecordID,
		},
		PhoneNumber:   r.PhoneNumber,
		ProviderID:    r.ProviderID,
		ProviderMsgID: r.ProviderMsgID,
		FromNumber:    r.FromNumber,
		MessageBody:   r.MessageBody,
		ReceivedAt:    receivedAt,
		IsStopKeyword: r.IsStopKeyword,
		MatchScore:    r.MatchScore,
		RawPayload:    payload,
		ProcessedAt:   processedAt,
		SenderHash:    r.SenderHash,
	}
	if r.CreatedAtRaw.Valid {
		out.CreatedAt.Time = createdAt
		out.CreatedAt.Valid = true
	}
	if r.UpdatedAtRaw.Valid {
		out.UpdatedAt.Time = updatedAt
		out.UpdatedAt.Valid = true
	}
	if r.SubscriberID.Valid && strings.TrimSpace(r.SubscriberID.String) != "" {
		v := r.SubscriberID.String
		out.SubscriberID = &v
	}
	if r.ListID.Valid && strings.TrimSpace(r.ListID.String) != "" {
		v := r.ListID.String
		out.ListID = &v
	}

	return out, nil
}

func hashInboundSMSSender(fromNumber, normalized string) string {
	basis := strings.TrimSpace(normalized)
	if basis == "" {
		basis = phoneutil.NormalizeDigits(fromNumber)
	}
	if basis == "" {
		basis = strings.ToLower(strings.TrimSpace(fromNumber))
	}
	sum := sha256.Sum256([]byte(basis))
	return hex.EncodeToString(sum[:])
}

func isLikelyDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	e := strings.ToLower(err.Error())
	return strings.Contains(e, "unique") || strings.Contains(e, "duplicate")
}

func (c *Core) getInboundSMSEventRecordIDBySenderHash(providerID, senderHash string, receivedAt time.Time) (string, error) {
	providerID = strings.TrimSpace(providerID)
	senderHash = strings.TrimSpace(senderHash)
	if providerID == "" || senderHash == "" {
		return "", nil
	}
	var recID string
	err := c.db.Get(&recID, `
		SELECT id
		FROM inbound_sms_events
		WHERE provider_id = ?
		  AND sender_hash = ?
		  AND strftime('%s', received_at) = strftime('%s', ?)
		ORDER BY rowid DESC
		LIMIT 1
	`, providerID, senderHash, receivedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return recID, nil
}
