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

	"github.com/compdani/list_pocket/models"
	pbcore "github.com/pocketbase/pocketbase/core"
)

func (c *Core) GetInboundEmailRepliesBySubscriber(ctx context.Context, subscriberID int, limit int, offset int) ([]models.InboundEmailReplyEvent, int, error) {
	_ = ctx
	if subscriberID <= 0 {
		return []models.InboundEmailReplyEvent{}, 0, nil
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
			return []models.InboundEmailReplyEvent{}, 0, nil
		}
		return nil, 0, err
	}

	var total int
	if err := c.db.Get(&total, `SELECT COUNT(*) FROM inbound_email_replies WHERE subscriber_id = ?`, subRecID); err != nil {
		c.log.Printf("error counting inbound email replies: %v", err)
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "inbound email replies", "error", dbErr(err)))
	}
	if total == 0 {
		return []models.InboundEmailReplyEvent{}, 0, nil
	}

	rows := []inboundEmailReplyRow{}
	if err := c.db.Select(&rows, `
		SELECT
			rowid AS id,
			id AS record_id,
			created AS created_at,
			updated AS updated_at,
			subscriber_id,
			linked_message_id,
			from_address,
			subject,
			message_id,
			in_reply_to,
			"references" AS "references",
			received_at,
			body_snippet,
			COALESCE(body_html, '') AS body_html,
			COALESCE(body_text, '') AS body_text,
			COALESCE(to_address, '') AS to_address,
			COALESCE(cc, '') AS cc,
			COALESCE(reply_to, '') AS reply_to,
			structured_headers,
			has_attachments,
			match_score,
			COALESCE(spam_status, '') AS spam_status,
			COALESCE(spam_score, 0) AS spam_score,
			processed_at,
			dedupe_key
		FROM inbound_email_replies
		WHERE subscriber_id = ?
		ORDER BY received_at DESC, rowid DESC
		LIMIT ? OFFSET ?
	`, subRecID, limit, offset); err != nil {
		c.log.Printf("error querying inbound email replies: %v", err)
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "inbound email replies", "error", dbErr(err)))
	}

	out := make([]models.InboundEmailReplyEvent, 0, len(rows))
	for _, r := range rows {
		e, mapErr := mapInboundEmailReplyRow(r)
		if mapErr != nil {
			c.log.Printf("error mapping inbound email reply row: %v", mapErr)
			continue
		}
		out = append(out, e)
	}

	return out, total, nil
}

