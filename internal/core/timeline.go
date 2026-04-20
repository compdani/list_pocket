package core

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/phoneutil"
	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
	pbcore "github.com/pocketbase/pocketbase/core"
)

// TimelineQueryParams represents options for fetching timeline events for a subscriber
type TimelineQueryParams struct {
	SubscriberID int        // Required: the subscriber to fetch events for
	Limit        int        // Default: 50; max results per query
	Offset       int        // Default: 0; pagination offset
	EventTypes   []string   // Optional: filter by specific event types (campaign_send, inbound_sms, etc.)
	StartDate    *time.Time // Optional: only events on or after this date
	EndDate      *time.Time // Optional: only events before this date
	SortOrder    string     // Default: "desc" (newest first); "asc" for oldest first
}

// GetUnifiedContactTimeline returns merged timeline events (outbound activity + inbound SMS + inbound email replies)
// for a subscriber, sorted by occurrence timestamp.
//
// Implementation strategy (Phase 5):
// 1. Query campaign_send_ledger for campaign sends (with open/click counts)
// 2. Query campaign_views and link_clicks tables for campaign view/click events
// 3. Query inbound_sms_events collection for inbound SMS events
// 4. Query inbound_email_replies collection for inbound email replies
// 5. Merge all events into []TimelineEvent structs
// 6. Sort by occurred_at DESC, then by rowid DESC (stable tiebreaker)
// 7. Apply pagination (offset/limit)
// 8. Return UnifiedContactTimeline with total count and has_more indicator
//
// Contract:
// - Events are sorted newest-first by occurred_at (DESC), with rowid as stable tiebreaker
// - All events share the canonical TimelineEvent struct with event_type, channel, occurred_at, actor, status, metadata
// - Outbound events reuse existing campaign_send_ledger, campaign_views, link_clicks data
// - Inbound events use new inbound_sms_events and inbound_email_replies collections
// - Returns complete UnifiedContactTimeline with pagination metadata
//
// Potential filtering:
// - By event type (e.g., show only campaign sends)
// - By date range
// - By channel (email vs SMS)
//
// Note: This is defined as an interface; implementation in Phase 5.
func (c *Core) GetUnifiedContactTimeline(ctx context.Context, params TimelineQueryParams) (*models.UnifiedContactTimeline, error) {
	_ = ctx
	if params.SubscriberID <= 0 {
		return &models.UnifiedContactTimeline{
			Events:  []models.TimelineEvent{},
			Total:   0,
			HasMore: false,
			Offset:  0,
			Limit:   0,
		}, nil
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	sortOrder := strings.ToLower(strings.TrimSpace(params.SortOrder))
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	allowedTypes := map[string]struct{}{
		models.TimelineEventCampaignSend:      {},
		models.TimelineEventCampaignView:      {},
		models.TimelineEventLinkClick:         {},
		models.TimelineEventInboundSMS:        {},
		models.TimelineEventInboundEmailReply: {},
	}
	requestedTypes := map[string]struct{}{}
	if len(params.EventTypes) > 0 {
		for _, t := range params.EventTypes {
			t = strings.TrimSpace(t)
			if _, ok := allowedTypes[t]; ok {
				requestedTypes[t] = struct{}{}
			}
		}
	}
	hasTypeFilter := len(requestedTypes) > 0

	type sortableTimelineEvent struct {
		Event models.TimelineEvent
		RowID int64
	}
	all := make([]sortableTimelineEvent, 0, 128)

	includeType := func(eventType string) bool {
		if !hasTypeFilter {
			return true
		}
		_, ok := requestedTypes[eventType]
		return ok
	}
	inDateRange := func(ts time.Time) bool {
		if ts.IsZero() {
			return false
		}
		ts = ts.UTC()
		if params.StartDate != nil && ts.Before(params.StartDate.UTC()) {
			return false
		}
		if params.EndDate != nil && !ts.Before(params.EndDate.UTC()) {
			return false
		}
		return true
	}
	appendEvent := func(eventType string, occurredAt time.Time, rowID int64, channel, source, status string, actor models.TimelineActor, metadata any) {
		if !includeType(eventType) || !inDateRange(occurredAt) {
			return
		}
		meta, err := json.Marshal(metadata)
		if err != nil {
			meta = []byte("{}")
		}
		all = append(all, sortableTimelineEvent{
			Event: models.TimelineEvent{
				EventType:  eventType,
				Channel:    channel,
				OccurredAt: occurredAt.UTC(),
				Source:     source,
				Actor:      actor,
				Status:     status,
				Metadata:   meta,
			},
			RowID: rowID,
		})
	}

	subscriberRecordID, err := c.subscriberRecordIDByRowID(params.SubscriberID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &models.UnifiedContactTimeline{
				Events:  []models.TimelineEvent{},
				Total:   0,
				HasMore: false,
				Offset:  offset,
				Limit:   limit,
			}, nil
		}
		return nil, err
	}

	if includeType(models.TimelineEventCampaignSend) {
		rows := []struct {
			LedgerRowID      int64          `db:"ledger_rowid"`
			CampaignRowID    int            `db:"campaign_rowid"`
			CampaignRecordID string         `db:"campaign_record_id"`
			CampaignUUID     sql.NullString `db:"campaign_uuid"`
			CampaignName     sql.NullString `db:"campaign_name"`
			CampaignSubject  sql.NullString `db:"campaign_subject"`
			Status           string         `db:"status"`
			OccurredAtRaw    sql.NullString `db:"occurred_at"`
			ViewCount        int            `db:"view_count"`
			ClickCount       int            `db:"click_count"`
			FirstOpenedAtRaw sql.NullString `db:"first_opened_at"`
			FirstClickedRaw  sql.NullString `db:"first_clicked_at"`
		}{}
		if err := c.db.Select(&rows, `
			SELECT
				l.rowid AS ledger_rowid,
				c.rowid AS campaign_rowid,
				c.id AS campaign_record_id,
				c.uuid AS campaign_uuid,
				c.name AS campaign_name,
				c.subject AS campaign_subject,
				l.status,
				COALESCE(NULLIF(TRIM(l.updated), ''), l.created) AS occurred_at,
				(
					SELECT COUNT(*)
					FROM campaign_views cv
					WHERE cv.campaign_id = l.campaign_id
					  AND cv.subscriber_id = l.subscriber_id
					  AND COALESCE(cv.is_suspected_privacy_open, 0) = 0
				) AS view_count,
				(
					SELECT COUNT(*)
					FROM link_clicks lc
					WHERE lc.campaign_id = l.campaign_id
					  AND lc.subscriber_id = l.subscriber_id
				) AS click_count,
				(
					SELECT MIN(cv.created)
					FROM campaign_views cv
					WHERE cv.campaign_id = l.campaign_id
					  AND cv.subscriber_id = l.subscriber_id
					  AND COALESCE(cv.is_suspected_privacy_open, 0) = 0
				) AS first_opened_at,
				(
					SELECT MIN(lc.created)
					FROM link_clicks lc
					WHERE lc.campaign_id = l.campaign_id
					  AND lc.subscriber_id = l.subscriber_id
				) AS first_clicked_at
			FROM campaign_send_ledger l
			JOIN subscribers s ON s.id = l.subscriber_id
			JOIN campaigns c ON c.id = l.campaign_id
			WHERE s.rowid = ?
		`, params.SubscriberID); err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "timeline", "error", pqErrMsg(err)))
		}
		for _, r := range rows {
			occurredAt, err := parseSQLiteDateTime(r.OccurredAtRaw.String)
			if err != nil {
				continue
			}
			var firstOpenedAt *time.Time
			if strings.TrimSpace(r.FirstOpenedAtRaw.String) != "" {
				if v, err := parseSQLiteDateTime(r.FirstOpenedAtRaw.String); err == nil {
					firstOpenedAt = &v
				}
			}
			var firstClickedAt *time.Time
			if strings.TrimSpace(r.FirstClickedRaw.String) != "" {
				if v, err := parseSQLiteDateTime(r.FirstClickedRaw.String); err == nil {
					firstClickedAt = &v
				}
			}

			appendEvent(
				models.TimelineEventCampaignSend,
				occurredAt,
				r.LedgerRowID,
				models.ChannelEmail,
				"campaign_send_ledger",
				strings.TrimSpace(r.Status),
				models.TimelineActor{
					Type:  "campaign",
					ID:    strings.TrimSpace(r.CampaignRecordID),
					Label: strings.TrimSpace(r.CampaignName.String),
				},
				models.TimelineEventCampaignSendMetadata{
					CampaignID:     r.CampaignRowID,
					CampaignName:   strings.TrimSpace(r.CampaignName.String),
					CampaignUUID:   strings.TrimSpace(r.CampaignUUID.String),
					Subject:        strings.TrimSpace(r.CampaignSubject.String),
					MessageID:      strings.TrimSpace(r.CampaignRecordID),
					HasOpened:      r.ViewCount > 0,
					HasClicked:     r.ClickCount > 0,
					FirstOpenedAt:  firstOpenedAt,
					FirstClickedAt: firstClickedAt,
				},
			)
		}
	}

	if includeType(models.TimelineEventCampaignView) {
		rows := []struct {
			ViewRowID        int64          `db:"view_rowid"`
			CampaignRowID    int            `db:"campaign_rowid"`
			CampaignRecordID string         `db:"campaign_record_id"`
			CampaignUUID     sql.NullString `db:"campaign_uuid"`
			CampaignName     sql.NullString `db:"campaign_name"`
			CampaignSubject  sql.NullString `db:"campaign_subject"`
			ViewCount        int            `db:"view_count"`
			LastViewedAtRaw  sql.NullString `db:"last_viewed_at"`
		}{}
		if err := c.db.Select(&rows, `
			SELECT
				MAX(cv.rowid) AS view_rowid,
				c.rowid AS campaign_rowid,
				c.id AS campaign_record_id,
				c.uuid AS campaign_uuid,
				c.name AS campaign_name,
				c.subject AS campaign_subject,
				COUNT(*) AS view_count,
				MAX(cv.created) AS last_viewed_at
			FROM campaign_views cv
			LEFT JOIN campaigns c ON c.id = cv.campaign_id
			LEFT JOIN subscribers s ON s.id = cv.subscriber_id
			WHERE s.rowid = ?
			  AND COALESCE(cv.is_suspected_privacy_open, 0) = 0
			GROUP BY c.rowid, c.id, c.uuid, c.name, c.subject
		`, params.SubscriberID); err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "timeline", "error", pqErrMsg(err)))
		}
		for _, r := range rows {
			occurredAt, err := parseSQLiteDateTime(r.LastViewedAtRaw.String)
			if err != nil {
				continue
			}
			appendEvent(
				models.TimelineEventCampaignView,
				occurredAt,
				r.ViewRowID,
				models.ChannelEmail,
				"campaign_views",
				"viewed",
				models.TimelineActor{
					Type:  "campaign",
					ID:    strings.TrimSpace(r.CampaignRecordID),
					Label: strings.TrimSpace(r.CampaignName.String),
				},
				models.TimelineEventCampaignViewMetadata{
					CampaignID:   r.CampaignRowID,
					CampaignName: strings.TrimSpace(r.CampaignName.String),
					CampaignUUID: strings.TrimSpace(r.CampaignUUID.String),
					Subject:      strings.TrimSpace(r.CampaignSubject.String),
					ViewCount:    r.ViewCount,
				},
			)
		}
	}

	if includeType(models.TimelineEventLinkClick) {
		rows := []struct {
			ClickRowID       int64          `db:"click_rowid"`
			CampaignRowID    int            `db:"campaign_rowid"`
			CampaignRecordID sql.NullString `db:"campaign_record_id"`
			CampaignUUID     sql.NullString `db:"campaign_uuid"`
			CampaignName     sql.NullString `db:"campaign_name"`
			CampaignSubject  sql.NullString `db:"campaign_subject"`
			LinkURL          sql.NullString `db:"link_url"`
			ClickCount       int            `db:"click_count"`
			LastClickedAtRaw sql.NullString `db:"last_clicked_at"`
		}{}
		if err := c.db.Select(&rows, `
			SELECT
				MAX(lc.rowid) AS click_rowid,
				COALESCE(c.rowid, 0) AS campaign_rowid,
				c.id AS campaign_record_id,
				c.uuid AS campaign_uuid,
				c.name AS campaign_name,
				c.subject AS campaign_subject,
				l.url AS link_url,
				COUNT(*) AS click_count,
				MAX(lc.created) AS last_clicked_at
			FROM link_clicks lc
			LEFT JOIN links l ON l.id = lc.link_id
			LEFT JOIN campaigns c ON c.id = lc.campaign_id
			LEFT JOIN subscribers s ON s.id = lc.subscriber_id
			WHERE s.rowid = ?
			GROUP BY c.rowid, c.id, c.uuid, c.name, c.subject, l.url
		`, params.SubscriberID); err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "timeline", "error", pqErrMsg(err)))
		}
		for _, r := range rows {
			occurredAt, err := parseSQLiteDateTime(r.LastClickedAtRaw.String)
			if err != nil {
				continue
			}
			appendEvent(
				models.TimelineEventLinkClick,
				occurredAt,
				r.ClickRowID,
				models.ChannelEmail,
				"link_clicks",
				"clicked",
				models.TimelineActor{
					Type:  "campaign",
					ID:    strings.TrimSpace(r.CampaignRecordID.String),
					Label: strings.TrimSpace(r.CampaignName.String),
				},
				models.TimelineEventLinkClickMetadata{
					CampaignID:   r.CampaignRowID,
					CampaignName: strings.TrimSpace(r.CampaignName.String),
					CampaignUUID: strings.TrimSpace(r.CampaignUUID.String),
					Subject:      strings.TrimSpace(r.CampaignSubject.String),
					URL:          strings.TrimSpace(r.LinkURL.String),
					ClickCount:   r.ClickCount,
				},
			)
		}
	}

	if includeType(models.TimelineEventInboundSMS) {
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
		`, subscriberRecordID); err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "timeline", "error", pqErrMsg(err)))
		}
		for _, row := range rows {
			e, mapErr := mapInboundSMSRow(row)
			if mapErr != nil {
				continue
			}
			appendEvent(
				models.TimelineEventInboundSMS,
				e.ReceivedAt,
				int64(row.ID),
				models.ChannelSMS,
				"inbound_sms_events",
				models.InboundSMSStatusReceived,
				models.TimelineActor{
					Type:  "provider",
					ID:    strings.TrimSpace(e.ProviderID),
					Label: strings.TrimSpace(e.ProviderID),
				},
				models.TimelineEventInboundSMSMetadata{
					FromNumber:    strings.TrimSpace(e.FromNumber),
					MessageBody:   strings.TrimSpace(e.MessageBody),
					ProviderID:    strings.TrimSpace(e.ProviderID),
					ProviderMsgID: strings.TrimSpace(e.ProviderMsgID),
					IsStopKeyword: e.IsStopKeyword,
					MatchScore:    strings.TrimSpace(e.MatchScore),
				},
			)
		}
	}

	if includeType(models.TimelineEventInboundEmailReply) {
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
		`, subscriberRecordID); err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "timeline", "error", pqErrMsg(err)))
		}
		for _, row := range rows {
			e, mapErr := mapInboundEmailReplyRow(row)
			if mapErr != nil {
				continue
			}
			appendEvent(
				models.TimelineEventInboundEmailReply,
				e.ReceivedAt,
				int64(row.ID),
				models.ChannelEmail,
				"inbound_email_replies",
				models.InboundEmailStatusReceived,
				models.TimelineActor{
					Type:  "provider",
					ID:    "email_reply",
					Label: "Email Reply",
				},
				models.TimelineEventInboundEmailReplyMetadata{
					InboundEmailReplyID: strings.TrimSpace(row.RecordID),
					FromAddress:         strings.TrimSpace(e.FromAddress),
					Subject:             strings.TrimSpace(e.Subject),
					BodySnippet:         strings.TrimSpace(e.BodySnippet),
					MessageID:           strings.TrimSpace(e.MessageID),
					InReplyTo:           strings.TrimSpace(e.InReplyTo),
					References:          strings.TrimSpace(e.References),
					HasAttachments:      e.HasAttachments,
					MatchScore:          strings.TrimSpace(e.MatchScore),
					SpamStatus:          strings.TrimSpace(e.SpamStatus),
				},
			)
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		lhs := all[i]
		rhs := all[j]
		if lhs.Event.OccurredAt.Equal(rhs.Event.OccurredAt) {
			if sortOrder == "asc" {
				return lhs.RowID < rhs.RowID
			}
			return lhs.RowID > rhs.RowID
		}
		if sortOrder == "asc" {
			return lhs.Event.OccurredAt.Before(rhs.Event.OccurredAt)
		}
		return lhs.Event.OccurredAt.After(rhs.Event.OccurredAt)
	})

	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}

	events := make([]models.TimelineEvent, 0, end-offset)
	for _, item := range all[offset:end] {
		events = append(events, item.Event)
	}

	return &models.UnifiedContactTimeline{
		Events:  events,
		Total:   total,
		HasMore: end < total,
		Offset:  offset,
		Limit:   limit,
	}, nil
}

