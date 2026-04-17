package core

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
)

type txMessageRow struct {
	ID              int    `db:"id"`
	RecordID        string `db:"record_id"`
	CreatedAt       string `db:"created_at"`
	UpdatedAt       string `db:"updated_at"`
	UUID            string `db:"uuid"`
	SubscriberID    string `db:"subscriber_record_id"`
	SubscriberEmail string `db:"subscriber_email"`
	TemplateID      string `db:"template_record_id"`
	TemplateName    string `db:"template_name"`
	FromEmail       string `db:"from_email"`
	Subject         string `db:"subject"`
	ContentType     string `db:"content_type"`
	Messenger       string `db:"messenger"`
	Status          string `db:"status"`
	Error           string `db:"error"`
	Body            string `db:"body"`
	Data            []byte `db:"data"`
	Headers         []byte `db:"headers"`
	Views           int    `db:"views"`
	RawViews        int    `db:"raw_views"`
	SuspectedViews  int    `db:"suspected_views"`
	Clicks          int    `db:"clicks"`
	SentAt          string `db:"sent_at"`
}

func txMessageRowToModel(row txMessageRow) models.TransactionalMessage {
	out := models.TransactionalMessage{
		Base: models.Base{
			ID:        row.ID,
			RecordID:  row.RecordID,
			CreatedAt: parseNullTime(row.CreatedAt),
			UpdatedAt: parseNullTime(row.UpdatedAt),
		},
		UUID:            row.UUID,
		SubscriberID:    row.SubscriberID,
		SubscriberEmail: row.SubscriberEmail,
		TemplateID:      row.TemplateID,
		TemplateName:    row.TemplateName,
		FromEmail:       row.FromEmail,
		Subject:         row.Subject,
		ContentType:     row.ContentType,
		Messenger:       row.Messenger,
		Status:          row.Status,
		Error:           row.Error,
		Body:            row.Body,
		Data:            models.JSON{},
		Headers:         models.JSON{},
		Views:           row.Views,
		RawViews:        row.RawViews,
		SuspectedViews:  row.SuspectedViews,
		Clicks:          row.Clicks,
		SentAt:          parseNullTime(row.SentAt),
	}

	if len(row.Data) > 0 && string(row.Data) != "null" {
		_ = json.Unmarshal(row.Data, &out.Data)
	}
	if len(row.Headers) > 0 && string(row.Headers) != "null" {
		_ = json.Unmarshal(row.Headers, &out.Headers)
	}

	return out
}

