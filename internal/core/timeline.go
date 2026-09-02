package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"

	"github.com/compdani/list_pocket/models"
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

type timelineKeyRow struct {
	EventType  string `db:"event_type"`
	OccurredAt string `db:"occurred_at"`
	RowID      int64  `db:"event_rowid"`
	Ref1       string `db:"ref1"`
	Ref2       string `db:"ref2"`
}

type timelineSendHydrateRow struct {
	LedgerRowID      int64          `db:"ledger_rowid"`
	CampaignRowID    int            `db:"campaign_rowid"`
	CampaignRecordID string         `db:"campaign_record_id"`
	CampaignUUID     sql.NullString `db:"campaign_uuid"`
	CampaignName     sql.NullString `db:"campaign_name"`
	CampaignSubject  sql.NullString `db:"campaign_subject"`
	Status           string         `db:"status"`
}

type timelineViewHydrateRow struct {
	CampaignRecordID string         `db:"campaign_record_id"`
	CampaignRowID    int            `db:"campaign_rowid"`
	CampaignUUID     sql.NullString `db:"campaign_uuid"`
	CampaignName     sql.NullString `db:"campaign_name"`
	CampaignSubject  sql.NullString `db:"campaign_subject"`
	ViewCount        int            `db:"view_count"`
}

type timelineClickHydrateRow struct {
	CampaignRecordID string         `db:"campaign_record_id"`
	CampaignRowID    int            `db:"campaign_rowid"`
	CampaignUUID     sql.NullString `db:"campaign_uuid"`
	CampaignName     sql.NullString `db:"campaign_name"`
	CampaignSubject  sql.NullString `db:"campaign_subject"`
	LinkURL          sql.NullString `db:"link_url"`
	ClickCount       int            `db:"click_count"`
}

type timelineEngagementRow struct {
	CampaignID string         `db:"campaign_id"`
	EventCount int            `db:"event_count"`
	FirstAtRaw sql.NullString `db:"first_at"`
}