// GetInboundSMSEventsBySubscriber returns all inbound SMS events for a subscriber, ordered by received_at DESC
// Used by Phase 3 webhook handler and timeline assembly in Phase 5.
//
// Implementation strategy (Phase 3):
// - Query inbound_sms_events where subscriber_id = params.SubscriberID
// - Order by received_at DESC, rowid DESC
// - Apply offset/limit pagination
// - Return events as []InboundSMSEvent
//
// Note: This is defined as an interface; implementation in Phase 3.
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
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "inbound sms events", "error", pqErrMsg(err)))
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
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "inbound sms events", "error", pqErrMsg(err)))
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
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "inbound email replies", "error", pqErrMsg(err)))
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
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "inbound email replies", "error", pqErrMsg(err)))
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
		var recID string
		err := c.db.Get(&recID, `SELECT id FROM campaign_send_ledger WHERE id = ? LIMIT 1`, candidate)
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
	ID              int            `db:"id"`
	RecordID        string         `db:"record_id"`
	CreatedAtRaw    sql.NullString `db:"created_at"`
	UpdatedAtRaw    sql.NullString `db:"updated_at"`
	SubscriberID    sql.NullString `db:"subscriber_id"`
	LinkedMessageID sql.NullString `db:"linked_message_id"`
	FromAddress     string         `db:"from_address"`
	Subject         string         `db:"subject"`
	MessageID       string         `db:"message_id"`
	InReplyTo       string         `db:"in_reply_to"`
	References      string         `db:"references"`
	ReceivedAtRaw   sql.NullString `db:"received_at"`
	BodySnippet     string         `db:"body_snippet"`
	BodyHTML        string         `db:"body_html"`
	BodyText        string         `db:"body_text"`
	ToAddress       string         `db:"to_address"`
	CC              string         `db:"cc"`
	ReplyTo         string         `db:"reply_to"`
	StructuredHeadersRaw any       `db:"structured_headers"`
	HasAttachments  bool           `db:"has_attachments"`
	MatchScore      string         `db:"match_score"`
	SpamStatus      string         `db:"spam_status"`
	SpamScore       float64        `db:"spam_score"`
	ProcessedAtRaw  sql.NullString `db:"processed_at"`
	DedupeKey       string         `db:"dedupe_key"`
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
func spamLevelOrder(level string) int {
	switch level {
	case "confirmed_spam":
		return 3
	case "spam":
		return 2
	case "suspected":
		return 1
	}
	return 0
}