func (c *Core) QueryTransactionalMessages(search string, offset, limit int) ([]models.TransactionalMessage, int, error) {
	search = strings.TrimSpace(search)
	searchLike := "%"
	if search != "" {
		searchLike = "%" + search + "%"
	}

	total := 0
	if err := c.db.Get(&total, `
		SELECT COUNT(*)
		FROM transactional_messages tm
		LEFT JOIN templates t ON t.id = tm.template_id
		WHERE (? = '' OR tm.subject LIKE ? OR tm.to_email LIKE ? OR COALESCE(t.name, '') LIKE ?)
	`, search, searchLike, searchLike, searchLike); err != nil {
		c.log.Printf("error counting transactional messages: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("globals.messages.errorFetching", "name", "transactional messages", "error", err.Error()))
	}

	rows := []txMessageRow{}
	if err := c.db.Select(&rows, `
		SELECT
			tm.rowid AS id,
			tm.id AS record_id,
			tm.created AS created_at,
			tm.updated AS updated_at,
			tm.uuid,
			COALESCE(tm.subscriber_id, '') AS subscriber_record_id,
			tm.to_email AS subscriber_email,
			COALESCE(tm.template_id, '') AS template_record_id,
			COALESCE(t.name, '') AS template_name,
			tm.from_email,
			tm.subject,
			tm.content_type,
			tm.messenger,
			tm.status,
			COALESCE(tm.error, '') AS error,
			'' AS body,
			'{}' AS data,
			'{}' AS headers,
			(SELECT COUNT(*) FROM tx_message_views tv WHERE tv.tx_message_id = tm.id AND COALESCE(tv.is_suspected_privacy_open, 0) = 0) AS views,
			(SELECT COUNT(*) FROM tx_message_views tv WHERE tv.tx_message_id = tm.id) AS raw_views,
			(SELECT COUNT(*) FROM tx_message_views tv WHERE tv.tx_message_id = tm.id AND COALESCE(tv.is_suspected_privacy_open, 0) = 1) AS suspected_views,
			(SELECT COUNT(*) FROM tx_link_clicks tc WHERE tc.tx_message_id = tm.id) AS clicks,
			COALESCE(tm.sent_at, '') AS sent_at
		FROM transactional_messages tm
		LEFT JOIN templates t ON t.id = tm.template_id
		WHERE (? = '' OR tm.subject LIKE ? OR tm.to_email LIKE ? OR COALESCE(t.name, '') LIKE ?)
		ORDER BY tm.created DESC
		LIMIT ? OFFSET ?
	`, search, searchLike, searchLike, searchLike, limit, offset); err != nil {
		c.log.Printf("error querying transactional messages: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("globals.messages.errorFetching", "name", "transactional messages", "error", err.Error()))
	}

	out := make([]models.TransactionalMessage, 0, len(rows))
	for _, row := range rows {
		out = append(out, txMessageRowToModel(row))
	}
	return out, total, nil
}

func (c *Core) GetTransactionalMessage(recordID string) (models.TransactionalMessage, error) {
	recordID = strings.TrimSpace(recordID)
	if recordID == "" {
		return models.TransactionalMessage{}, echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
	}

	row := txMessageRow{}
	err := c.db.Get(&row, `
		SELECT
			tm.rowid AS id,
			tm.id AS record_id,
			tm.created AS created_at,
			tm.updated AS updated_at,
			tm.uuid,
			COALESCE(tm.subscriber_id, '') AS subscriber_record_id,
			tm.to_email AS subscriber_email,
			COALESCE(tm.template_id, '') AS template_record_id,
			COALESCE(t.name, '') AS template_name,
			tm.from_email,
			tm.subject,
			tm.content_type,
			tm.messenger,
			tm.status,
			COALESCE(tm.error, '') AS error,
			tm.body,
			COALESCE(tm.data, '{}') AS data,
			COALESCE(tm.headers, '{}') AS headers,
			(SELECT COUNT(*) FROM tx_message_views tv WHERE tv.tx_message_id = tm.id AND COALESCE(tv.is_suspected_privacy_open, 0) = 0) AS views,
			(SELECT COUNT(*) FROM tx_message_views tv WHERE tv.tx_message_id = tm.id) AS raw_views,
			(SELECT COUNT(*) FROM tx_message_views tv WHERE tv.tx_message_id = tm.id AND COALESCE(tv.is_suspected_privacy_open, 0) = 1) AS suspected_views,
			(SELECT COUNT(*) FROM tx_link_clicks tc WHERE tc.tx_message_id = tm.id) AS clicks,
			COALESCE(tm.sent_at, '') AS sent_at
		FROM transactional_messages tm
		LEFT JOIN templates t ON t.id = tm.template_id
		WHERE tm.id = ?
		LIMIT 1
	`, recordID)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.TransactionalMessage{}, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.notFound", "name", "transactional message"))
		}
		c.log.Printf("error fetching transactional message: %v", err)
		return models.TransactionalMessage{}, echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("globals.messages.errorFetching", "name", "transactional message", "error", err.Error()))
	}

	out := txMessageRowToModel(row)
	_ = c.db.Select(&out.LinkStats, `
		SELECT l.url, COUNT(*) AS count
		FROM tx_link_clicks tc
		INNER JOIN links l ON l.id = tc.link_id
		WHERE tc.tx_message_id = ?
		GROUP BY l.url
		ORDER BY count DESC, l.url ASC
	`, recordID)

	return out, nil
}

func (c *Core) RegisterTransactionalMessageView(msgUUID string, event models.OpenEvent) error {
	event = normalizeOpenEvent(event)

	var row struct {
		MessageID    string         `db:"message_id"`
		SubscriberID sql.NullString `db:"subscriber_id"`
		SentAt       string         `db:"sent_at"`
	}
	if err := c.db.Get(&row, `
		SELECT tm.id AS message_id, tm.subscriber_id, COALESCE(tm.sent_at, '') AS sent_at
		FROM transactional_messages tm
		WHERE tm.uuid = ?
		LIMIT 1
	`, msgUUID); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		c.log.Printf("error resolving transactional message view target: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}

	sentAt := time.Time{}
	if parsed := parseNullTime(row.SentAt); parsed.Valid {
		sentAt = parsed.Time
	}
	suspected, meta, err := classifyPrivacyOpen(event, sentAt, "transactional_sent_at")
	if err != nil {
		c.log.Printf("error marshaling transactional view metadata: %v", err)
		meta = "{}"
	}

	if _, err := c.db.Exec(`
		INSERT INTO tx_message_views (tx_message_id, subscriber_id, meta, is_suspected_privacy_open, created)
		VALUES (?, ?, ?, ?, ?)
	`, row.MessageID, row.SubscriberID, meta, suspected, sqliteTimestampValue(event.OpenedAt)); err != nil {
		c.log.Printf("error registering transactional message view: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}
	return nil
}

func (c *Core) RegisterTransactionalLinkClick(linkUUID, msgUUID string) (string, error) {
	var out struct {
		ID  string `db:"id"`
		URL string `db:"url"`
	}

	if err := c.db.Get(&out, `SELECT id, url FROM links WHERE id = ? OR uuid = ?`, linkUUID, linkUUID); err != nil {
		if err == sql.ErrNoRows {
			return "", echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("public.invalidLink"))
		}
		c.log.Printf("error getting transactional link URL: %v", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}

	if _, err := c.db.Exec(`
		INSERT INTO tx_link_clicks (tx_message_id, subscriber_id, link_id)
		SELECT tm.id, tm.subscriber_id, ?
		FROM transactional_messages tm
		WHERE tm.uuid = ?
		LIMIT 1
	`, out.ID, msgUUID); err != nil {
		c.log.Printf("error registering transactional link click: %v", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}

	return out.URL, nil
}