// GetUnifiedContactTimeline returns merged timeline events (outbound activity + inbound SMS + inbound email replies)
// for a subscriber, sorted by occurrence timestamp.
//
// Pagination is done in SQL: a UNION ALL key query (send, grouped view, grouped click, SMS, email)
// is ordered and LIMIT/OFFSET'd, then the page is hydrated with batched IN lookups.
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
	sortDir := "DESC"
	if sortOrder == "asc" {
		sortDir = "ASC"
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
	includeType := func(eventType string) bool {
		if !hasTypeFilter {
			return true
		}
		_, ok := requestedTypes[eventType]
		return ok
	}

	empty := func() *models.UnifiedContactTimeline {
		return &models.UnifiedContactTimeline{
			Events:  []models.TimelineEvent{},
			Total:   0,
			HasMore: false,
			Offset:  offset,
			Limit:   limit,
		}
	}

	subscriberRecordID, err := c.subscriberRecordIDByRowID(params.SubscriberID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty(), nil
		}
		return nil, err
	}

	dateClause := func(expr string) (string, []any) {
		var clause strings.Builder
		var extra []any
		if params.StartDate != nil {
			clause.WriteString(" AND datetime(" + expr + ") >= datetime(?)")
			extra = append(extra, params.StartDate.UTC().Format(time.RFC3339Nano))
		}
		if params.EndDate != nil {
			clause.WriteString(" AND datetime(" + expr + ") < datetime(?)")
			extra = append(extra, params.EndDate.UTC().Format(time.RFC3339Nano))
		}
		return clause.String(), extra
	}

	var branches []string
	var args []any

	if includeType(models.TimelineEventCampaignSend) {
		occurred := `COALESCE(NULLIF(TRIM(l.updated), ''), l.created)`
		df, dargs := dateClause(occurred)
		branches = append(branches, `
			SELECT '`+models.TimelineEventCampaignSend+`' AS event_type,
				`+occurred+` AS occurred_at,
				l.rowid AS event_rowid,
				l.campaign_id AS ref1,
				'' AS ref2
			FROM campaign_send_ledger l
			JOIN subscribers s ON s.id = l.subscriber_id
			WHERE s.rowid = ?`+df)
		args = append(args, params.SubscriberID)
		args = append(args, dargs...)
	}

	if includeType(models.TimelineEventCampaignView) {
		df, dargs := dateClause(`MAX(cv.created)`)
		branches = append(branches, `
			SELECT '`+models.TimelineEventCampaignView+`' AS event_type,
				MAX(cv.created) AS occurred_at,
				MAX(cv.rowid) AS event_rowid,
				COALESCE(cv.campaign_id, '') AS ref1,
				'' AS ref2
			FROM campaign_views cv
			JOIN subscribers s ON s.id = cv.subscriber_id
			WHERE s.rowid = ?
			  AND COALESCE(cv.is_suspected_privacy_open, 0) = 0
			GROUP BY cv.campaign_id
			HAVING 1=1`+df)
		args = append(args, params.SubscriberID)
		args = append(args, dargs...)
	}

	if includeType(models.TimelineEventLinkClick) {
		df, dargs := dateClause(`MAX(lc.created)`)
		branches = append(branches, `
			SELECT '`+models.TimelineEventLinkClick+`' AS event_type,
				MAX(lc.created) AS occurred_at,
				MAX(lc.rowid) AS event_rowid,
				COALESCE(lc.campaign_id, '') AS ref1,
				COALESCE(l.url, '') AS ref2
			FROM link_clicks lc
			LEFT JOIN links l ON l.id = lc.link_id
			JOIN subscribers s ON s.id = lc.subscriber_id
			WHERE s.rowid = ?
			GROUP BY lc.campaign_id, l.url
			HAVING 1=1`+df)
		args = append(args, params.SubscriberID)
		args = append(args, dargs...)
	}

	if includeType(models.TimelineEventInboundSMS) {
		df, dargs := dateClause(`received_at`)
		branches = append(branches, `
			SELECT '`+models.TimelineEventInboundSMS+`' AS event_type,
				received_at AS occurred_at,
				rowid AS event_rowid,
				id AS ref1,
				'' AS ref2
			FROM inbound_sms_events
			WHERE subscriber_id = ?`+df)
		args = append(args, subscriberRecordID)
		args = append(args, dargs...)
	}

	if includeType(models.TimelineEventInboundEmailReply) {
		df, dargs := dateClause(`received_at`)
		branches = append(branches, `
			SELECT '`+models.TimelineEventInboundEmailReply+`' AS event_type,
				received_at AS occurred_at,
				rowid AS event_rowid,
				id AS ref1,
				'' AS ref2
			FROM inbound_email_replies
			WHERE subscriber_id = ?`+df)
		args = append(args, subscriberRecordID)
		args = append(args, dargs...)
	}

	if len(branches) == 0 {
		return empty(), nil
	}

	unionSQL := strings.Join(branches, " UNION ALL ")
	var total int
	if err := c.db.Get(&total, `SELECT COUNT(*) FROM (`+unionSQL+`)`, args...); err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "timeline", "error", dbErr(err)))
	}
	if offset > total {
		offset = total
	}
	if total == 0 || offset >= total {
		return &models.UnifiedContactTimeline{
			Events:  []models.TimelineEvent{},
			Total:   total,
			HasMore: false,
			Offset:  offset,
			Limit:   limit,
		}, nil
	}

	// Compound SELECTs may only ORDER BY result columns or positions. Wrap so
	// datetime(occurred_at) is a valid expression over the union result.
	pageSQL := `SELECT event_type, occurred_at, event_rowid, ref1, ref2 FROM (` + unionSQL + `) ORDER BY datetime(occurred_at) ` + sortDir + `, event_rowid ` + sortDir + ` LIMIT ? OFFSET ?`
	pageArgs := append(append([]any{}, args...), limit, offset)
	var keys []timelineKeyRow
	if err := c.db.Select(&keys, pageSQL, pageArgs...); err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "timeline", "error", dbErr(err)))
	}

	events, err := c.hydrateTimelinePage(params.SubscriberID, subscriberRecordID, keys)
	if err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "timeline", "error", dbErr(err)))
	}

	return &models.UnifiedContactTimeline{
		Events:  events,
		Total:   total,
		HasMore: offset+len(events) < total,
		Offset:  offset,
		Limit:   limit,
	}, nil
}