// maxSpamLevel returns the higher of two spam level strings.
func maxSpamLevel(a, b string) string {
	if spamLevelOrder(a) >= spamLevelOrder(b) {
		return a
	}
	return b
}

// spamKeywords is a minimal English stop-word set used to filter low-value keywords.
var spamStopWords = map[string]struct{}{
	"that": {}, "this": {}, "with": {}, "from": {}, "have": {}, "will": {},
	"your": {}, "been": {}, "they": {}, "were": {}, "said": {}, "each": {},
	"which": {}, "their": {}, "there": {}, "when": {}, "what": {}, "some": {},
	"about": {}, "would": {}, "these": {}, "other": {}, "into": {}, "than": {},
	"then": {}, "more": {}, "also": {}, "click": {}, "here": {}, "http": {},
	"https": {}, "email": {}, "message": {}, "reply": {},
}

// extractSpamKeywords extracts up to maxN significant words from text (subject + body).
func extractSpamKeywords(subject, bodyText string, maxN int) []string {
	text := strings.ToLower(subject + " " + bodyText)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	seen := map[string]int{}
	for _, w := range words {
		if len(w) < 4 {
			continue
		}
		if _, stop := spamStopWords[w]; stop {
			continue
		}
		seen[w]++
	}
	// Sort by frequency descending.
	type wf struct {
		word  string
		count int
	}
	ranked := make([]wf, 0, len(seen))
	for w, c := range seen {
		ranked = append(ranked, wf{w, c})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].count > ranked[j].count
	})
	out := make([]string, 0, maxN)
	for i, w := range ranked {
		if i >= maxN {
			break
		}
		out = append(out, w.word)
	}
	return out
}

