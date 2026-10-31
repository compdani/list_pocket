package models

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"

	"github.com/pocketbase/pocketbase/tools/security"
)

// ChannelMessengerPrefix is reserved for messengers registered by plugins.
const ChannelMessengerPrefix = "ch:"

// AddressSource identifies where a channel finds a subscriber's destination.
type AddressSource string

const (
	AddressSourceEmail   AddressSource = "email"
	AddressSourcePhone   AddressSource = "phone"
	AddressSourceChannel AddressSource = "channel"
)

// ConsentSource identifies where a channel reads list-level consent from.
type ConsentSource string

const (
	ConsentSourceListStatus ConsentSource = "list_status"
	ConsentSourceSMSStatus  ConsentSource = "sms_status"
	ConsentSourceChannel    ConsentSource = "channel"
)

// UnsendableAction is what the manager does when a messenger returns
// ErrUnsendableDestination for a recipient.
type UnsendableAction string

const (
	// UnsendableNone treats the error like any other send error.
	UnsendableNone UnsendableAction = ""
	// UnsendableSMSOptOut opts the subscriber out of SMS and skips the ledger row.
	UnsendableSMSOptOut UnsendableAction = "sms_opt_out"
	// UnsendableSkip only skips the ledger row. The messenger owns any address changes.
	UnsendableSkip UnsendableAction = "skip"
)

// CampaignBodyNone hides the campaign's content step.
const CampaignBodyNone = "none"

// ChannelSpec describes how a messenger's campaigns behave: where addresses and
// consent come from, how content is compiled, and how sends are limited.
type ChannelSpec struct {
	ID           string
	Label        string
	Icon         string
	AddressLabel string

	Address AddressSource
	Consent ConsentSource

	// TextPipeline compiles bodies as plain text, drops the subject, and
	// treats campaigns.from_email as an optional sender override.
	TextPipeline bool
	TemplateType string
	ContentTypes []string
	AllowOptin   bool
	EmailHeaders bool
	Unsendable   UnsendableAction

	// SMSLimits applies the typed text messaging send_limits instead of the
	// app-wide rate settings.
	SMSLimits bool

	// CampaignBody is CampaignBodyNone when campaigns have no body, because the
	// channel sends something else, such as a provider-side template.
	CampaignBody string

	// CampaignFields is a form schema for this channel's campaign-level options,
	// which the plugin receives as msg.fields.
	CampaignFields json.RawMessage

	// MaxPerSecond caps this channel's send rate when above zero.
	MaxPerSecond int

	// Disabled marks a ch:* messenger with no registered spec, which must not send.
	Disabled bool
}

// SupportsContentType reports whether the channel accepts a campaign content type.
func (s ChannelSpec) SupportsContentType(contentType string) bool {
	return slices.Contains(s.ContentTypes, contentType)
}

var (
	emailChannelSpec = ChannelSpec{
		ID:           "email",
		Label:        "Email",
		Icon:         "mdi-email-outline",
		AddressLabel: "Email",
		Address:      AddressSourceEmail,
		Consent:      ConsentSourceListStatus,
		TemplateType: TemplateTypeCampaign,
		ContentTypes: []string{
			CampaignContentTypeRichtext,
			CampaignContentTypeHTML,
			CampaignContentTypeMarkdown,
			CampaignContentTypePlain,
			CampaignContentTypeVisual,
			CampaignContentTypeGrapes,
		},
		AllowOptin:   true,
		EmailHeaders: true,
	}

	channelsMu sync.RWMutex
	channels   = map[string]ChannelSpec{
		CampaignMessengerQuo: {
			ID:           CampaignMessengerQuo,
			Label:        "SMS",
			Icon:         "mdi-message-text-outline",
			AddressLabel: "Phone",
			Address:      AddressSourcePhone,
			Consent:      ConsentSourceSMSStatus,
			TextPipeline: true,
			TemplateType: TemplateTypeCampaignSMS,
			ContentTypes: []string{CampaignContentTypePlain},
			Unsendable:   UnsendableSMSOptOut,
			SMSLimits:    true,
		},
		ChannelWhatsApp: WhatsAppChannelSpec(WhatsAppSettings{}),
	}
)

// RegisterChannel adds a channel spec, replacing any existing spec with that ID.
func RegisterChannel(spec ChannelSpec) {
	channelsMu.Lock()
	defer channelsMu.Unlock()
	channels[spec.ID] = spec
}

// ChannelFor returns the spec that governs a messenger. Unregistered ch:*
// messengers get a disabled spec so they fail closed. Every other name is an
// email-style messenger (email, email-<name> and postback messengers).
func ChannelFor(messenger string) ChannelSpec {
	id := strings.TrimSpace(messenger)

	channelsMu.RLock()
	spec, ok := channels[id]
	channelsMu.RUnlock()
	if ok {
		return spec
	}

	if strings.HasPrefix(id, ChannelMessengerPrefix) {
		return ChannelSpec{
			ID:           id,
			Label:        id,
			Address:      AddressSourceChannel,
			Consent:      ConsentSourceChannel,
			TextPipeline: true,
			TemplateType: TemplateTypeCampaignSMS,
			ContentTypes: []string{CampaignContentTypePlain},
			Unsendable:   UnsendableSkip,
			Disabled:     true,
		}
	}

	spec = emailChannelSpec
	if id != "" {
		spec.ID = id
	}
	return spec
}

// PlaceholderEmailDomain marks contacts who have no real email. .invalid is
// reserved by RFC 2606 and can never route.
const PlaceholderEmailDomain = "no-email.invalid"

// NewPlaceholderEmail returns an address for a contact who only exists on a
// non-email channel, because subscribers.email is required and unique.
func NewPlaceholderEmail() string {
	return security.RandomStringWithAlphabet(24, "abcdefghijklmnopqrstuvwxyz0123456789") + "@" + PlaceholderEmailDomain
}

// IsPlaceholderEmail reports whether an address came from NewPlaceholderEmail.
func IsPlaceholderEmail(email string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(email)), "@"+PlaceholderEmailDomain)
}

// PhoneSourcedChannels returns registered channels that address the subscriber's
// phone column. Channel IDs are validated as [a-z0-9_:] at load, which makes them
// safe to place in SQL.
func PhoneSourcedChannels() []string {
	channelsMu.RLock()
	defer channelsMu.RUnlock()
	var out []string
	for id, spec := range channels {
		if strings.HasPrefix(id, ChannelMessengerPrefix) && spec.Address == AddressSourcePhone {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}