// GetInboundEmailReplyByMessageID checks for existing inbound email reply (idempotency check)
// during webhook processing to prevent duplicate ingestion.
//
// Implementation strategy (Phase 4):
// - Query inbound_email_replies where message_id = messageID AND from_address = fromAddress AND received_at = receivedAt
// - Use dedupe_key if exact match not found
// - Return event if found, nil if not found, error if query fails
//
// Used in Phase 4 for idempotency check before persisting.
func (c *Core) GetInboundEmailReplyByMessageID(ctx context.Context, messageID string, fromAddress string, receivedAt time.Time) (*models.InboundEmailReplyEvent, error) {
	_ = ctx
	messageID = normalizeMessageID(messageID)
	fromAddress = strings.ToLower(strings.TrimSpace(fromAddress))
	if messageID == "" || fromAddress == "" {
		return nil, nil
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	var row inboundEmailReplyRow
	err := c.db.Get(&row, `
		SELECT
			rowid AS id,
			id AS record_id,
			created AS created_at,
			updated AS updated_at,
			subscriber_id,
			linked_message_id,
			from_address,
			subject,
			message_id,
			in_reply_to,
			"references" AS "references",
			received_at,
			body_snippet,
			COALESCE(body_html, '') AS body_html,
			COALESCE(body_text, '') AS body_text,
			COALESCE(to_address, '') AS to_address,
			COALESCE(cc, '') AS cc,
			COALESCE(reply_to, '') AS reply_to,
			structured_headers,
			has_attachments,
			match_score,
			COALESCE(spam_status, '') AS spam_status,
			COALESCE(spam_score, 0) AS spam_score,
			processed_at,
			dedupe_key
		FROM inbound_email_replies
		WHERE message_id = ?
		  AND LOWER(from_address) = ?
		  AND strftime('%s', received_at) = strftime('%s', ?)
		ORDER BY rowid DESC
		LIMIT 1
	`, messageID, fromAddress, receivedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	e, err := mapInboundEmailReplyRow(row)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CreateInboundEmailReplyEvent persists a new inbound email reply to the database
// Called during Phase 4 webhook processing.
//
// Implementation strategy (Phase 4):
// - Perform idempotency check via GetInboundEmailReplyByMessageID
// - If exists, return early (already processed)
// - If not exists, attempt to link to outbound message via In-Reply-To/References headers
// - Persist new InboundEmailReplyEvent record
// - Trigger downstream processing (e.g., timeline updates, threading)
//
// Returns the created event ID and any errors.
func (c *Core) CreateInboundEmailReplyEvent(ctx context.Context, event *models.InboundEmailReplyEvent) (string, error) {
	_ = ctx
	if event == nil {
		return "", errors.New("inbound email reply event cannot be nil")
	}

	now := time.Now().UTC()
	if event.ReceivedAt.IsZero() {
		event.ReceivedAt = now
	}
	if event.ProcessedAt.IsZero() {
		event.ProcessedAt = now
	}
	event.FromAddress = strings.ToLower(strings.TrimSpace(event.FromAddress))
	event.Subject = strings.TrimSpace(event.Subject)
	event.MessageID = normalizeMessageID(event.MessageID)
	event.InReplyTo = normalizeMessageID(event.InReplyTo)
	event.References = strings.TrimSpace(event.References)
	if event.MatchScore == "" {
		event.MatchScore = "unmatched"
	}
	event.DedupeKey = strings.TrimSpace(event.DedupeKey)
	if event.DedupeKey == "" {
		event.DedupeKey = hashInboundEmailDedupe(event.MessageID, event.FromAddress, event.ReceivedAt)
	}

	if existing, err := c.GetInboundEmailReplyByMessageID(ctx, event.MessageID, event.FromAddress, event.ReceivedAt); err == nil && existing != nil {
		return existing.RecordID, nil
	} else if err != nil {
		return "", err
	}

	if existingID, err := c.getInboundEmailReplyRecordIDByDedupe(event.DedupeKey); err != nil {
		return "", err
	} else if existingID != "" {
		return existingID, nil
	}

	if event.SubscriberID == nil {
		if subRowID, score, err := c.MatchSubscriberByInboundEmail(ctx, event.FromAddress); err == nil && subRowID > 0 {
			if subRecID, convErr := c.subscriberRecordIDByRowID(subRowID); convErr == nil {
				event.SubscriberID = &subRecID
				event.MatchScore = score
			}
		} else if err != nil {
			c.log.Printf("inbound email match warning: %v", err)
		}
	}

	if event.LinkedMessageID == nil {
		if linked, err := c.LinkInboundEmailToOutboundMessage(ctx, event.InReplyTo, event.References); err == nil && linked != nil {
			event.LinkedMessageID = linked
		} else if err != nil {
			c.log.Printf("inbound email linkage warning: %v", err)
		}
	}

	// Check spam rules to auto-classify the incoming email.
	if event.SpamStatus == "" {
		if spamStatus, spamScore, checkErr := c.CheckInboundSpamRules(ctx, event.FromAddress, event.Subject, event.BodyText); checkErr == nil && spamStatus != "" {
			event.SpamStatus = spamStatus
			event.SpamScore = spamScore
		}
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return "", fmt.Errorf("pocketbase is not initialized")
	}
	collection, err := pb.FindCollectionByNameOrId("inbound_email_replies")
	if err != nil {
		return "", err
	}
	rec := pbcore.NewRecord(collection)
	if event.SubscriberID != nil {
		rec.Set("subscriber_id", *event.SubscriberID)
	}
	if event.LinkedMessageID != nil {
		rec.Set("linked_message_id", *event.LinkedMessageID)
	}
	rec.Set("from_address", event.FromAddress)
	rec.Set("subject", event.Subject)
	rec.Set("message_id", event.MessageID)
	rec.Set("in_reply_to", event.InReplyTo)
	rec.Set("references", event.References)
	rec.Set("received_at", event.ReceivedAt.UTC().Format(time.RFC3339Nano))
	rec.Set("body_snippet", event.BodySnippet)
	rec.Set("body_html", event.BodyHTML)
	rec.Set("body_text", event.BodyText)
	rec.Set("to_address", event.ToAddress)
	rec.Set("cc", event.CC)
	rec.Set("reply_to", event.ReplyTo)
	if len(event.StructuredHeaders) > 0 {
		rec.Set("structured_headers", map[string]any(event.StructuredHeaders))
	}
	rec.Set("has_attachments", event.HasAttachments)
	rec.Set("match_score", event.MatchScore)
	if event.SpamStatus != "" {
		rec.Set("spam_status", event.SpamStatus)
	}
	if event.SpamScore > 0 {
		rec.Set("spam_score", event.SpamScore)
	}
	rec.Set("processed_at", event.ProcessedAt.UTC().Format(time.RFC3339Nano))
	rec.Set("dedupe_key", event.DedupeKey)

	if err := pb.Save(rec); err != nil {
		if isLikelyDuplicateError(err) {
			if existingID, lookErr := c.getInboundEmailReplyRecordIDByDedupe(event.DedupeKey); lookErr == nil && existingID != "" {
				return existingID, nil
			}
			if existing, lookErr := c.GetInboundEmailReplyByMessageID(ctx, event.MessageID, event.FromAddress, event.ReceivedAt); lookErr == nil && existing != nil {
				return existing.RecordID, nil
			}
		}
		return "", err
	}

	return rec.Id, nil
}

// MatchSubscriberByInboundSMS attempts to link an inbound SMS to a subscriber
// using phone number matching (exact + last-10 fallback, reusing SMS opt-out logic).
//
// Returns:
// - subscriber ID (if match found)
// - match_score ("exact", "fallback_10digit", "unmatched")
// - error if query fails
//
// Implementation strategy (Phase 3):
// - Normalize phone number (extract digits only)
// - Query subscriber_lists where phone matches normalized inbound number
// - If no exact match, try last-10-digit fallback
// - Return first match found (if multi-match, log but return first)
// - Return "unmatched" if no matches
//
// Reuses existing phone normalization from SMS opt-out handler.
func (c *Core) MatchSubscriberByInboundEmail(ctx context.Context, emailAddress string) (int, string, error) {
	_ = ctx
	emailAddress = strings.ToLower(strings.TrimSpace(emailAddress))
	if emailAddress == "" {
		return 0, "unmatched", nil
	}
	rows := []struct {
		SubscriberID int `db:"subscriber_id"`
	}{}
	if err := c.db.Select(&rows, `
		SELECT rowid AS subscriber_id
		FROM subscribers
		WHERE LOWER(email) = ?
		ORDER BY rowid ASC
		LIMIT 2
	`, emailAddress); err != nil {
		return 0, "unmatched", err
	}
	if len(rows) == 0 {
		return 0, "unmatched", nil
	}
	if len(rows) > 1 {
		c.log.Printf("inbound email match: multiple matches for %q; using rowid=%d", emailAddress, rows[0].SubscriberID)
	}
	return rows[0].SubscriberID, "exact_email", nil
}

// LinkInboundEmailToOutboundMessage attempts to link an inbound email reply to an outbound campaign message
// using RFC 5322 threading headers (In-Reply-To, References, Message-ID).
//
// Returns:
// - campaign_send_ledger record ID (if match found)
// - error if query fails
//
// Implementation strategy (Phase 4):
// - Extract message IDs from In-Reply-To header (primary linkage)
// - Fallback to References header if In-Reply-To not found
// - Query campaign_send_ledger where message_id matches extracted ID
// - Return first match found
// - Return nil if no matches (thread may be orphaned or from non-tracked campaign)
//
// Used to populate linked_message_id for threading and context.
func (c *Core) LinkInboundEmailToOutboundMessage(ctx context.Context, inReplyTo string, references string) (*string, error) {
	_ = ctx
	candidateIDs := extractRecordIDCandidatesFromThreadHeaders(inReplyTo, references)
	if len(candidateIDs) == 0 {
		return nil, nil
	}
	for _, candidate := range candidateIDs {
		candidate = normalizeMessageID(candidate)
		if candidate == "" {
			continue
		}
		var recID string
		err := c.db.Get(&recID, `SELECT id FROM campaign_send_ledger WHERE message_id = ? LIMIT 1`, candidate)
		if err == nil {
			v := recID
			return &v, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		err = c.db.Get(&recID, `SELECT id FROM campaign_send_ledger WHERE id = ? LIMIT 1`, candidate)
		if err == nil {
			v := recID
			return &v, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return nil, nil
}

type inboundEmailReplyRow struct {
	ID                   int            `db:"id"`
	RecordID             string         `db:"record_id"`
	CreatedAtRaw         sql.NullString `db:"created_at"`
	UpdatedAtRaw         sql.NullString `db:"updated_at"`
	SubscriberID         sql.NullString `db:"subscriber_id"`
	LinkedMessageID      sql.NullString `db:"linked_message_id"`
	FromAddress          string         `db:"from_address"`
	Subject              string         `db:"subject"`
	MessageID            string         `db:"message_id"`
	InReplyTo            string         `db:"in_reply_to"`
	References           string         `db:"references"`
	ReceivedAtRaw        sql.NullString `db:"received_at"`
	BodySnippet          string         `db:"body_snippet"`
	BodyHTML             string         `db:"body_html"`
	BodyText             string         `db:"body_text"`
	ToAddress            string         `db:"to_address"`
	CC                   string         `db:"cc"`
	ReplyTo              string         `db:"reply_to"`
	StructuredHeadersRaw any            `db:"structured_headers"`
	HasAttachments       bool           `db:"has_attachments"`
	MatchScore           string         `db:"match_score"`
	SpamStatus           string         `db:"spam_status"`
	SpamScore            float64        `db:"spam_score"`
	ProcessedAtRaw       sql.NullString `db:"processed_at"`
	DedupeKey            string         `db:"dedupe_key"`
}

func mapInboundEmailReplyRow(r inboundEmailReplyRow) (models.InboundEmailReplyEvent, error) {
	createdAt, err := parseSQLiteDateTime(r.CreatedAtRaw.String)
	if err != nil && strings.TrimSpace(r.CreatedAtRaw.String) != "" {
		return models.InboundEmailReplyEvent{}, err
	}
	updatedAt, err := parseSQLiteDateTime(r.UpdatedAtRaw.String)
	if err != nil && strings.TrimSpace(r.UpdatedAtRaw.String) != "" {
		return models.InboundEmailReplyEvent{}, err
	}
	receivedAt, err := parseSQLiteDateTime(r.ReceivedAtRaw.String)
	if err != nil {
		return models.InboundEmailReplyEvent{}, err
	}
	processedAt, err := parseSQLiteDateTime(r.ProcessedAtRaw.String)
	if err != nil {
		return models.InboundEmailReplyEvent{}, err
	}
	structuredHeaders, err := decodeJSONFieldToModelJSON(r.StructuredHeadersRaw)
	if err != nil {
		return models.InboundEmailReplyEvent{}, err
	}

	out := models.InboundEmailReplyEvent{
		Base: models.Base{
			ID:       r.ID,
			RecordID: r.RecordID,
		},
		FromAddress:       r.FromAddress,
		Subject:           r.Subject,
		MessageID:         r.MessageID,
		InReplyTo:         r.InReplyTo,
		References:        r.References,
		ReceivedAt:        receivedAt,
		BodySnippet:       r.BodySnippet,
		BodyHTML:          r.BodyHTML,
		BodyText:          r.BodyText,
		ToAddress:         r.ToAddress,
		CC:                r.CC,
		ReplyTo:           r.ReplyTo,
		StructuredHeaders: structuredHeaders,
		HasAttachments:    r.HasAttachments,
		MatchScore:        r.MatchScore,
		SpamStatus:        r.SpamStatus,
		SpamScore:         r.SpamScore,
		ProcessedAt:       processedAt,
		DedupeKey:         r.DedupeKey,
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
	if r.LinkedMessageID.Valid && strings.TrimSpace(r.LinkedMessageID.String) != "" {
		v := r.LinkedMessageID.String
		out.LinkedMessageID = &v
	}

	return out, nil
}

func normalizeMessageID(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "<>")
	return strings.TrimSpace(raw)
}

func hashInboundEmailDedupe(messageID, fromAddress string, receivedAt time.Time) string {
	basis := normalizeMessageID(messageID) + "|" + strings.ToLower(strings.TrimSpace(fromAddress)) + "|" + receivedAt.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(basis))
	return hex.EncodeToString(sum[:])
}

func (c *Core) getInboundEmailReplyRecordIDByDedupe(dedupeKey string) (string, error) {
	dedupeKey = strings.TrimSpace(dedupeKey)
	if dedupeKey == "" {
		return "", nil
	}
	var recID string
	err := c.db.Get(&recID, `
		SELECT id
		FROM inbound_email_replies
		WHERE dedupe_key = ?
		ORDER BY rowid DESC
		LIMIT 1
	`, dedupeKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return recID, nil
}

func extractRecordIDCandidatesFromThreadHeaders(inReplyTo string, references string) []string {
	all := strings.TrimSpace(inReplyTo + " " + references)
	if all == "" {
		return nil
	}
	all = strings.NewReplacer("<", " ", ">", " ", ",", " ", ";", " ", "\n", " ", "\r", " ", "\t", " ").Replace(all)
	parts := strings.Fields(all)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || len(p) < 10 {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// spamLevelOrder returns a numeric rank for a spam level to allow max-of comparison.