// extractDomain returns the domain part of an email address.
func extractDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}

// CheckInboundSpamRules evaluates active spam rules against the incoming email fields.
// It checks sender address, sender domain, then keyword scoring.
// Returns the highest implied spam level and composite score, or ("", 0, nil) if no match.
func (c *Core) CheckInboundSpamRules(ctx context.Context, fromAddress, subject, bodyText string) (string, float64, error) {
	_ = ctx
	fromAddress = strings.ToLower(strings.TrimSpace(fromAddress))
	if fromAddress == "" {
		return "", 0, nil
	}
	domain := extractDomain(fromAddress)

	type ruleRow struct {
		Type      string  `db:"type"`
		Value     string  `db:"value"`
		Weight    float64 `db:"weight"`
		SpamLevel string  `db:"spam_level"`
	}
	rules := []ruleRow{}
	if err := c.db.Select(&rules, `
		SELECT type, value, COALESCE(weight, 1.0) AS weight, COALESCE(spam_level, 'suspected') AS spam_level
		FROM inbound_spam_rules
		WHERE is_active = 1
		  AND spam_level != ''
		ORDER BY rowid ASC
	`); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", 0, err
	}

	bestLevel := ""
	var keywordRules []ruleRow
	for _, r := range rules {
		switch r.Type {
		case "sender":
			if strings.EqualFold(r.Value, fromAddress) {
				bestLevel = maxSpamLevel(bestLevel, r.SpamLevel)
			}
		case "domain":
			if domain != "" && strings.EqualFold(r.Value, domain) {
				bestLevel = maxSpamLevel(bestLevel, r.SpamLevel)
			}
		case "keyword":
			keywordRules = append(keywordRules, r)
		}
	}

	// If sender/domain rule already determined spam or confirmed_spam, return early.
	if spamLevelOrder(bestLevel) >= spamLevelOrder("spam") {
		return bestLevel, 1.0, nil
	}

	// Keyword scoring.
	var keywordScore float64
	if len(keywordRules) > 0 {
		keywords := extractSpamKeywords(subject, bodyText, 20)
		kwSet := make(map[string]struct{}, len(keywords))
		for _, kw := range keywords {
			kwSet[kw] = struct{}{}
		}
		var totalWeight, matchWeight float64
		for _, kr := range keywordRules {
			totalWeight += kr.Weight
			if _, ok := kwSet[strings.ToLower(kr.Value)]; ok {
				matchWeight += kr.Weight
			}
		}
		if totalWeight > 0 {
			keywordScore = matchWeight / totalWeight
		}
		// Mark as suspected if keyword score exceeds threshold.
		if keywordScore >= 0.3 {
			bestLevel = maxSpamLevel(bestLevel, "suspected")
		}
	}

	return bestLevel, keywordScore, nil
}

