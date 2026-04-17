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
				Direction string   `json:"direction"`
				Text      string   `json:"text"`
				From      string   `json:"from"`
				To        []string `json:"to"`
				ID        string   `json:"id"`
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
		return c.NoContent(http.StatusOK)
	}
	if !quoIsStopKeyword(obj.Text) {
		return c.NoContent(http.StatusOK)
	}
	n, err := a.core.SMSOptOutSubscriberByPhone(from)
	if err != nil {
		a.log.Printf("quo webhook STOP: %v", err)
		return c.NoContent(http.StatusOK)
	}
	if n > 0 {
		a.log.Printf("quo webhook: SMS opt-out for phone %q (%d rows)", from, n)
	}
	return c.NoContent(http.StatusOK)
}

func quoIsStopKeyword(text string) bool {
	t := strings.TrimSpace(strings.ToUpper(text))
	switch t {
	case "STOP", "UNSUBSCRIBE", "CANCEL", "END", "QUIT":
		return true
	default:
		return false
	}
}
