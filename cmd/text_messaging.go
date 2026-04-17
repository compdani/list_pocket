package main

import (
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

func (a *App) GetTextMessagingSettings(c echo.Context) error {
	s := a.loadTextMessagingSettings()
	root := strings.TrimSpace(a.urlCfg.RootURL)
	if root == "" && a.cfg != nil {
		root = strings.TrimSpace(a.cfg.SiteName)
	}
	return c.JSON(http.StatusOK, okResp{s.ToResponse(root)})
}

func (a *App) UpdateTextMessagingSettings(c echo.Context) error {
	var in models.TextMessagingSettings
	if err := c.Bind(&in); err != nil {
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
	return c.JSON(http.StatusOK, okResp{merged.ToResponse(root)})
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

func (a *App) TestTextMessagingSettings(c echo.Context) error {
	var req textMessagingTestRequest
	if err := c.Bind(&req); err != nil {
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
	if err := client.SendText(c.Request().Context(), to, body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, okResp{Data: map[string]bool{"sent": true}})
}

// QuoMessageWebhook handles POST /webhooks/quo/:token for inbound message events (STOP, etc.).
func (a *App) QuoMessageWebhook(c echo.Context) error {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	s := a.loadTextMessagingSettings()
	if strings.TrimSpace(s.WebhookPath) == "" || s.WebhookPath != token {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 1<<20))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(body) >= 1<<20 {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "body too large")
	}

	if sec := strings.TrimSpace(s.WebhookSigningSecret); sec != "" {
		sig := c.Request().Header.Get(quo.OpenPhoneSignatureHeader())
		if err := quo.VerifyWebhookSignature(sec, sig, body, 10*time.Minute); err != nil {
			a.log.Printf("quo webhook: signature: %v", err)
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
	}

	var envelope struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				Direction string          `json:"direction"`
				Text      string          `json:"text"`
				From      string          `json:"from"`
				To        json.RawMessage `json:"to"` // Quo sends string for incoming, []string for outgoing.
				ID        string          `json:"id"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		a.log.Printf("quo webhook: invalid json: %v", err)
		return c.NoContent(http.StatusOK)
	}
	if envelope.Type != "message.received" {
		return c.NoContent(http.StatusOK)
	}
	obj := envelope.Data.Object
	if strings.ToLower(strings.TrimSpace(obj.Direction)) != "incoming" {
		return c.NoContent(http.StatusOK)
	}
	from := strings.TrimSpace(obj.From)
	if from == "" {
		a.log.Printf("quo webhook: message.received with empty from; id=%q text=%q", obj.ID, quoTrimForLog(obj.Text))
		return c.NoContent(http.StatusOK)
	}
	if !quoIsStopKeyword(obj.Text) {
		// Log every inbound so operators can audit why STOPs sometimes don't opt
		// people out (e.g. the reply was "stop texting me please" but our keyword
		// matcher didn't recognize it for some reason).
		a.log.Printf("quo webhook: inbound message (no stop keyword) from=%q id=%q text=%q", from, obj.ID, quoTrimForLog(obj.Text))
		return c.NoContent(http.StatusOK)
	}
	n, err := a.core.SMSOptOutSubscriberByPhone(from)
	if err != nil {
		a.log.Printf("quo webhook STOP: from=%q err=%v", from, err)
		return c.NoContent(http.StatusOK)
	}
	if n > 0 {
		a.log.Printf("quo webhook: SMS opt-out for phone %q (%d rows) text=%q", from, n, quoTrimForLog(obj.Text))
	} else {
		// Keyword matched but we couldn't find a subscriber with that phone. This
		// is almost always a phone-format mismatch — the subscriber was imported
		// with a local-format number and Quo sent E.164 (or vice-versa).
		a.log.Printf("quo webhook: STOP from %q matched keyword %q but no subscriber rows updated; check phone format in subscribers table", from, quoTrimForLog(obj.Text))
	}
	return c.NoContent(http.StatusOK)
}

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
// Specifically, if any whitespace-separated token in the body (after stripping
// surrounding punctuation) equals a stop keyword, we treat it as opt-out.
//
// This is intentionally permissive to honor user opt-out intent and stay on
// the right side of TCPA/10DLC compliance — it's far worse to keep texting
// someone who said "stop" than to occasionally opt out a word like "endgame"
// if a subscriber actually writes that (they won't).
func quoIsStopKeyword(text string) bool {
	upper := strings.ToUpper(strings.TrimSpace(text))
	if upper == "" {
		return false
	}
	for _, raw := range strings.Fields(upper) {
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