func (c *Core) hydrateTimelinePage(subscriberRowID int, subscriberRecordID string, keys []timelineKeyRow) ([]models.TimelineEvent, error) {
	events := make([]models.TimelineEvent, 0, len(keys))
	if len(keys) == 0 {
		return events, nil
	}

	sendRowIDs := make([]int64, 0)
	viewCampaignIDs := make([]string, 0)
	clickKeys := make([][2]string, 0)
	smsIDs := make([]string, 0)
	emailIDs := make([]string, 0)
	seenSend := map[int64]struct{}{}
	seenView := map[string]struct{}{}
	seenClick := map[string]struct{}{}
	seenSMS := map[string]struct{}{}
	seenEmail := map[string]struct{}{}

	for _, k := range keys {
		switch k.EventType {
		case models.TimelineEventCampaignSend:
			if _, ok := seenSend[k.RowID]; ok {
				continue
			}
			seenSend[k.RowID] = struct{}{}
			sendRowIDs = append(sendRowIDs, k.RowID)
		case models.TimelineEventCampaignView:
			if _, ok := seenView[k.Ref1]; ok {
				continue
			}
			seenView[k.Ref1] = struct{}{}
			viewCampaignIDs = append(viewCampaignIDs, k.Ref1)
		case models.TimelineEventLinkClick:
			ck := k.Ref1 + "\x00" + k.Ref2
			if _, ok := seenClick[ck]; ok {
				continue
			}
			seenClick[ck] = struct{}{}
			clickKeys = append(clickKeys, [2]string{k.Ref1, k.Ref2})
		case models.TimelineEventInboundSMS:
			if strings.TrimSpace(k.Ref1) == "" {
				continue
			}
			if _, ok := seenSMS[k.Ref1]; ok {
				continue
			}
			seenSMS[k.Ref1] = struct{}{}
			smsIDs = append(smsIDs, k.Ref1)
		case models.TimelineEventInboundEmailReply:
			if strings.TrimSpace(k.Ref1) == "" {
				continue
			}
			if _, ok := seenEmail[k.Ref1]; ok {
				continue
			}
			seenEmail[k.Ref1] = struct{}{}
			emailIDs = append(emailIDs, k.Ref1)
		}
	}

	sends, err := c.hydrateTimelineSends(subscriberRecordID, sendRowIDs)
	if err != nil {
		return nil, err
	}
	views, err := c.hydrateTimelineViews(subscriberRowID, viewCampaignIDs)
	if err != nil {
		return nil, err
	}
	clicks, err := c.hydrateTimelineClicks(subscriberRowID, clickKeys)
	if err != nil {
		return nil, err
	}
	smsByID, err := c.hydrateTimelineSMS(smsIDs)
	if err != nil {
		return nil, err
	}
	emailsByID, err := c.hydrateTimelineEmails(emailIDs)
	if err != nil {
		return nil, err
	}

	for _, k := range keys {
		occurredAt, err := parseSQLiteDateTime(k.OccurredAt)
		if err != nil || occurredAt.IsZero() {
			continue
		}
		switch k.EventType {
		case models.TimelineEventCampaignSend:
			row, ok := sends[k.RowID]
			if !ok {
				continue
			}
			events = append(events, models.TimelineEvent{
				EventType:  models.TimelineEventCampaignSend,
				Channel:    models.ChannelEmail,
				OccurredAt: occurredAt,
				Source:     "campaign_send_ledger",
				Actor: models.TimelineActor{
					Type:  "campaign",
					ID:    strings.TrimSpace(row.CampaignRecordID),
					Label: strings.TrimSpace(row.CampaignName.String),
				},
				Status:   strings.TrimSpace(row.Status),
				Metadata: marshalTimelineMetadata(row.Metadata),
			})
		case models.TimelineEventCampaignView:
			row, ok := views[k.Ref1]
			if !ok {
				continue
			}
			events = append(events, models.TimelineEvent{
				EventType:  models.TimelineEventCampaignView,
				Channel:    models.ChannelEmail,
				OccurredAt: occurredAt,
				Source:     "campaign_views",
				Actor: models.TimelineActor{
					Type:  "campaign",
					ID:    strings.TrimSpace(k.Ref1),
					Label: strings.TrimSpace(row.CampaignName),
				},
				Status:   "viewed",
				Metadata: marshalTimelineMetadata(row),
			})
		case models.TimelineEventLinkClick:
			row, ok := clicks[k.Ref1+"\x00"+k.Ref2]
			if !ok {
				continue
			}
			events = append(events, models.TimelineEvent{
				EventType:  models.TimelineEventLinkClick,
				Channel:    models.ChannelEmail,
				OccurredAt: occurredAt,
				Source:     "link_clicks",
				Actor: models.TimelineActor{
					Type:  "campaign",
					ID:    strings.TrimSpace(row.CampaignRecordID),
					Label: strings.TrimSpace(row.Meta.CampaignName),
				},
				Status:   "clicked",
				Metadata: marshalTimelineMetadata(row.Meta),
			})
		case models.TimelineEventInboundSMS:
			e, ok := smsByID[k.Ref1]
			if !ok {
				continue
			}
			events = append(events, models.TimelineEvent{
				EventType:  models.TimelineEventInboundSMS,
				Channel:    models.ChannelSMS,
				OccurredAt: e.ReceivedAt.UTC(),
				Source:     "inbound_sms_events",
				Actor: models.TimelineActor{
					Type:  "provider",
					ID:    strings.TrimSpace(e.ProviderID),
					Label: strings.TrimSpace(e.ProviderID),
				},
				Status: models.InboundSMSStatusReceived,
				Metadata: marshalTimelineMetadata(models.TimelineEventInboundSMSMetadata{
					FromNumber:    strings.TrimSpace(e.FromNumber),
					MessageBody:   strings.TrimSpace(e.MessageBody),
					ProviderID:    strings.TrimSpace(e.ProviderID),
					ProviderMsgID: strings.TrimSpace(e.ProviderMsgID),
					IsStopKeyword: e.IsStopKeyword,
					MatchScore:    strings.TrimSpace(e.MatchScore),
				}),
			})
		case models.TimelineEventInboundEmailReply:
			pack, ok := emailsByID[k.Ref1]
			if !ok {
				continue
			}
			events = append(events, models.TimelineEvent{
				EventType:  models.TimelineEventInboundEmailReply,
				Channel:    models.ChannelEmail,
				OccurredAt: pack.event.ReceivedAt.UTC(),
				Source:     "inbound_email_replies",
				Actor: models.TimelineActor{
					Type:  "provider",
					ID:    "email_reply",
					Label: "Email Reply",
				},
				Status: models.InboundEmailStatusReceived,
				Metadata: marshalTimelineMetadata(models.TimelineEventInboundEmailReplyMetadata{
					InboundEmailReplyID: strings.TrimSpace(pack.recordID),
					FromAddress:         strings.TrimSpace(pack.event.FromAddress),
					Subject:             strings.TrimSpace(pack.event.Subject),
					BodySnippet:         strings.TrimSpace(pack.event.BodySnippet),
					MessageID:           strings.TrimSpace(pack.event.MessageID),
					InReplyTo:           strings.TrimSpace(pack.event.InReplyTo),
					References:          strings.TrimSpace(pack.event.References),
					HasAttachments:      pack.event.HasAttachments,
					MatchScore:          strings.TrimSpace(pack.event.MatchScore),
					SpamStatus:          strings.TrimSpace(pack.event.SpamStatus),
				}),
			})
		}
	}

	return events, nil
}

