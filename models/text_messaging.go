package models

import (
	"encoding/json"
	"strings"
	"time"
)

const ListPocketSettingsTypeTextMessaging = "text_messaging"

// TextMessagingSettings is stored in listpocket_settings (type=text_messaging) as JSON.
type TextMessagingSettings struct {
	Providers            []TextMessagingProvider `json:"providers"`
	SendLimits           TextMessagingSendLimits `json:"send_limits"`
	WebhookPath          string                  `json:"webhook_path,omitempty"`
	WebhookSigningSecret string                  `json:"webhook_signing_secret,omitempty"`
}

type TextMessagingProvider struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key,omitempty"`
	From    string `json:"from,omitempty"`
}

type TextMessagingSendLimits struct {
	MaxMessagesPerSecond int  `json:"max_messages_per_second"`
	SlidingEnabled       bool `json:"sliding_window_enabled"`
	SlidingWindowSeconds int  `json:"sliding_window_seconds"`
	SlidingMaxMessages   int  `json:"sliding_max_messages"`
	MinDelayMS           int  `json:"min_delay_ms"`
	MaxSendErrors        int  `json:"max_send_errors"`
}

func DefaultTextMessagingSettings() TextMessagingSettings {
	return TextMessagingSettings{
		Providers: []TextMessagingProvider{
			{ID: "quo", Enabled: false, APIKey: "", From: ""},
		},
		SendLimits: TextMessagingSendLimits{
			MaxMessagesPerSecond: 5,
			SlidingEnabled:       false,
			SlidingWindowSeconds: 60,
			SlidingMaxMessages:   60,
			MinDelayMS:           0,
			MaxSendErrors:        5,
		},
		WebhookPath: "",
	}
}

func ParseTextMessagingSettings(raw string) TextMessagingSettings {
	out := DefaultTextMessagingSettings()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	var parsed TextMessagingSettings
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return out
	}
	if len(parsed.Providers) > 0 {
		out.Providers = parsed.Providers
	} else {
		out.Providers = DefaultTextMessagingSettings().Providers
	}
	if parsed.SendLimits.MaxMessagesPerSecond > 0 {
		out.SendLimits.MaxMessagesPerSecond = parsed.SendLimits.MaxMessagesPerSecond
	}
	out.SendLimits.SlidingEnabled = parsed.SendLimits.SlidingEnabled
	if parsed.SendLimits.SlidingWindowSeconds > 0 {
		out.SendLimits.SlidingWindowSeconds = parsed.SendLimits.SlidingWindowSeconds
	}
	if parsed.SendLimits.SlidingMaxMessages > 0 {
		out.SendLimits.SlidingMaxMessages = parsed.SendLimits.SlidingMaxMessages
	}
	if parsed.SendLimits.MinDelayMS >= 0 {
		out.SendLimits.MinDelayMS = parsed.SendLimits.MinDelayMS
	}
	if parsed.SendLimits.MaxSendErrors > 0 {
		out.SendLimits.MaxSendErrors = parsed.SendLimits.MaxSendErrors
	}
	out.WebhookPath = strings.TrimSpace(parsed.WebhookPath)
	out.WebhookSigningSecret = strings.TrimSpace(parsed.WebhookSigningSecret)
	return out
}

// TextMessagingSettingsResponse is safe for GET (masks API key).
type TextMessagingSettingsResponse struct {
	Providers               []TextMessagingProviderResponse `json:"providers"`
	SendLimits              TextMessagingSendLimits         `json:"send_limits"`
	WebhookPath             string                          `json:"webhook_path,omitempty"`
	WebhookURL              string                          `json:"webhook_url,omitempty"`
	HasWebhookSigningSecret bool                            `json:"has_webhook_signing_secret"`
}

type TextMessagingProviderResponse struct {
	ID        string `json:"id"`
	Enabled   bool   `json:"enabled"`
	HasAPIKey bool   `json:"has_api_key"`
	From      string `json:"from,omitempty"`
}

func (s TextMessagingSettings) ToResponse(rootURL string) TextMessagingSettingsResponse {
	provs := make([]TextMessagingProviderResponse, 0, len(s.Providers))
	for _, p := range s.Providers {
		provs = append(provs, TextMessagingProviderResponse{
			ID:        p.ID,
			Enabled:   p.Enabled,
			HasAPIKey: strings.TrimSpace(p.APIKey) != "",
			From:      p.From,
		})
	}
	webhookURL := ""
	path := strings.Trim(s.WebhookPath, "/")
	if rootURL != "" && path != "" {
		root := strings.TrimRight(rootURL, "/")
		webhookURL = root + "/webhooks/quo/" + path
	}
	return TextMessagingSettingsResponse{
		Providers:               provs,
		SendLimits:              s.SendLimits,
		WebhookPath:             s.WebhookPath,
		WebhookURL:              webhookURL,
		HasWebhookSigningSecret: strings.TrimSpace(s.WebhookSigningSecret) != "",
	}
}

// MergeTextMessagingUpdate merges an incoming payload over cur, preserving API keys when the client omits them (empty string).
func MergeTextMessagingUpdate(cur, in TextMessagingSettings) TextMessagingSettings {
	out := cur
	if len(in.Providers) == 0 {
		in.Providers = cur.Providers
	}
	prevByID := map[string]TextMessagingProvider{}
	for _, p := range cur.Providers {
		prevByID[p.ID] = p
	}
	out.Providers = make([]TextMessagingProvider, 0, len(in.Providers))
	for _, p := range in.Providers {
		merged := p
		if prev, ok := prevByID[p.ID]; ok && strings.TrimSpace(p.APIKey) == "" {
			merged.APIKey = prev.APIKey
		}
		out.Providers = append(out.Providers, merged)
	}
	out.SendLimits = in.SendLimits
	out.WebhookPath = strings.TrimSpace(in.WebhookPath)
	if strings.TrimSpace(in.WebhookSigningSecret) == "" {
		out.WebhookSigningSecret = cur.WebhookSigningSecret
	} else {
		out.WebhookSigningSecret = strings.TrimSpace(in.WebhookSigningSecret)
	}
	return out
}

// QuoProvider returns the quo provider config or nil.
func (s TextMessagingSettings) QuoProvider() *TextMessagingProvider {
	for i := range s.Providers {
		if s.Providers[i].ID == "quo" {
			return &s.Providers[i]
		}
	}
	return nil
}

func (s TextMessagingSendLimits) MinDelay() time.Duration {
	if s.MinDelayMS <= 0 {
		return 0
	}
	return time.Duration(s.MinDelayMS) * time.Millisecond
}
