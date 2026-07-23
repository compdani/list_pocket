package main

import (
	pbcore "github.com/pocketbase/pocketbase/core"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/messenger/quo"
	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
)

func loadTextMessagingSettingsFromPB(pb *pocketbase.PocketBase) models.TextMessagingSettings {
	if pb == nil {
		return models.DefaultTextMessagingSettings()
	}
	rec, err := pb.FindFirstRecordByFilter("listpocket_settings", "type={:type}", dbx.Params{"type": models.ListPocketSettingsTypeTextMessaging})
	if err != nil || rec == nil {
		return models.DefaultTextMessagingSettings()
	}
	return models.ParseTextMessagingSettings(rec.GetString("value"))
}

func (a *App) loadTextMessagingSettings() models.TextMessagingSettings {
	if a == nil {
		return models.DefaultTextMessagingSettings()
	}
	return loadTextMessagingSettingsFromPB(a.pb)
}

// ensureQuoMessengerRegistered registers the Quo messenger at runtime when settings are saved enabled,
// so campaigns can send without a full process restart.
func (a *App) ensureQuoMessengerRegistered() {
	if a == nil || a.manager == nil || a.pb == nil {
		return
	}
	s := loadTextMessagingSettingsFromPB(a.pb)
	p := s.QuoProvider()
	if p == nil || !p.Enabled || strings.TrimSpace(p.APIKey) == "" {
		return
	}
	if a.manager.HasMessenger(models.CampaignMessengerQuo) {
		return
	}
	qm := quo.NewMessenger(func() models.TextMessagingSettings {
		return loadTextMessagingSettingsFromPB(a.pb)
	})
	if err := a.manager.AddMessenger(qm); err != nil {
		a.log.Printf("register quo messenger: %v", err)
		return
	}
	a.messengers = append(a.messengers, qm)
}

func (a *App) GetTextMessagingSettings(re *pbcore.RequestEvent) error {
	s := a.loadTextMessagingSettings()
	root := strings.TrimSpace(a.urlCfg.RootURL)
	if root == "" && a.cfg != nil {
		root = strings.TrimSpace(a.cfg.SiteName)
	}
	return okJSON(re, s.ToResponse(root))
}

func (a *App) UpdateTextMessagingSettings(re *pbcore.RequestEvent) error {
	var in models.TextMessagingSettings
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	cur := a.loadTextMessagingSettings()
	merged := models.MergeTextMessagingUpdate(cur, in)
	if err := validateTextMessagingSettings(merged); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to encode text messaging settings")
	}
	if err := a.upsertTypedSettingsRecord(models.ListPocketSettingsTypeTextMessaging, string(b)); err != nil {
		return err
	}
	a.ensureQuoMessengerRegistered()
	root := strings.TrimSpace(a.urlCfg.RootURL)
	return okJSON(re, merged.ToResponse(root))
}

func validateTextMessagingSettings(s models.TextMessagingSettings) error {
	if s.SendLimits.MaxMessagesPerSecond < 1 || s.SendLimits.MaxMessagesPerSecond > 500 {
		return errors.New("send_limits.max_messages_per_second must be between 1 and 500")
	}
	if s.SendLimits.SlidingEnabled {
		if s.SendLimits.SlidingWindowSeconds < 1 || s.SendLimits.SlidingWindowSeconds > 86400 {
			return errors.New("send_limits.sliding_window_seconds must be between 1 and 86400")
		}
		if s.SendLimits.SlidingMaxMessages < 1 {
			return errors.New("send_limits.sliding_max_messages must be at least 1")
		}
	}
	if s.SendLimits.MaxSendErrors < 1 {
		return errors.New("send_limits.max_send_errors must be at least 1")
	}
	for _, p := range s.Providers {
		if p.ID == "quo" && p.Enabled {
			if strings.TrimSpace(p.APIKey) == "" {
				return errors.New("quo provider requires api_key when enabled")
			}
			if strings.TrimSpace(p.From) == "" {
				return errors.New("quo provider requires from (E.164 or phone number id) when enabled")
			}
		}
	}
	return nil
}

type textMessagingTestRequest struct {
	To string `json:"to"`
}

func (a *App) TestTextMessagingSettings(re *pbcore.RequestEvent) error {
	var req textMessagingTestRequest
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	to := strings.TrimSpace(req.To)
	if to == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "to is required")
	}
	s := a.loadTextMessagingSettings()
	client, err := quo.NewClientFromSettings(s)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	body := []byte("ListPocket SMS test message.")
	if err := client.SendText(re.Request.Context(), to, body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return okJSON(re, map[string]bool{"sent": true})
}