type timelineSendHydrated struct {
	CampaignRecordID string
	CampaignName     sql.NullString
	Status           string
	Metadata         models.TimelineEventCampaignSendMetadata
}

func (c *Core) hydrateTimelineSends(subscriberRecordID string, ledgerRowIDs []int64) (map[int64]timelineSendHydrated, error) {
	out := map[int64]timelineSendHydrated{}
	if len(ledgerRowIDs) == 0 {
		return out, nil
	}

	args := make([]any, len(ledgerRowIDs))
	for i, id := range ledgerRowIDs {
		args[i] = id
	}
	var rows []timelineSendHydrateRow
	if err := c.db.Select(&rows, `
		SELECT
			l.rowid AS ledger_rowid,
			c.rowid AS campaign_rowid,
			c.id AS campaign_record_id,
			c.uuid AS campaign_uuid,
			c.name AS campaign_name,
			c.subject AS campaign_subject,
			l.status
		FROM campaign_send_ledger l
		JOIN campaigns c ON c.id = l.campaign_id
		WHERE l.rowid IN (`+sqlitePlaceholders(len(ledgerRowIDs))+`)
	`, args...); err != nil {
		return nil, err
	}

	campaignIDs := make([]string, 0, len(rows))
	seenCamp := map[string]struct{}{}
	for _, r := range rows {
		id := strings.TrimSpace(r.CampaignRecordID)
		if id == "" {
			continue
		}
		if _, ok := seenCamp[id]; ok {
			continue
		}
		seenCamp[id] = struct{}{}
		campaignIDs = append(campaignIDs, id)
	}

	opens, err := c.timelineEngagementByCampaign(subscriberRecordID, campaignIDs, `
		SELECT campaign_id, COUNT(*) AS event_count, MIN(created) AS first_at
		FROM campaign_views
		WHERE subscriber_id = ?
		  AND campaign_id IN (`+sqlitePlaceholders(len(campaignIDs))+`)
		  AND COALESCE(is_suspected_privacy_open, 0) = 0
		GROUP BY campaign_id
	`)
	if err != nil {
		return nil, err
	}
	clicks, err := c.timelineEngagementByCampaign(subscriberRecordID, campaignIDs, `
		SELECT campaign_id, COUNT(*) AS event_count, MIN(created) AS first_at
		FROM link_clicks
		WHERE subscriber_id = ?
		  AND campaign_id IN (`+sqlitePlaceholders(len(campaignIDs))+`)
		GROUP BY campaign_id
	`)
	if err != nil {
		return nil, err
	}

	for _, r := range rows {
		campID := strings.TrimSpace(r.CampaignRecordID)
		open := opens[campID]
		click := clicks[campID]
		var firstOpenedAt *time.Time
		if open.FirstAtRaw.Valid && strings.TrimSpace(open.FirstAtRaw.String) != "" {
			if v, err := parseSQLiteDateTime(open.FirstAtRaw.String); err == nil {
				firstOpenedAt = &v
			}
		}
		var firstClickedAt *time.Time
		if click.FirstAtRaw.Valid && strings.TrimSpace(click.FirstAtRaw.String) != "" {
			if v, err := parseSQLiteDateTime(click.FirstAtRaw.String); err == nil {
				firstClickedAt = &v
			}
		}
		out[r.LedgerRowID] = timelineSendHydrated{
			CampaignRecordID: r.CampaignRecordID,
			CampaignName:     r.CampaignName,
			Status:           r.Status,
			Metadata: models.TimelineEventCampaignSendMetadata{
				CampaignID:     r.CampaignRowID,
				CampaignName:   strings.TrimSpace(r.CampaignName.String),
				CampaignUUID:   strings.TrimSpace(r.CampaignUUID.String),
				Subject:        strings.TrimSpace(r.CampaignSubject.String),
				MessageID:      campID,
				HasOpened:      open.EventCount > 0,
				HasClicked:     click.EventCount > 0,
				FirstOpenedAt:  firstOpenedAt,
				FirstClickedAt: firstClickedAt,
			},
		}
	}
	return out, nil
}