// InboxQueryParams contains filter options for the unified inbox listing.
type InboxQueryParams struct {
	Limit      int
	Offset     int
	Search     string    // filter by from_address or subject (partial match)
	SpamStatus string    // filter by spam_status value (empty = non-spam only, "all" = all)
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
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "inbox", "error", pqErrMsg(err)))
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
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "inbox", "error", pqErrMsg(err)))
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
		return nil, echo.NewHTTPError(http.StatusBadRequest, "id is required")
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
			return nil, echo.NewHTTPError(http.StatusNotFound, "inbound email not found")
		}
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "inbound email", "error", pqErrMsg(err)))
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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid spam_status value")
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return fmt.Errorf("pocketbase is not initialized")
	}
	rec, err := pb.FindRecordById("inbound_email_replies", id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "inbound email not found")
	}

	rec.Set("spam_status", spamStatus)
	if err := pb.Save(rec); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update spam status")
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
func (c *Core) LearnSpamFromEmail(ctx context.Context, fromAddress, subject, bodyText, spamStatus string) error {
	_ = ctx
	fromAddress = strings.ToLower(strings.TrimSpace(fromAddress))
	if fromAddress == "" {
		return nil
	}
	domain := extractDomain(fromAddress)

	upsertRule := func(ruleType, value string) error {
		if value == "" {
			return nil
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if err := c.upsertSpamRule(ruleType, value, 1.0, spamStatus); err != nil {
			c.log.Printf("upsert spam rule type=%q value=%q: %v", ruleType, value, err)
		}
		return nil
	}

	_ = upsertRule("sender", fromAddress)
	if domain != "" {
		_ = upsertRule("domain", domain)
	}

	keywords := extractSpamKeywords(subject, bodyText, 10)
	for _, kw := range keywords {
		_ = upsertRule("keyword", kw)
	}
	return nil
}

// upsertSpamRule creates or updates a spam rule, incrementing hit_count and applying the
// max spam level observed.
func (c *Core) upsertSpamRule(ruleType, value string, weight float64, spamLevel string) error {
	pb := c.db.PocketBase()
	if pb == nil {
		return fmt.Errorf("pocketbase is not initialized")
	}
	collection, err := pb.FindCollectionByNameOrId("inbound_spam_rules")
	if err != nil {
		return err
	}

	// Try to find existing rule.
	existing, err := pb.FindFirstRecordByFilter("inbound_spam_rules",
		fmt.Sprintf(`type = "%s" && value = "%s"`,
			strings.ReplaceAll(ruleType, `"`, ``),
			strings.ReplaceAll(value, `"`, ``)))
	if err == nil && existing != nil {
		// Update existing: increment hit_count, apply max spam_level.
		hitCount := int(toFloat(existing.Get("hit_count"))) + 1
		existing.Set("hit_count", hitCount)
		existing.Set("spam_level", maxSpamLevel(toString(existing.Get("spam_level")), spamLevel))
		existing.Set("is_active", true)
		return pb.Save(existing)
	}

	// Create new rule.
	rec := pbcore.NewRecord(collection)
	rec.Set("type", ruleType)
	rec.Set("value", value)
	rec.Set("weight", weight)
	rec.Set("hit_count", 1)
	rec.Set("spam_level", spamLevel)
	rec.Set("is_active", true)
	return pb.Save(rec)
}

// GetInboundSpamRules returns paginated spam rules.
func (c *Core) GetInboundSpamRules(ctx context.Context, limit, offset int, ruleType string) ([]models.InboundSpamRule, int, error) {
	_ = ctx
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	conds := []string{}
	args := []any{}
	if ruleType != "" {
		conds = append(conds, "type = ?")
		args = append(args, ruleType)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := c.db.Get(&total, "SELECT COUNT(*) FROM inbound_spam_rules "+where, args...); err != nil {
		return nil, 0, err
	}

	type ruleRow struct {
		RecordID  string         `db:"record_id"`
		Type      string         `db:"type"`
		Value     string         `db:"value"`
		Weight    float64        `db:"weight"`
		HitCount  int            `db:"hit_count"`
		SpamLevel string         `db:"spam_level"`
		IsActive  bool           `db:"is_active"`
		CreatedAt sql.NullString `db:"created_at"`
		UpdatedAt sql.NullString `db:"updated_at"`
	}
	rows := []ruleRow{}
	listArgs := append(args, limit, offset)
	if err := c.db.Select(&rows, `
		SELECT
			id AS record_id,
			type,
			value,
			COALESCE(weight, 1.0) AS weight,
			COALESCE(hit_count, 0) AS hit_count,
			COALESCE(spam_level, '') AS spam_level,
			COALESCE(is_active, 0) AS is_active,
			created AS created_at,
			updated AS updated_at
		FROM inbound_spam_rules
		`+where+`
		ORDER BY hit_count DESC, rowid DESC
		LIMIT ? OFFSET ?
	`, listArgs...); err != nil {
		return nil, 0, err
	}

	out := make([]models.InboundSpamRule, 0, len(rows))
	for _, r := range rows {
		created, _ := parseSQLiteDateTime(r.CreatedAt.String)
		updated, _ := parseSQLiteDateTime(r.UpdatedAt.String)
		out = append(out, models.InboundSpamRule{
			RecordID:  r.RecordID,
			Type:      r.Type,
			Value:     r.Value,
			Weight:    r.Weight,
			HitCount:  r.HitCount,
			SpamLevel: r.SpamLevel,
			IsActive:  r.IsActive,
			CreatedAt: created,
			UpdatedAt: updated,
		})
	}
	return out, total, nil
}

// DeleteInboundSpamRule removes a spam rule by its PocketBase record ID.
func (c *Core) DeleteInboundSpamRule(ctx context.Context, id string) error {
	_ = ctx
	pb := c.db.PocketBase()
	if pb == nil {
		return fmt.Errorf("pocketbase is not initialized")
	}
	rec, err := pb.FindRecordById("inbound_spam_rules", id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "spam rule not found")
	}
	return pb.Delete(rec)
}

// DeleteSpamInboundEmails removes inbound emails marked as spam or confirmed_spam that are
// older than 7 days, along with their associated attachment records.
// Returns the number of deleted email records.
func (c *Core) DeleteSpamInboundEmails(ctx context.Context) (int, error) {
	_ = ctx
	pb := c.db.PocketBase()
	if pb == nil {
		return 0, fmt.Errorf("pocketbase is not initialized")
	}

	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)

	// Find spam email record IDs to delete.
	type idRow struct {
		RecordID string `db:"record_id"`
	}
	rows := []idRow{}
	if err := c.db.Select(&rows, `
		SELECT id AS record_id
		FROM inbound_email_replies
		WHERE spam_status IN ('spam', 'confirmed_spam')
		  AND received_at < ?
	`, cutoff); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	deleted := 0
	for _, r := range rows {
		// Delete attachment records first (PocketBase cascades file deletion).
		attachmentRecords, err := pb.FindRecordsByFilter("inbound_email_attachments",
			fmt.Sprintf(`inbound_email_reply_id = "%s"`, strings.ReplaceAll(r.RecordID, `"`, ``)), "", 0, 200)
		if err == nil {
			for _, a := range attachmentRecords {
				_ = pb.Delete(a)
			}
		}
		emailRec, err := pb.FindRecordById("inbound_email_replies", r.RecordID)
		if err != nil {
			continue
		}
		if err := pb.Delete(emailRec); err != nil {
			c.log.Printf("spam gc: failed to delete email record_id=%q: %v", r.RecordID, err)
			continue
		}
		deleted++
	}
	return deleted, nil
}

