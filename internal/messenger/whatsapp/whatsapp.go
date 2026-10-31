package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/models"
)

// Messenger implements manager.Messenger. Settings are read per send, so saving
// credentials takes effect without a restart.
type Messenger struct {
	load func() models.WhatsAppSettings
	core *core.Core
	http *http.Client
	log  *log.Logger
}

// NewMessenger returns a Messenger reading its settings through load.
func NewMessenger(load func() models.WhatsAppSettings, c *core.Core, lo *log.Logger) *Messenger {
	return &Messenger{load: load, core: c, http: &http.Client{Timeout: sendTimeout}, log: lo}
}

// Name returns the campaign messenger value.
func (m *Messenger) Name() string { return MessengerName }

// Flush does nothing: a send finishes before Push returns.
func (m *Messenger) Flush() error { return nil }

// Close does nothing: the client holds no state to release.
func (m *Messenger) Close() error { return nil }

// Provider returns the active provider, or an error if WhatsApp isn't set up.
func (m *Messenger) Provider() (Provider, error) {
	s := m.load()
	if !s.Configured() {
		return nil, errors.New("whatsapp is not enabled or not configured")
	}
	return NewProvider(s, m.http), nil
}

// Push sends one message. A number the provider can't reach is retired, and a user
// who stopped marketing messages is opted out. Both return ErrUnsendableDestination,
// so the manager skips the recipient instead of retrying.
func (m *Messenger) Push(msg models.Message) error {
	provider, err := m.Provider()
	if err != nil {
		return err
	}
	if len(msg.To) == 0 || strings.TrimSpace(msg.To[0]) == "" {
		return models.ErrUnsendableDestination
	}
	to := strings.TrimSpace(msg.To[0])
	// Without a plus sign the API assumes the business number's country, which can
	// reach the wrong person.
	if !strings.HasPrefix(to, "+") {
		m.log.Printf("whatsapp: skipping subscriber %s: phone has no country code", msg.Subscriber.RecordID)
		return models.ErrUnsendableDestination
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	var (
		result SendResult
		body   string
	)
	if provider.Capabilities().Templates {
		choice, err := ChoiceFromMessage(msg)
		if err != nil {
			return err
		}
		tpl, err := BuildTemplate(choice, msg.Subscriber)
		if err != nil {
			return err
		}
		body = "[template: " + tpl.Name + "]"
		result, err = provider.SendTemplate(ctx, to, tpl)
		if err != nil {
			return m.handleSendError(msg, to, err)
		}
	} else {
		body = strings.TrimSpace(string(msg.Body))
		if body == "" {
			return errors.New("whatsapp: message has no text")
		}
		result, err = provider.SendText(ctx, to, body)
		if err != nil {
			return m.handleSendError(msg, to, err)
		}
	}

	m.afterSend(msg, to, body, "send", result)
	return nil
}

func (m *Messenger) handleSendError(msg models.Message, to string, err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch Classify(apiErr.Code) {
	case ActionRetire:
		if mErr := m.core.MarkChannelAddressInvalid(msg.Subscriber.RecordID, MessengerName, to); mErr != nil {
			m.log.Printf("whatsapp: retiring number: %v", mErr)
		}
		return models.ErrUnsendableDestination
	case ActionOptOut:
		if _, oErr := m.core.OptOutChannelAddress(MessengerName, to); oErr != nil {
			m.log.Printf("whatsapp: opt-out after refusal: %v", oErr)
		}
		return models.ErrUnsendableDestination
	}
	return fmt.Errorf("whatsapp send: %w", err)
}

func (m *Messenger) afterSend(msg models.Message, to, body, kind string, result SendResult) {
	campaign := ""
	if msg.Campaign != nil {
		campaign = msg.Campaign.RecordID
	}
	if _, err := m.core.RecordChannelEvent(core.ChannelEvent{
		Channel: MessengerName, Direction: core.ChannelDirectionOutbound, Kind: kind, Status: "sent",
		ProviderMsgID: result.MessageID, SubscriberID: msg.Subscriber.RecordID, CampaignID: campaign, Address: to, Body: body,
	}); err != nil {
		m.log.Printf("whatsapp: recording send: %v", err)
	}
	// The provider can know this person by another form of the number, and later
	// messages from them will use it.
	if id := digits(result.RecipientID); id != "" && id != digits(to) {
		if err := m.core.SetChannelAddressAlias(msg.Subscriber.RecordID, MessengerName, id); err != nil {
			m.log.Printf("whatsapp: storing alias: %v", err)
		}
	}
}

// ReplyError says why a reply wasn't sent, in terms the admin UI can show.
type ReplyError struct {
	Reason string
}

func (e *ReplyError) Error() string { return e.Reason }

// Reply reasons.
const (
	ReplyNoPhone      = "no_phone"
	ReplyOptedOut     = "opted_out"
	ReplyWindowClosed = "window_closed"
)

// Thread is what the reply box shows.
type Thread struct {
	Events         []core.ChannelEventRow `json:"events"`
	HasPhone       bool                   `json:"has_phone"`
	OptedOut       bool                   `json:"opted_out"`
	RequiresWindow bool                   `json:"requires_window"`
	WindowOpen     bool                   `json:"window_open"`
	WindowClosesAt *time.Time             `json:"window_closes_at"`
	Configured     bool                   `json:"configured"`
}

// GetThread returns a subscriber's conversation and whether they can be replied to.
func (m *Messenger) GetThread(sub models.Subscriber) (Thread, error) {
	events, err := m.core.ListChannelEvents(sub.RecordID, MessengerName, 50)
	if err != nil {
		return Thread{}, err
	}
	optedOut, err := m.core.ChannelHasOptOut(sub.RecordID, MessengerName)
	if err != nil {
		return Thread{}, err
	}
	out := Thread{Events: events, HasPhone: strings.TrimSpace(sub.Phone) != "", OptedOut: optedOut}

	provider, err := m.Provider()
	if err != nil {
		return out, nil
	}
	out.Configured = true
	out.RequiresWindow = provider.Capabilities().Window
	last, ok, err := m.core.LastChannelInbound(sub.RecordID, MessengerName)
	if err != nil {
		return out, err
	}
	out.WindowOpen = WindowOpen(last, ok, time.Now())
	if out.WindowOpen {
		closes := last.Add(ServiceWindow)
		out.WindowClosesAt = &closes
	}
	return out, nil
}

// Reply sends free-form text straight away rather than through the send queue, so an
// error reaches the person replying. Someone who opted out is never messaged, and
// providers with a service window only allow it while the window is open.
func (m *Messenger) Reply(sub models.Subscriber, body string) error {
	provider, err := m.Provider()
	if err != nil {
		return err
	}
	to := strings.TrimSpace(sub.Phone)
	if to == "" {
		return &ReplyError{Reason: ReplyNoPhone}
	}
	optedOut, err := m.core.ChannelHasOptOut(sub.RecordID, MessengerName)
	if err != nil {
		return err
	}
	if optedOut {
		return &ReplyError{Reason: ReplyOptedOut}
	}
	if provider.Capabilities().Window {
		last, ok, err := m.core.LastChannelInbound(sub.RecordID, MessengerName)
		if err != nil {
			return err
		}
		if !WindowOpen(last, ok, time.Now()) {
			return &ReplyError{Reason: ReplyWindowClosed}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	result, err := provider.SendText(ctx, to, body)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == 131047 {
			return &ReplyError{Reason: ReplyWindowClosed}
		}
		return err
	}
	m.afterSend(models.Message{Subscriber: sub}, to, body, "reply", result)
	return nil
}
