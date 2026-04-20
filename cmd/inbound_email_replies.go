package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
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
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
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
	Attachments    []inboundEmailAttachment
	Headers        map[string]any
	RawBody        models.JSON
}

type inboundEmailAttachment struct {
	Filename    string
	ContentType string
	Disposition string
	ContentID   string
	Content     []byte
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
		a.log.Printf("inbound email webhook public: unauthorized remote=%q ua=%q expected_set=%t provided_set=%t", c.RealIP(), c.Request().UserAgent(), expected != "", provided != "")
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	a.log.Printf("inbound email webhook public: authorized remote=%q ua=%q", c.RealIP(), c.Request().UserAgent())
	id, err := a.processInboundEmailReplyWebhook(c)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{Data: map[string]string{"id": id}})
}

func (a *App) processInboundEmailReplyWebhook(c echo.Context) (string, error) {
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 2<<20))
	if err != nil {
		a.log.Printf("inbound email webhook: read body failed remote=%q ua=%q err=%v", c.RealIP(), c.Request().UserAgent(), err)
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(body) >= 2<<20 {
		a.log.Printf("inbound email webhook: body too large remote=%q ua=%q size=%d", c.RealIP(), c.Request().UserAgent(), len(body))
		return "", echo.NewHTTPError(http.StatusRequestEntityTooLarge, "body too large")
	}
	return a.processInboundEmailReplyWebhookBody(c, body)
}

