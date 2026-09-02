package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"

	"github.com/compdani/list_pocket/models"
)

type InboxQueryParams struct {
	Limit      int
	Offset     int
	Search     string // filter by from_address or subject (partial match)
	SpamStatus string // filter by spam_status value (empty = non-spam only, "all" = all)
	StartDate  *time.Time
	EndDate    *time.Time
	SortOrder  string // "desc" (default) or "asc"
}

// GetInboundEmailInbox returns a paginated list of all inbound emails across all subscribers.
func (c *Core) GetInboundEmailInbox(ctx context.Context, params InboxQueryParams) ([]models.InboundEmailSummary, int, error) {
	_ = ctx
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	sortDir := "DESC"
	if strings.ToLower(params.SortOrder) == "asc" {
		sortDir = "ASC"
	}

	conds := []string{}
	args := []any{}
	search := strings.TrimSpace(params.Search)
	if search != "" {
		conds = append(conds, `(LOWER(e.from_address) LIKE ? OR LOWER(e.subject) LIKE ?)`)
		like := "%" + strings.ToLower(search) + "%"
		args = append(args, like, like)
	}
	switch params.SpamStatus {
	case "all":
		// No filter.
	case "spam", "confirmed_spam", "suspected":
		conds = append(conds, `e.spam_status = ?`)
		args = append(args, params.SpamStatus)
	default:
		// Default: exclude spam (show clean inbox only).
		conds = append(conds, `(e.spam_status IS NULL OR e.spam_status = '')`)
	}
	if params.StartDate != nil {
		conds = append(conds, `e.received_at >= ?`)
		args = append(args, params.StartDate.UTC().Format(time.RFC3339Nano))
	}
	if params.EndDate != nil {
		conds = append(conds, `e.received_at < ?`)
		args = append(args, params.EndDate.UTC().Format(time.RFC3339Nano))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	countQuery := `SELECT COUNT(*) FROM inbound_email_replies e ` + where
	if err := c.db.Get(&total, countQuery, args...); err != nil {
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "inbox", "error", dbErr(err)))
	}
	if total == 0 {
		return []models.InboundEmailSummary{}, 0, nil
	}

	orderClause := fmt.Sprintf("ORDER BY e.received_at %s, e.rowid %s", sortDir, sortDir)
	listQuery := fmt.Sprintf(`
		SELECT
			e.id AS record_id,
			e.subscriber_id,
			e.from_address,
			e.subject,
			e.body_snippet,
			e.message_id,
			e.received_at,
			e.has_attachments,
			e.match_score,
			COALESCE(e.spam_status, '') AS spam_status,
			COALESCE(e.spam_score, 0) AS spam_score,
			s.name AS subscriber_name,
			s.email AS subscriber_email
		FROM inbound_email_replies e
		LEFT JOIN subscribers s ON s.id = e.subscriber_id
		%s
		%s
		LIMIT ? OFFSET ?
	`, where, orderClause)
	listArgs := append(args, limit, offset)

	type summaryRow struct {
		RecordID        string         `db:"record_id"`
		SubscriberID    sql.NullString `db:"subscriber_id"`
		FromAddress     string         `db:"from_address"`
		Subject         string         `db:"subject"`
		BodySnippet     string         `db:"body_snippet"`
		MessageID       string         `db:"message_id"`
		ReceivedAtRaw   sql.NullString `db:"received_at"`
		HasAttachments  bool           `db:"has_attachments"`
		MatchScore      string         `db:"match_score"`
		SpamStatus      string         `db:"spam_status"`
		SpamScore       float64        `db:"spam_score"`
		SubscriberName  sql.NullString `db:"subscriber_name"`
		SubscriberEmail sql.NullString `db:"subscriber_email"`
	}
	rows := []summaryRow{}
	if err := c.db.Select(&rows, listQuery, listArgs...); err != nil {
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "inbox", "error", dbErr(err)))
	}

	out := make([]models.InboundEmailSummary, 0, len(rows))
	for _, r := range rows {
		receivedAt, _ := parseSQLiteDateTime(r.ReceivedAtRaw.String)
		s := models.InboundEmailSummary{
			RecordID:       r.RecordID,
			FromAddress:    r.FromAddress,
			Subject:        r.Subject,
			BodySnippet:    r.BodySnippet,
			MessageID:      r.MessageID,
			ReceivedAt:     receivedAt,
			HasAttachments: r.HasAttachments,
			MatchScore:     r.MatchScore,
			SpamStatus:     r.SpamStatus,
			SpamScore:      r.SpamScore,
		}
		if r.SubscriberID.Valid && r.SubscriberID.String != "" {
			v := r.SubscriberID.String
			s.SubscriberID = &v
		}
		if r.SubscriberName.Valid && r.SubscriberName.String != "" {
			v := r.SubscriberName.String
			s.SubscriberName = &v
		}
		if r.SubscriberEmail.Valid && r.SubscriberEmail.String != "" {
			v := r.SubscriberEmail.String
			s.SubscriberEmail = &v
		}
		out = append(out, s)
	}
	return out, total, nil
}

// GetInboundEmailByID retrieves a single inbound email reply by its PocketBase record ID.
func (c *Core) GetInboundEmailByID(ctx context.Context, id string) (*models.InboundEmailReplyEvent, error) {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperr.BadRequest("id is required")
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
		WHERE id = ?
		LIMIT 1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("inbound email not found")
		}
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "inbound email", "error", dbErr(err)))
	}
	e, mapErr := mapInboundEmailReplyRow(row)
	if mapErr != nil {
		return nil, mapErr
	}
	return &e, nil
}

// UpdateInboundEmailSpamStatus updates the spam_status on an inbound email reply and triggers
// spam learning (upserts sender/domain/keyword rules) so future emails are auto-classified.
func (c *Core) UpdateInboundEmailSpamStatus(ctx context.Context, id string, spamStatus string) error {
	_ = ctx
	id = strings.TrimSpace(id)
	validStatuses := map[string]bool{"": true, "suspected": true, "spam": true, "confirmed_spam": true}
	if !validStatuses[spamStatus] {
		return apperr.BadRequest("invalid spam_status value")
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return fmt.Errorf("pocketbase is not initialized")
	}
	rec, err := pb.FindRecordById("inbound_email_replies", id)
	if err != nil {
		return apperr.NotFound("inbound email not found")
	}

	rec.Set("spam_status", spamStatus)
	if err := pb.Save(rec); err != nil {
		return apperr.Internal("failed to update spam status")
	}

	// Trigger learning when explicitly marking as spam level.
	if spamStatus == "spam" || spamStatus == "confirmed_spam" {
		fromAddress := strings.ToLower(strings.TrimSpace(toString(rec.Get("from_address"))))
		subject := strings.TrimSpace(toString(rec.Get("subject")))
		bodyText := strings.TrimSpace(toString(rec.Get("body_text")))
		if learnErr := c.LearnSpamFromEmail(ctx, fromAddress, subject, bodyText, spamStatus); learnErr != nil {
			c.log.Printf("spam learning warning id=%q: %v", id, learnErr)
		}
	}
	return nil
}

// LearnSpamFromEmail upserts sender, domain, and keyword spam rules based on
// an explicitly user-marked spam email.