func (c *Core) timelineEngagementByCampaign(subscriberRecordID string, campaignIDs []string, query string) (map[string]timelineEngagementRow, error) {
	out := map[string]timelineEngagementRow{}
	if len(campaignIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, 1+len(campaignIDs))
	args = append(args, subscriberRecordID)
	for _, id := range campaignIDs {
		args = append(args, id)
	}
	var rows []timelineEngagementRow
	if err := c.db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.CampaignID] = r
	}
	return out, nil
}

func (c *Core) hydrateTimelineViews(subscriberRowID int, campaignIDs []string) (map[string]models.TimelineEventCampaignViewMetadata, error) {
	out := map[string]models.TimelineEventCampaignViewMetadata{}
	if len(campaignIDs) == 0 {
		return out, nil
	}
	args := []any{subscriberRowID}
	for _, id := range campaignIDs {
		args = append(args, id)
	}
	var rows []timelineViewHydrateRow
	if err := c.db.Select(&rows, `
		SELECT
			COALESCE(cv.campaign_id, '') AS campaign_record_id,
			COALESCE(c.rowid, 0) AS campaign_rowid,
			c.uuid AS campaign_uuid,
			c.name AS campaign_name,
			c.subject AS campaign_subject,
			COUNT(*) AS view_count
		FROM campaign_views cv
		LEFT JOIN campaigns c ON c.id = cv.campaign_id
		JOIN subscribers s ON s.id = cv.subscriber_id
		WHERE s.rowid = ?
		  AND COALESCE(cv.is_suspected_privacy_open, 0) = 0
		  AND COALESCE(cv.campaign_id, '') IN (`+sqlitePlaceholders(len(campaignIDs))+`)
		GROUP BY cv.campaign_id, c.rowid, c.uuid, c.name, c.subject
	`, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.CampaignRecordID] = models.TimelineEventCampaignViewMetadata{
			CampaignID:   r.CampaignRowID,
			CampaignName: strings.TrimSpace(r.CampaignName.String),
			CampaignUUID: strings.TrimSpace(r.CampaignUUID.String),
			Subject:      strings.TrimSpace(r.CampaignSubject.String),
			ViewCount:    r.ViewCount,
		}
	}
	return out, nil
}