func (a *App) processInboundEmailReplyWebhookBody(c echo.Context, body []byte) (string, error) {
	a.logInboundEmailWebhookRequest(c, body)

	if snsType, ok := parseSNSControlType(body); ok {
		a.log.Printf("inbound email webhook: SNS control message type=%q acknowledged", snsType)
		return "sns_control_" + strings.ToLower(snsType), nil
	}

	rawPayload := models.JSON{}
	if err := json.Unmarshal(body, &rawPayload); err != nil {
		a.log.Printf("inbound email webhook: invalid json remote=%q ua=%q err=%v body=%s", c.RealIP(), c.Request().UserAgent(), err, string(body))
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid json")
	}

	var req inboundEmailReplyWebhookRequest
	if err := json.Unmarshal(body, &req); err != nil {
		a.log.Printf("inbound email webhook: invalid payload shape remote=%q ua=%q err=%v body=%s", c.RealIP(), c.Request().UserAgent(), err, string(body))
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
	maps.Copy(event.RawBody, normalized.RawBody)
	if len(req.Attachments) > 0 {
		event.RawBody["attachments"] = req.Attachments
	}

	if strings.TrimSpace(toString(rawPayload["raw"])) == "" {
		event.RawBody["raw_payload"] = rawPayload
	}

	id, err := a.core.CreateInboundEmailReplyEvent(c.Request().Context(), event)
	if err != nil {
		a.log.Printf("inbound email webhook: persistence failed provider=%q message_id=%q from=%q err=%v", provider, messageID, fromAddress, err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, "failed to persist inbound email reply")
	}
	if len(normalized.Attachments) > 0 {
		pb := c.Get("app").(*pocketbase.PocketBase)
		attachments, attachmentErrors := a.saveInboundEmailAttachments(pb, id, normalized.Attachments)
		if len(attachments) > 0 || len(attachmentErrors) > 0 {
			if updateErr := a.updateInboundEmailReplyAttachmentMetadata(pb, id, attachments, attachmentErrors); updateErr != nil {
				a.log.Printf("inbound email webhook: attachment metadata update failed record_id=%q message_id=%q err=%v", id, messageID, updateErr)
			}
		}
	}
	a.log.Printf("inbound email webhook: persisted id=%q provider=%q message_id=%q from=%q", id, provider, messageID, fromAddress)

	return id, nil
}

func (a *App) logInboundEmailWebhookRequest(c echo.Context, body []byte) {
	headers := c.Request().Header
	snsType := strings.TrimSpace(headers.Get("x-amz-sns-message-type"))
	snsTopic := strings.TrimSpace(headers.Get("x-amz-sns-topic-arn"))
	snsMsgID := strings.TrimSpace(headers.Get("x-amz-sns-message-id"))
	a.log.Printf(
		"inbound email webhook: received method=%q path=%q remote=%q ua=%q content_type=%q size=%d sns_type=%q sns_topic=%q sns_message_id=%q body=%s",
		c.Request().Method,
		c.Path(),
		c.RealIP(),
		c.Request().UserAgent(),
		c.Request().Header.Get(echo.HeaderContentType),
		len(body),
		snsType,
		snsTopic,
		snsMsgID,
		string(body),
	)
}

func (a *App) saveInboundEmailAttachments(pb *pocketbase.PocketBase, inboundEmailReplyID string, attachments []inboundEmailAttachment) ([]map[string]any, []map[string]any) {
	out := make([]map[string]any, 0, len(attachments))
	errs := make([]map[string]any, 0)
	for _, attachment := range attachments {
		rec, err := a.saveInboundEmailAttachment(pb, inboundEmailReplyID, attachment)
		if err != nil {
			errs = append(errs, map[string]any{
				"filename": strings.TrimSpace(attachment.Filename),
				"error":    err.Error(),
			})
			continue
		}
		storedName := ""
		if raw := rec.Get("file"); raw != nil {
			switch v := raw.(type) {
			case []string:
				if len(v) > 0 {
					storedName = strings.TrimSpace(v[0])
				}
			case []any:
				if len(v) > 0 {
					storedName = strings.TrimSpace(toString(v[0]))
				}
			case string:
				storedName = strings.TrimSpace(v)
			}
		}
		out = append(out, map[string]any{
			"attachment_record_id": rec.Id,
			"filename":             strings.TrimSpace(attachment.Filename),
			"content_type":         strings.TrimSpace(attachment.ContentType),
			"size_bytes":           len(attachment.Content),
			"content_id":           strings.TrimSpace(attachment.ContentID),
			"disposition":          strings.TrimSpace(attachment.Disposition),
			"stored_name":          storedName,
			"download_url":         fmt.Sprintf("/mailapi/inbound-email-attachments/%s/download", rec.Id),
		})
	}
	return out, errs
}

func (a *App) saveInboundEmailAttachment(pb *pocketbase.PocketBase, inboundEmailReplyID string, attachment inboundEmailAttachment) (*pbcore.Record, error) {
	collection, err := pb.FindCollectionByNameOrId("inbound_email_attachments")
	if err != nil {
		return nil, err
	}

	fileName := strings.TrimSpace(attachment.Filename)
	if fileName == "" {
		fileName = "attachment.bin"
	}

	f, err := filesystem.NewFileFromBytes(attachment.Content, fileName)
	if err != nil {
		return nil, err
	}

	rec := pbcore.NewRecord(collection)
	rec.Set("inbound_email_reply_id", inboundEmailReplyID)
	rec.Set("file", f)
	rec.Set("original_name", fileName)
	rec.Set("content_type", strings.TrimSpace(attachment.ContentType))
	rec.Set("content_id", strings.TrimSpace(attachment.ContentID))
	rec.Set("disposition", strings.TrimSpace(attachment.Disposition))
	rec.Set("size_bytes", len(attachment.Content))

	if err := pb.Save(rec); err != nil {
		return nil, err
	}

	return rec, nil
}

func (a *App) updateInboundEmailReplyAttachmentMetadata(pb *pocketbase.PocketBase, recordID string, attachments []map[string]any, attachmentErrors []map[string]any) error {
	rec, err := pb.FindRecordById("inbound_email_replies", recordID)
	if err != nil {
		return err
	}

	rawBody := map[string]any{}
	if raw := rec.Get("raw_body"); raw != nil {
		switch v := raw.(type) {
		case map[string]any:
			maps.Copy(rawBody, v)
		case models.JSON:
			maps.Copy(rawBody, map[string]any(v))
		case string:
			_ = json.Unmarshal([]byte(v), &rawBody)
		case []byte:
			_ = json.Unmarshal(v, &rawBody)
		}
	}

	if len(attachments) > 0 {
		rawBody["attachments"] = attachments
	}
	if len(attachmentErrors) > 0 {
		rawBody["attachment_errors"] = attachmentErrors
	}

	rec.Set("raw_body", rawBody)
	return pb.Save(rec)
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
	attachments := make([]inboundEmailAttachment, 0)
	attachmentMetadata := make([]map[string]any, 0)
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
			ct, _, _ := h.ContentType()
			filename, _ := h.Filename()
			disposition, _, _ := h.ContentDisposition()
			contentID := strings.TrimSpace(h.Get("Content-Id"))
			b, _ := io.ReadAll(part.Body)
			filename = strings.TrimSpace(filename)
			attachments = append(attachments, inboundEmailAttachment{
				Filename:    filename,
				ContentType: strings.TrimSpace(ct),
				Disposition: strings.TrimSpace(disposition),
				ContentID:   strings.Trim(contentID, "<>() \t\r\n"),
				Content:     b,
			})
			attachmentMetadata = append(attachmentMetadata, map[string]any{
				"filename":     filename,
				"content_type": strings.TrimSpace(ct),
				"size_bytes":   len(b),
				"content_id":   strings.Trim(contentID, "<>() \t\r\n"),
				"disposition":  strings.TrimSpace(disposition),
			})
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
		Attachments:    attachments,
		Headers:        headers,
		RawBody: models.JSON{
			"raw_mime":    string(raw),
			"attachments": attachmentMetadata,
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

func parseSESNotificationType(raw []byte) string {
	var sns sesSNSNotification
	msgBytes := raw
	if err := json.Unmarshal(raw, &sns); err == nil && strings.TrimSpace(sns.Message) != "" {
		msgBytes = []byte(sns.Message)
	}

	var sesMsg sesInboundMessage
	if err := json.Unmarshal(msgBytes, &sesMsg); err != nil {
		return ""
	}
	return strings.TrimSpace(sesMsg.NotificationType)
}

// GetInboundEmailAttachments lists all attachments for a specific inbound email reply.
// Handler: GET /mailapi/inbound-email-replies/{replyId}/attachments
func (a *App) GetInboundEmailAttachments(c echo.Context) error {
	replyID := strings.TrimSpace(c.Param("replyId"))
	if replyID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing replyId")
	}

	pb := c.Get("app").(*pocketbase.PocketBase)
	collection, err := pb.FindCollectionByNameOrId("inbound_email_attachments")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "attachment collection not found")
	}

	records, err := pb.FindRecordsByFilter(collection.Id, fmt.Sprintf(`inbound_email_reply_id = "%s"`, strings.ReplaceAll(replyID, `"`, ``)), "-created", 0, 200)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list attachments")
	}

	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		fileName := firstFileName(record.Get("file"))
		items = append(items, map[string]any{
			"id":            record.Id,
			"reply_id":      replyID,
			"original_name": strings.TrimSpace(toString(record.Get("original_name"))),
			"content_type":  strings.TrimSpace(toString(record.Get("content_type"))),
			"size_bytes":    record.Get("size_bytes"),
			"file_name":     fileName,
			"download_url":  fmt.Sprintf("/mailapi/inbound-email-attachments/%s/download", record.Id),
			"created":       record.Get("created"),
		})
	}

	return c.JSON(http.StatusOK, okResp{Data: items})
}

// DownloadInboundEmailAttachment redirects to PocketBase file download endpoint.
// Handler: GET /mailapi/inbound-email-attachments/{id}/download
func (a *App) DownloadInboundEmailAttachment(c echo.Context) error {
	attachmentID := strings.TrimSpace(c.Param("id"))
	if attachmentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing attachment id")
	}

	pb := c.Get("app").(*pocketbase.PocketBase)
	collection, err := pb.FindCollectionByNameOrId("inbound_email_attachments")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "attachment collection not found")
	}

	record, err := pb.FindRecordById(collection.Id, attachmentID)
	if err != nil || record == nil {
		return echo.NewHTTPError(http.StatusNotFound, "attachment not found")
	}

	fileName := firstFileName(record.Get("file"))
	if fileName == "" {
		return echo.NewHTTPError(http.StatusNotFound, "attachment file missing")
	}

	return c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("/api/files/%s/%s/%s?download=1", collection.Id, record.Id, fileName))
}

func firstFileName(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case []string:
		if len(t) > 0 {
			return strings.TrimSpace(t[0])
		}
	case []any:
		if len(t) > 0 {
			return strings.TrimSpace(toString(t[0]))
		}
	}
	return ""
}