// quoWebhookMessagePayload is data.object for message webhooks. Quo v3 uses "body"; v4 docs use "text".
type quoWebhookMessagePayload struct {
	ID              string          `json:"id"`
	Direction       string          `json:"direction"`
	Text            string          `json:"text"`
	Body            string          `json:"body"`
	Status          string          `json:"status"`
	From            string          `json:"from"`
	CreatedAt       string          `json:"createdAt"`
	CreatedAtLegacy string          `json:"created_at"`
	To              json.RawMessage `json:"to"`
}

func (m quoWebhookMessagePayload) mergedText() string {
	if s := strings.TrimSpace(m.Body); s != "" {
		return s
	}
	return strings.TrimSpace(m.Text)
}

func (m quoWebhookMessagePayload) receivedAt() time.Time {
	for _, raw := range []string{m.CreatedAt, m.CreatedAtLegacy} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return ts.UTC()
		}
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts.UTC()
		}
	}
	return time.Now().UTC()
}

type quoWebhookEventData struct {
	Object quoWebhookMessagePayload `json:"object"`
}

// quoParseMessageWebhookEvent returns the event type and inner message from Quo webhook JSON.
// Supports v4-style flat envelopes (type + data at root) and v3-style wraps where the event
// is nested under top-level "object" (object.type, object.data.object).
func quoParseMessageWebhookEvent(body []byte) (eventType string, msg quoWebhookMessagePayload, err error) {
	var root struct {
		Type   string              `json:"type"`
		Data   quoWebhookEventData `json:"data"`
		Object json.RawMessage     `json:"object"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return "", quoWebhookMessagePayload{}, err
	}
	data := root.Data
	eventType = strings.TrimSpace(root.Type)
	// v3 sends the event as a JSON object under "object"; v4 sends `"object":"event"` (string).
	if len(root.Object) > 0 && root.Object[0] == '{' {
		var inner struct {
			Type string              `json:"type"`
			Data quoWebhookEventData `json:"data"`
		}
		if err := json.Unmarshal(root.Object, &inner); err == nil && strings.TrimSpace(inner.Type) != "" {
			eventType = strings.TrimSpace(inner.Type)
			data = inner.Data
		}
	}
	return eventType, data.Object, nil
}

// QuoMessageWebhook handles POST /webhooks/quo/:token for inbound message events (STOP, etc.).
func (a *App) QuoMessageWebhook(re *pbcore.RequestEvent) error {
	token := strings.TrimSpace(pathParam(re, "token"))
	if token == "" {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	s := a.loadTextMessagingSettings()
	if strings.TrimSpace(s.WebhookPath) == "" || s.WebhookPath != token {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	body, err := io.ReadAll(io.LimitReader(re.Request.Body, 1<<20))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(body) >= 1<<20 {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "body too large")
	}

	if sec := strings.TrimSpace(s.WebhookSigningSecret); sec != "" {
		sig := re.Request.Header.Get(quo.OpenPhoneSignatureHeader())
		if err := quo.VerifyWebhookSignature(sec, sig, body, 10*time.Minute); err != nil {
			a.log.Printf("quo webhook: signature: %v", err)
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
	}

	eventType, obj, err := quoParseMessageWebhookEvent(body)
	if err != nil {
		a.log.Printf("quo webhook: invalid json: %v", err)
		return re.NoContent(http.StatusOK)
	}
	if eventType != "message.received" {
		return re.NoContent(http.StatusOK)
	}
	if strings.ToLower(strings.TrimSpace(obj.Direction)) != "incoming" {
		return re.NoContent(http.StatusOK)
	}
	textBody := obj.mergedText()
	if strings.TrimSpace(textBody) == "" && strings.TrimSpace(obj.ID) != "" {
		if cl, err := quo.NewClientFromSettings(s); err == nil {
			ctx, cancel := context.WithTimeout(re.Request.Context(), 8*time.Second)
			t, err := cl.GetMessageText(ctx, obj.ID)
			cancel()
			if err == nil {
				textBody = t
			}
		}
	}
	from := strings.TrimSpace(obj.From)
	if from == "" {
		a.log.Printf("quo webhook: message.received with empty from; id=%q status=%q text=%q", obj.ID, obj.Status, quoTrimForLog(textBody))
		return re.NoContent(http.StatusOK)
	}

	isStop := quoIsStopKeyword(textBody)
	rawPayload := models.JSON{}
	if err := json.Unmarshal(body, &rawPayload); err != nil {
		a.log.Printf("quo webhook: raw payload decode warning: %v", err)
	}
	if _, err := a.core.CreateInboundSMSEvent(re.Request.Context(), &models.InboundSMSEvent{
		PhoneNumber:   from,
		ProviderID:    models.CampaignMessengerQuo,
		ProviderMsgID: strings.TrimSpace(obj.ID),
		FromNumber:    from,
		MessageBody:   textBody,
		ReceivedAt:    obj.receivedAt(),
		IsStopKeyword: isStop,
		MatchScore:    "unmatched",
		RawPayload:    rawPayload,
	}); err != nil {
		a.log.Printf("quo webhook: inbound SMS persistence failed from=%q id=%q err=%v", from, obj.ID, err)
	}

	if !isStop {
		// Log every inbound so operators can audit why STOPs sometimes don't opt
		// people out (e.g. the reply was "stop texting me please" but our keyword
		// matcher didn't recognize it for some reason).
		a.log.Printf("quo webhook: inbound message (no stop keyword) from=%q id=%q status=%q text=%q", from, obj.ID, obj.Status, quoTrimForLog(textBody))
		return re.NoContent(http.StatusOK)
	}
	n, err := a.core.SMSOptOutSubscriberByPhone(from)
	if err != nil {
		a.log.Printf("quo webhook STOP: from=%q status=%q err=%v", from, obj.Status, err)
		return re.NoContent(http.StatusOK)
	}
	if n > 0 {
		a.log.Printf("quo webhook: SMS opt-out for phone %q (%d rows) status=%q text=%q", from, n, obj.Status, quoTrimForLog(textBody))
	} else {
		// Keyword matched but we couldn't find a subscriber with that phone. This
		// is almost always a phone-format mismatch — the subscriber was imported
		// with a local-format number and Quo sent E.164 (or vice-versa).
		a.log.Printf("quo webhook: STOP from %q matched keyword %q but no subscriber rows updated; status=%q check phone format in subscribers table", from, quoTrimForLog(textBody), obj.Status)
	}
	return re.NoContent(http.StatusOK)
}

// quoStopKeywordLeadWordLimit is how many leading whitespace-separated tokens we
// scan for opt-out keywords. Quo can send reactions / quoted replies where the
// body appends the full original message (which may contain words like STOP or
// END); only the subscriber's own lead words should trigger an opt-out.
const quoStopKeywordLeadWordLimit = 4

// quoStopKeywords is the canonical set of inbound STOP keywords we honor.
// These are the CTIA / carrier-recommended opt-out keywords (case-insensitive).
var quoStopKeywords = map[string]struct{}{
	"STOP":        {},
	"STOPALL":     {},
	"UNSUBSCRIBE": {},
	"CANCEL":      {},
	"END":         {},
	"QUIT":        {},
	"OPTOUT":      {},
	"OPT-OUT":     {},
	"REVOKE":      {},
}

// quoIsStopKeyword reports whether an inbound SMS body should be treated as an
// opt-out. We match the carrier-standard STOP keywords case-insensitively and
// tolerate common real-world noise: surrounding whitespace, punctuation (e.g.
// "STOP."), and trailing polite text (e.g. "Stop please", "STOP texting me").
//
// Only the first quoStopKeywordLeadWordLimit whitespace-separated tokens are
// considered, so quoted thread bodies (reactions, forwarded meeting text, etc.)
// cannot trigger an opt-out from a keyword buried later in the payload.
func quoIsStopKeyword(text string) bool {
	upper := strings.ToUpper(strings.TrimSpace(text))
	if upper == "" {
		return false
	}
	fields := strings.Fields(upper)
	if len(fields) > quoStopKeywordLeadWordLimit {
		fields = fields[:quoStopKeywordLeadWordLimit]
	}
	for _, raw := range fields {
		token := strings.Trim(raw, ".,!?;:\"'`()[]{}<>*~")
		if token == "" {
			continue
		}
		if _, ok := quoStopKeywords[token]; ok {
			return true
		}
	}
	return false
}

// quoTrimForLog returns a single-line, length-capped version of a user-supplied
// SMS body suitable for log output. It flattens newlines so multi-line replies
// don't break log parsing.
func quoTrimForLog(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 140 {
		return s[:140] + "…"
	}
	return s
}
