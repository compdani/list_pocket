package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	stdmail "net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"
	"github.com/labstack/echo/v4"
)

const inboundEmailReplyWebhookSecretEnv = "LISTPOCKET_INBOUND_EMAIL_WEBHOOK_SECRET"

type inboundEmailReplyWebhookRequest struct {
	Provider       string           `json:"provider"`
	MessageID      string           `json:"message_id"`
	InReplyTo      string           `json:"in_reply_to"`
	References     string           `json:"references"`
	From           string           `json:"from"`
	Subject        string           `json:"subject"`
	Text           string           `json:"text"`
	HTML           string           `json:"html"`
	ReceivedAt     string           `json:"received_at"`
	Headers        map[string]any   `json:"headers"`
	Body           map[string]any   `json:"body"`
	Raw            map[string]any   `json:"raw"`
	HasAttachments *bool            `json:"has_attachments"`
	Attachments    []map[string]any `json:"attachments"`
}

type sesSNSNotification struct {
	Type    string `json:"Type"`
	Message string `json:"Message"`
}

type snsControlMessage struct {
	Type string `json:"Type"`
}

type sesInboundMailHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type sesInboundMessage struct {
	NotificationType string `json:"notificationType"`
	Mail             struct {
		Timestamp   string                 `json:"timestamp"`
		Source      string                 `json:"source"`
		MessageID   string                 `json:"messageId"`
		Destination []string               `json:"destination"`
		Headers     []sesInboundMailHeader `json:"headers"`
		Common      struct {
			From      []string `json:"from"`
			To        []string `json:"to"`
			Subject   string   `json:"subject"`
			MessageID string   `json:"messageId"`
			Date      string   `json:"date"`
		} `json:"commonHeaders"`
	} `json:"mail"`
	Content string `json:"content"`
}

type normalizedInboundEmail struct {
	Provider       string
	From           string
	MessageID      string
	InReplyTo      string
	References     string
	Subject        string
	ReceivedAt     time.Time
	Text           string
	HTML           string
	BodySnippet    string
	HasAttachments bool
	Headers        map[string]any
	RawBody        models.JSON
}

func (a *App) InboundEmailReplyWebhook(c echo.Context) error {
	id, err := a.processInboundEmailReplyWebhook(c)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{Data: map[string]string{"id": id}})
}

func (a *App) InboundEmailReplyWebhookPublic(c echo.Context) error {
	expected := strings.TrimSpace(os.Getenv(inboundEmailReplyWebhookSecretEnv))
	provided := strings.TrimSpace(c.Request().Header.Get("X-Listpocket-Webhook-Secret"))
	if expected == "" || provided == "" || expected != provided {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id, err := a.processInboundEmailReplyWebhook(c)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{Data: map[string]string{"id": id}})
}

func (a *App) processInboundEmailReplyWebhook(c echo.Context) (string, error) {
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 2<<20))
	if err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(body) >= 2<<20 {
		return "", echo.NewHTTPError(http.StatusRequestEntityTooLarge, "body too large")
	}

	if snsType, ok := parseSNSControlType(body); ok {
		a.log.Printf("inbound email webhook: SNS control message type=%q acknowledged", snsType)
		return "sns_control_" + strings.ToLower(snsType), nil
	}

	rawPayload := models.JSON{}
	if err := json.Unmarshal(body, &rawPayload); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid json")
	}

	var req inboundEmailReplyWebhookRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	normalized, ok := normalizeSESInboundEmail(body)
	if !ok {
		normalized = normalizeGenericInboundEmail(req)
	}
	headers := normalizeHeaderMap(normalized.Headers)
	provider := strings.TrimSpace(normalized.Provider)
	if provider == "" {
		provider = "inbound_email_webhook"
	}
	fromAddress := strings.TrimSpace(normalized.From)
	messageID := normalizeInboundMessageID(normalized.MessageID)
	inReplyTo := normalizeInboundMessageID(normalized.InReplyTo)
	references := strings.TrimSpace(normalized.References)
	subject := strings.TrimSpace(normalized.Subject)
	receivedAt := normalized.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	bodyText := strings.TrimSpace(normalized.Text)
	bodyHTML := strings.TrimSpace(normalized.HTML)
	bodySnippet := strings.TrimSpace(normalized.BodySnippet)
	if bodySnippet == "" {
		bodySnippet = makeInboundBodySnippet(bodyText, bodyHTML)
	}
	hasAttachments := normalized.HasAttachments
	if req.HasAttachments != nil {
		hasAttachments = *req.HasAttachments
	} else if !hasAttachments && len(req.Attachments) > 0 {
		hasAttachments = true
	}
	if messageID == "" {
		messageID = normalizeInboundMessageID(headerFirst(headers, "x-message-id"))
	}

	if fromAddress == "" {
		a.log.Printf("inbound email webhook: missing from address message_id=%q", messageID)
		return "", echo.NewHTTPError(http.StatusBadRequest, "from is required")
	}

	event := &models.InboundEmailReplyEvent{
		FromAddress:    strings.ToLower(strings.TrimSpace(fromAddress)),
		Subject:        strings.TrimSpace(subject),
		MessageID:      messageID,
		InReplyTo:      inReplyTo,
		References:     strings.TrimSpace(references),
		ReceivedAt:     receivedAt,
		BodySnippet:    bodySnippet,
		HasAttachments: hasAttachments,
		MatchScore:     "unmatched",
		RawHeaders:     models.JSON(headers),
		RawBody: models.JSON{
			"text": bodyText,
			"html": bodyHTML,
		},
		ProcessedAt: time.Now().UTC(),
	}
	for k, v := range normalized.RawBody {
		event.RawBody[k] = v
	}

	if strings.TrimSpace(toString(rawPayload["raw"])) == "" {
		event.RawBody["raw_payload"] = rawPayload
	}

	id, err := a.core.CreateInboundEmailReplyEvent(c.Request().Context(), event)
	if err != nil {
		a.log.Printf("inbound email webhook: persistence failed provider=%q message_id=%q from=%q err=%v", provider, messageID, fromAddress, err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, "failed to persist inbound email reply")
	}

	return id, nil
}