// toFloat converts an arbitrary value to float64, returning 0 on failure.
func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		var f float64
		_, _ = fmt.Sscanf(t, "%f", &f)
		return f
	}
	return 0
}

// toString converts an arbitrary value to string.
func toString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

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

func parseSQLiteDateTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid datetime: %q", raw)
}

func decodeJSONFieldToModelJSON(v any) (models.JSON, error) {
	out := models.JSON{}
	if v == nil {
		return out, nil
	}
	var data []byte
	switch t := v.(type) {
	case []byte:
		data = t
	case string:
		data = []byte(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		data = b
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
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

func (c *Core) subscriberRecordIDByRowID(subscriberID int) (string, error) {
	var recID string
	if err := c.db.Get(&recID, `SELECT id FROM subscribers WHERE rowid = ?`, subscriberID); err != nil {
		return "", err
	}
	return recID, nil
}

func (c *Core) inferSingleListRecordIDForSubscriberRow(subscriberID int) (string, error) {
	rows := []struct {
		ListRecordID string `db:"list_record_id"`
	}{}
	err := c.db.Select(&rows, `
		SELECT DISTINCT l.id AS list_record_id
		FROM subscriber_lists sl
		JOIN subscribers s ON s.id = sl.subscriber_id
		JOIN lists l ON l.id = sl.list_id
		WHERE s.rowid = ?
		ORDER BY l.rowid ASC
		LIMIT 2
	`, subscriberID)
	if err != nil {
		return "", err
	}
	if len(rows) != 1 {
		return "", nil
	}
	return rows[0].ListRecordID, nil
}
