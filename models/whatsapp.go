package models

import "strings"

const (
	// ListPocketSettingsTypeWhatsApp is the listpocket_settings row for WhatsApp.
	ListPocketSettingsTypeWhatsApp = "whatsapp"

	// ChannelWhatsApp is the WhatsApp messenger. The ch: prefix selects per-channel
	// consent, which WhatsApp needs just as plugin channels do.
	ChannelWhatsApp = ChannelMessengerPrefix + "whatsapp"

	WhatsAppProviderCloud = "cloud"
	WhatsAppProviderWhapi = "whapi"

	whatsAppDefaultAPIBase      = "https://graph.facebook.com"
	whatsAppDefaultAPIVersion   = "v23.0"
	whatsAppDefaultWhapiAPIBase = "https://gate.whapi.cloud"
	whatsAppCloudRate           = 40
	whatsAppWhapiRate           = 1
)

// WhatsAppSettings is stored in listpocket_settings (type=whatsapp) as JSON.
type WhatsAppSettings struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`

	// Cloud API.
	AccessToken   string `json:"access_token,omitempty"`
	AppSecret     string `json:"app_secret,omitempty"`
	VerifyToken   string `json:"verify_token,omitempty"`
	PhoneNumberID string `json:"phone_number_id,omitempty"`
	WABAID        string `json:"waba_id,omitempty"`
	APIBase       string `json:"api_base,omitempty"`
	APIVersion    string `json:"api_version,omitempty"`

	// Whapi.
	WhapiToken        string `json:"whapi_token,omitempty"`
	WhapiWebhookToken string `json:"whapi_webhook_token,omitempty"`
	WhapiAPIBase      string `json:"whapi_api_base,omitempty"`
	WhapiRiskAccepted bool   `json:"whapi_risk_accepted,omitempty"`

	// MaxPerSecond overrides the provider's default send rate when above zero.
	MaxPerSecond int `json:"max_per_second,omitempty"`

	// WhapiConnected is the phone link state from Whapi's webhooks, and
	// WhapiConnectionNote says what last changed it. Saving settings leaves them alone.
	WhapiConnected      *bool  `json:"whapi_connected,omitempty"`
	WhapiConnectionNote string `json:"whapi_connection_note,omitempty"`
}

// Normalized fills in defaults and trims the fields people paste in.
func (s WhatsAppSettings) Normalized() WhatsAppSettings {
	s.Provider = strings.TrimSpace(s.Provider)
	if s.Provider != WhatsAppProviderWhapi {
		s.Provider = WhatsAppProviderCloud
	}
	s.AccessToken = strings.TrimSpace(s.AccessToken)
	s.AppSecret = strings.TrimSpace(s.AppSecret)
	s.VerifyToken = strings.TrimSpace(s.VerifyToken)
	s.PhoneNumberID = strings.TrimSpace(s.PhoneNumberID)
	s.WABAID = strings.TrimSpace(s.WABAID)
	s.WhapiToken = strings.TrimSpace(s.WhapiToken)
	s.WhapiWebhookToken = strings.TrimSpace(s.WhapiWebhookToken)
	s.APIBase = strings.TrimRight(strings.TrimSpace(s.APIBase), "/")
	if s.APIBase == "" {
		s.APIBase = whatsAppDefaultAPIBase
	}
	s.APIVersion = strings.TrimSpace(s.APIVersion)
	if s.APIVersion == "" {
		s.APIVersion = whatsAppDefaultAPIVersion
	}
	s.WhapiAPIBase = strings.TrimRight(strings.TrimSpace(s.WhapiAPIBase), "/")
	if s.WhapiAPIBase == "" {
		s.WhapiAPIBase = whatsAppDefaultWhapiAPIBase
	}
	return s
}

// Configured reports whether the active provider has what it needs to send.
func (s WhatsAppSettings) Configured() bool {
	n := s.Normalized()
	if !n.Enabled {
		return false
	}
	if n.Provider == WhatsAppProviderWhapi {
		return n.WhapiToken != "" && n.WhapiRiskAccepted
	}
	return n.AccessToken != "" && n.PhoneNumberID != ""
}

// SendRate is the campaign send rate for the active provider.
func (s WhatsAppSettings) SendRate() int {
	if s.MaxPerSecond > 0 {
		return s.MaxPerSecond
	}
	if s.Normalized().Provider == WhatsAppProviderWhapi {
		return whatsAppWhapiRate
	}
	return whatsAppCloudRate
}

// WhatsAppSettingsResponse is safe for GET: it says which secrets are set, without
// their values.
type WhatsAppSettingsResponse struct {
	Enabled             bool   `json:"enabled"`
	Provider            string `json:"provider"`
	HasAccessToken      bool   `json:"has_access_token"`
	HasAppSecret        bool   `json:"has_app_secret"`
	HasVerifyToken      bool   `json:"has_verify_token"`
	PhoneNumberID       string `json:"phone_number_id"`
	WABAID              string `json:"waba_id"`
	APIBase             string `json:"api_base"`
	APIVersion          string `json:"api_version"`
	HasWhapiToken       bool   `json:"has_whapi_token"`
	WhapiAPIBase        string `json:"whapi_api_base"`
	WhapiRiskAccepted   bool   `json:"whapi_risk_accepted"`
	MaxPerSecond        int    `json:"max_per_second"`
	SendRate            int    `json:"send_rate"`
	WebhookURL          string `json:"webhook_url"`
	WhapiWebhookURL     string `json:"whapi_webhook_url"`
	WhapiConnected      *bool  `json:"whapi_connected"`
	WhapiConnectionNote string `json:"whapi_connection_note"`
}

// ToResponse masks secrets. The Whapi webhook URL contains its secret token, which
// the admin needs in order to configure Whapi.
func (s WhatsAppSettings) ToResponse(rootURL string) WhatsAppSettingsResponse {
	n := s.Normalized()
	root := strings.TrimRight(rootURL, "/")
	out := WhatsAppSettingsResponse{
		Enabled:             n.Enabled,
		Provider:            n.Provider,
		HasAccessToken:      n.AccessToken != "",
		HasAppSecret:        n.AppSecret != "",
		HasVerifyToken:      n.VerifyToken != "",
		PhoneNumberID:       n.PhoneNumberID,
		WABAID:              n.WABAID,
		APIBase:             n.APIBase,
		APIVersion:          n.APIVersion,
		HasWhapiToken:       n.WhapiToken != "",
		WhapiAPIBase:        n.WhapiAPIBase,
		WhapiRiskAccepted:   n.WhapiRiskAccepted,
		MaxPerSecond:        n.MaxPerSecond,
		SendRate:            n.SendRate(),
		WhapiConnected:      n.WhapiConnected,
		WhapiConnectionNote: n.WhapiConnectionNote,
	}
	// With no root URL configured these stay relative, and the admin UI adds its own origin.
	out.WebhookURL = root + "/webhooks/whatsapp"
	if n.WhapiWebhookToken != "" {
		out.WhapiWebhookURL = root + "/webhooks/whatsapp/whapi/" + n.WhapiWebhookToken
	}
	return out
}

// MergeWhatsAppUpdate applies an update, keeping secrets the client left blank and
// the connection state, which only webhooks change.
func MergeWhatsAppUpdate(cur, in WhatsAppSettings) WhatsAppSettings {
	if strings.TrimSpace(in.AccessToken) == "" {
		in.AccessToken = cur.AccessToken
	}
	if strings.TrimSpace(in.AppSecret) == "" {
		in.AppSecret = cur.AppSecret
	}
	if strings.TrimSpace(in.VerifyToken) == "" {
		in.VerifyToken = cur.VerifyToken
	}
	if strings.TrimSpace(in.WhapiToken) == "" {
		in.WhapiToken = cur.WhapiToken
	}
	in.WhapiWebhookToken = cur.WhapiWebhookToken
	in.WhapiConnected = cur.WhapiConnected
	in.WhapiConnectionNote = cur.WhapiConnectionNote
	return in.Normalized()
}

// WhatsAppChannelSpec describes the channel on a provider. Cloud API campaigns send a
// provider-side template, so they have no body. Whapi campaigns send their body as text.
func WhatsAppChannelSpec(s WhatsAppSettings) ChannelSpec {
	spec := ChannelSpec{
		ID:           ChannelWhatsApp,
		Label:        "WhatsApp",
		Icon:         "mdi-whatsapp",
		AddressLabel: "Phone",
		Address:      AddressSourcePhone,
		Consent:      ConsentSourceChannel,
		TextPipeline: true,
		TemplateType: TemplateTypeCampaignSMS,
		ContentTypes: []string{CampaignContentTypePlain},
		Unsendable:   UnsendableSkip,
		MaxPerSecond: s.SendRate(),
	}
	if s.Normalized().Provider == WhatsAppProviderCloud {
		spec.CampaignBody = CampaignBodyNone
	}
	return spec
}