func parseInboundEmailReceivedAt(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

func normalizeHeaderMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out
}

func normalizeGenericInboundEmail(req inboundEmailReplyWebhookRequest) normalizedInboundEmail {
	headers := normalizeHeaderMap(req.Headers)
	fromAddress := firstNonEmpty(req.From, headerFirst(headers, "from"))
	messageID := normalizeInboundMessageID(firstNonEmpty(req.MessageID, headerFirst(headers, "message-id"), headerFirst(headers, "message_id")))
	inReplyTo := normalizeInboundMessageID(firstNonEmpty(req.InReplyTo, headerFirst(headers, "in-reply-to"), headerFirst(headers, "in_reply_to")))
	references := firstNonEmpty(req.References, headerFirst(headers, "references"))
	subject := firstNonEmpty(req.Subject, headerFirst(headers, "subject"))
	receivedAt := parseInboundEmailReceivedAt(firstNonEmpty(req.ReceivedAt, headerFirst(headers, "date")))
	bodyText := strings.TrimSpace(req.Text)
	if bodyText == "" {
		if v, ok := req.Body["text"]; ok {
			bodyText = strings.TrimSpace(toString(v))
		}
	}
	bodyHTML := strings.TrimSpace(req.HTML)
	if bodyHTML == "" {
		if v, ok := req.Body["html"]; ok {
			bodyHTML = strings.TrimSpace(toString(v))
		}
	}
	hasAttachments := false
	if req.HasAttachments != nil {
		hasAttachments = *req.HasAttachments
	} else if len(req.Attachments) > 0 {
		hasAttachments = true
	}
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = "inbound_email_webhook"
	}
	return normalizedInboundEmail{
		Provider:       provider,
		From:           fromAddress,
		MessageID:      messageID,
		InReplyTo:      inReplyTo,
		References:     references,
		Subject:        subject,
		ReceivedAt:     receivedAt,
		Text:           bodyText,
		HTML:           bodyHTML,
		BodySnippet:    makeInboundBodySnippet(bodyText, bodyHTML),
		HasAttachments: hasAttachments,
		Headers:        headers,
		RawBody:        models.JSON{},
	}
}

func normalizeSESInboundEmail(raw []byte) (normalizedInboundEmail, bool) {
	var sns sesSNSNotification
	msgBytes := raw
	if err := json.Unmarshal(raw, &sns); err == nil && strings.EqualFold(strings.TrimSpace(sns.Type), "Notification") && strings.TrimSpace(sns.Message) != "" {
		msgBytes = []byte(sns.Message)
	}

	var sesMsg sesInboundMessage
	if err := json.Unmarshal(msgBytes, &sesMsg); err != nil {
		return normalizedInboundEmail{}, false
	}
	if strings.TrimSpace(sesMsg.Content) == "" {
		return normalizedInboundEmail{}, false
	}

	rawMIME, err := decodeBase64Content(strings.TrimSpace(sesMsg.Content))
	if err != nil {
		return normalizedInboundEmail{}, false
	}

	parsed, err := parseRawMIMEInboundEmail(rawMIME)
	if err != nil {
		return normalizedInboundEmail{}, false
	}

	if parsed.MessageID == "" {
		parsed.MessageID = normalizeInboundMessageID(sesMsg.Mail.MessageID)
	}
	if parsed.Subject == "" {
		parsed.Subject = strings.TrimSpace(sesMsg.Mail.Common.Subject)
	}
	if parsed.From == "" && len(sesMsg.Mail.Common.From) > 0 {
		parsed.From = extractEmailAddress(sesMsg.Mail.Common.From[0])
	}
	if parsed.ReceivedAt.IsZero() {
		parsed.ReceivedAt = parseInboundEmailReceivedAt(sesMsg.Mail.Timestamp)
	}

	parsed.Provider = "ses"
	if parsed.Headers == nil {
		parsed.Headers = map[string]any{}
	}
	for _, h := range sesMsg.Mail.Headers {
		k := strings.ToLower(strings.TrimSpace(h.Name))
		if k == "" {
			continue
		}
		if _, ok := parsed.Headers[k]; !ok {
			parsed.Headers[k] = strings.TrimSpace(h.Value)
		}
	}
	parsed.RawBody["raw_mime_base64"] = sesMsg.Content
	parsed.RawBody["ses_message"] = map[string]any{
		"notificationType": sesMsg.NotificationType,
		"timestamp":        sesMsg.Mail.Timestamp,
		"source":           sesMsg.Mail.Source,
		"messageId":        sesMsg.Mail.MessageID,
	}

	return parsed, true
}