type timelineClickHydrated struct {
	CampaignRecordID string
	Meta             models.TimelineEventLinkClickMetadata
}

func (c *Core) hydrateTimelineClicks(subscriberRowID int, clickKeys [][2]string) (map[string]timelineClickHydrated, error) {
	out := map[string]timelineClickHydrated{}
	if len(clickKeys) == 0 {
		return out, nil
	}

	var cond strings.Builder
	args := []any{subscriberRowID}
	for i, pair := range clickKeys {
		if i > 0 {
			cond.WriteString(" OR ")
		}
		cond.WriteString("(COALESCE(lc.campaign_id, '') = ? AND COALESCE(l.url, '') = ?)")
		args = append(args, pair[0], pair[1])
	}

	var rows []timelineClickHydrateRow
	if err := c.db.Select(&rows, `
		SELECT
			COALESCE(lc.campaign_id, '') AS campaign_record_id,
			COALESCE(c.rowid, 0) AS campaign_rowid,
			c.uuid AS campaign_uuid,
			c.name AS campaign_name,
			c.subject AS campaign_subject,
			l.url AS link_url,
			COUNT(*) AS click_count
		FROM link_clicks lc
		LEFT JOIN links l ON l.id = lc.link_id
		LEFT JOIN campaigns c ON c.id = lc.campaign_id
		JOIN subscribers s ON s.id = lc.subscriber_id
		WHERE s.rowid = ?
		  AND (`+cond.String()+`)
		GROUP BY lc.campaign_id, c.rowid, c.uuid, c.name, c.subject, l.url
	`, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		urlRaw := r.LinkURL.String
		out[r.CampaignRecordID+"\x00"+urlRaw] = timelineClickHydrated{
			CampaignRecordID: r.CampaignRecordID,
			Meta: models.TimelineEventLinkClickMetadata{
				CampaignID:   r.CampaignRowID,
				CampaignName: strings.TrimSpace(r.CampaignName.String),
				CampaignUUID: strings.TrimSpace(r.CampaignUUID.String),
				Subject:      strings.TrimSpace(r.CampaignSubject.String),
				URL:          strings.TrimSpace(urlRaw),
				ClickCount:   r.ClickCount,
			},
		}
	}
	return out, nil
}

func (c *Core) hydrateTimelineSMS(recordIDs []string) (map[string]models.InboundSMSEvent, error) {
	out := map[string]models.InboundSMSEvent{}
	if len(recordIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(recordIDs))
	for i, id := range recordIDs {
		args[i] = id
	}
	var rows []inboundSMSRow
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
		WHERE id IN (`+sqlitePlaceholders(len(recordIDs))+`)
	`, args...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		e, mapErr := mapInboundSMSRow(row)
		if mapErr != nil {
			continue
		}
		out[row.RecordID] = e
	}
	return out, nil
}

type timelineEmailHydrated struct {
	recordID string
	event    models.InboundEmailReplyEvent
}

func (c *Core) hydrateTimelineEmails(recordIDs []string) (map[string]timelineEmailHydrated, error) {
	out := map[string]timelineEmailHydrated{}
	if len(recordIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(recordIDs))
	for i, id := range recordIDs {
		args[i] = id
	}
	var rows []inboundEmailReplyRow
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
		WHERE id IN (`+sqlitePlaceholders(len(recordIDs))+`)
	`, args...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		e, mapErr := mapInboundEmailReplyRow(row)
		if mapErr != nil {
			continue
		}
		out[row.RecordID] = timelineEmailHydrated{recordID: row.RecordID, event: e}
	}
	return out, nil
}

func marshalTimelineMetadata(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}
