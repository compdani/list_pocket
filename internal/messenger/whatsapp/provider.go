// Package whatsapp sends and receives WhatsApp messages through one of two providers:
// Meta's Cloud API, or Whapi.Cloud, which is unofficial and links a phone by QR code.
// Everything above the Provider interface is shared, including consent and opt-out.
package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
)

const (
	// MessengerName is the campaign messenger value.
	MessengerName = models.ChannelWhatsApp

	sendTimeout = 15 * time.Second

	// ServiceWindow is how long after a user's message free-form text is allowed on
	// providers with a window.
	ServiceWindow = 24 * time.Hour
)

// ErrUnsupported means the active provider can't do this.
var ErrUnsupported = errors.New("not supported by this WhatsApp provider")

// Capabilities are the differences between providers that the rest of the app sees.
type Capabilities struct {
	Templates    bool `json:"templates"`
	Window       bool `json:"window"`
	ChannelPosts bool `json:"channel_posts"`
}

// SendResult is an accepted send. Acceptance isn't delivery: that comes by webhook.
type SendResult struct {
	MessageID string
	// RecipientID is the provider's ID for the recipient, which can differ from the
	// number sent to.
	RecipientID string
}

// Template is an approved provider-side template.
type Template struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Language        string              `json:"language"`
	Category        string              `json:"category"`
	Status          string              `json:"status"`
	ParameterFormat string              `json:"parameter_format"`
	Components      []TemplateComponent `json:"components"`
}

// TemplateComponent is one part of a template definition.
type TemplateComponent struct {
	Type    string           `json:"type"`
	Format  string           `json:"format,omitempty"`
	Text    string           `json:"text,omitempty"`
	Buttons []TemplateButton `json:"buttons,omitempty"`
}

// TemplateButton is a button in a template definition.
type TemplateButton struct {
	Type string `json:"type"`
	Text string `json:"text"`
	URL  string `json:"url,omitempty"`
}

// Channel is a WhatsApp Channel, the one-way broadcast feature with followers.
type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ChannelPost is one post to a WhatsApp Channel.
type ChannelPost struct {
	ChannelID string
	Kind      string
	Body      string
	MediaURL  string
	// Title is the heading of a link post's preview card.
	Title string
}

// Channel post kinds.
const (
	PostText  = "text"
	PostImage = "image"
	PostVideo = "video"
	PostLink  = "link"
)

// Provider is one way of reaching WhatsApp.
type Provider interface {
	Capabilities() Capabilities
	SendText(ctx context.Context, to, body string) (SendResult, error)
	SendTemplate(ctx context.Context, to string, tpl TemplateSend) (SendResult, error)
	ListTemplates(ctx context.Context) ([]Template, error)
	ListChannels(ctx context.Context) ([]Channel, error)
	PostToChannel(ctx context.Context, post ChannelPost) (SendResult, error)
	CheckConnection(ctx context.Context) error
}

// NewProvider returns the settings' active provider.
func NewProvider(s models.WhatsAppSettings, client *http.Client) Provider {
	n := s.Normalized()
	if n.Provider == models.WhatsAppProviderWhapi {
		return &whapiProvider{token: n.WhapiToken, base: n.WhapiAPIBase, http: client}
	}
	return &cloudProvider{
		token:   n.AccessToken,
		base:    n.APIBase + "/" + n.APIVersion,
		phoneID: n.PhoneNumberID,
		wabaID:  n.WABAID,
		http:    client,
	}
}

// APIError is a provider's refusal. Code is the provider's own error code, or 0.
type APIError struct {
	HTTPStatus int
	Code       int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("whatsapp: HTTP %d, code %d: %s", e.HTTPStatus, e.Code, e.Message)
}

// Action is what to do about a failed send.
type Action int

const (
	// ActionError keeps the send retryable, and enough of them pause a campaign.
	ActionError Action = iota
	// ActionRetire stops sending to the number, because it can't receive this.
	ActionRetire
	// ActionOptOut withdraws consent, because the user asked to stop.
	ActionOptOut
)

// Classify sorts a Cloud API error code. Every code not listed is an error, because
// retiring a number or withdrawing consent on a guess would be worse.
func Classify(code int) Action {
	switch code {
	case 131026:
		return ActionRetire
	case 131050:
		return ActionOptOut
	}
	return ActionError
}

// digits strips everything but digits from a phone number.
func digits(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