func parseRawMIMEInboundEmail(raw []byte) (normalizedInboundEmail, error) {
	if len(raw) == 0 {
		return normalizedInboundEmail{}, io.EOF
	}

	ent, err := message.Read(bytes.NewReader(raw))
	if err != nil {
		return normalizedInboundEmail{}, err
	}
	headers := map[string]any{}
	for k, vals := range ent.Header.Map() {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" || len(vals) == 0 {
			continue
		}
		headers[key] = strings.TrimSpace(vals[0])
	}

	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return normalizedInboundEmail{}, err
	}

	from := extractEmailAddress(headerFirst(headers, "from"))
	messageID := normalizeInboundMessageID(headerFirst(headers, "message-id"))
	inReplyTo := normalizeInboundMessageID(headerFirst(headers, "in-reply-to"))
	references := headerFirst(headers, "references")
	subject := headerFirst(headers, "subject")
	receivedAt := parseInboundEmailReceivedAt(headerFirst(headers, "date"))

	textBody := ""
	htmlBody := ""
	hasAttachments := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return normalizedInboundEmail{}, err
		}

		switch h := part.Header.(type) {
		case *gomail.InlineHeader:
			ct, _, _ := h.ContentType()
			b, _ := io.ReadAll(part.Body)
			content := strings.TrimSpace(string(b))
			if strings.HasPrefix(strings.ToLower(ct), "text/plain") && textBody == "" {
				textBody = content
			}
			if strings.HasPrefix(strings.ToLower(ct), "text/html") && htmlBody == "" {
				htmlBody = content
			}
		case *gomail.AttachmentHeader:
			hasAttachments = true
		}
	}

	if textBody == "" && htmlBody == "" {
		all, _ := io.ReadAll(ent.Body)
		textBody = strings.TrimSpace(string(all))
	}

	return normalizedInboundEmail{
		From:           from,
		MessageID:      messageID,
		InReplyTo:      inReplyTo,
		References:     references,
		Subject:        subject,
		ReceivedAt:     receivedAt,
		Text:           textBody,
		HTML:           htmlBody,
		BodySnippet:    makeInboundBodySnippet(textBody, htmlBody),
		HasAttachments: hasAttachments,
		Headers:        headers,
		RawBody: models.JSON{
			"raw_mime": string(raw),
		},
	}, nil
}

func decodeBase64Content(raw string) ([]byte, error) {
	if raw == "" {
		return nil, io.EOF
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err == nil {
		return b, nil
	}
	b, err = base64.RawStdEncoding.DecodeString(raw)
	if err == nil {
		return b, nil
	}
	return nil, err
}

func extractEmailAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if addr, err := stdmail.ParseAddress(raw); err == nil {
		return strings.ToLower(strings.TrimSpace(addr.Address))
	}
	if addrs, err := stdmail.ParseAddressList(raw); err == nil && len(addrs) > 0 {
		return strings.ToLower(strings.TrimSpace(addrs[0].Address))
	}
	if at := strings.Index(raw, "@"); at > 0 {
		return strings.ToLower(strings.TrimSpace(strings.Trim(raw, "<>\"'")))
	}
	return strings.ToLower(strings.TrimSpace(raw))
}

func headerFirst(headers map[string]any, key string) string {
	if headers == nil {
		return ""
	}
	v, ok := headers[strings.ToLower(strings.TrimSpace(key))]
	if !ok {
		return ""
	}
	return strings.TrimSpace(toString(v))
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func normalizeInboundMessageID(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "<>")
	return strings.TrimSpace(raw)
}

func makeInboundBodySnippet(textBody, htmlBody string) string {
	snippet := strings.TrimSpace(textBody)
	if snippet == "" {
		snippet = stripHTMLTags(strings.TrimSpace(htmlBody))
	}
	snippet = strings.Join(strings.Fields(snippet), " ")
	if snippet == "" {
		return ""
	}
	runes := []rune(snippet)
	if len(runes) > 200 {
		return string(runes[:200])
	}
	return snippet
}

var htmlTagsRE = regexp.MustCompile("<[^>]+>")

func stripHTMLTags(s string) string {
	if s == "" {
		return ""
	}
	return htmlTagsRE.ReplaceAllString(s, " ")
}

func parseSNSControlType(raw []byte) (string, bool) {
	var ctrl snsControlMessage
	if err := json.Unmarshal(raw, &ctrl); err != nil {
		return "", false
	}
	t := strings.TrimSpace(ctrl.Type)
	if strings.EqualFold(t, "SubscriptionConfirmation") || strings.EqualFold(t, "UnsubscribeConfirmation") {
		return t, true
	}
	return "", false
}
