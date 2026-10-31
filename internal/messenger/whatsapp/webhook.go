package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const signaturePrefix = "sha256="

// VerifySignature checks X-Hub-Signature-256 over the raw body. A blank secret never
// verifies, so an unconfigured install can't be satisfied by an empty key.
func VerifySignature(body []byte, appSecret, header string) bool {
	if appSecret == "" || !strings.HasPrefix(header, signaturePrefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, signaturePrefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// TokensEqual compares secrets in constant time. A blank expected value never matches.
func TokensEqual(expected, got string) bool {
	return expected != "" && subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}

// Event types, in the shape shared by every provider.
const (
	EventMessage    = "message"
	EventStatus     = "status"
	EventPreference = "preference"
	EventConnection = "connection"
)

// Event is one thing a provider told us.
type Event struct {
	Type string

	// Message.
	MessageID string
	From      string
	Name      string
	Text      string
	Kind      string

	// Status. MessageID says which send it's about.
	Status      string
	RecipientID string
	ErrorCode   int
	ErrorTitle  string

	// Preference: a user stopping or resuming marketing messages.
	Preference string

	// Connection: the phone link state.
	Connected bool
	Note      string
}

type cloudWebhook struct {
	Entry []struct {
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					WaID    string `json:"wa_id"`
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"contacts"`
				Messages []struct {
					ID   string `json:"id"`
					From string `json:"from"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
					Button struct {
						Text string `json:"text"`
					} `json:"button"`
					Interactive struct {
						ButtonReply struct {
							Title string `json:"title"`
						} `json:"button_reply"`
						ListReply struct {
							Title string `json:"title"`
						} `json:"list_reply"`
					} `json:"interactive"`
				} `json:"messages"`
				Statuses []struct {
					ID          string `json:"id"`
					Status      string `json:"status"`
					RecipientID string `json:"recipient_id"`
					Errors      []struct {
						Code  int    `json:"code"`
						Title string `json:"title"`
					} `json:"errors"`
				} `json:"statuses"`
				UserPreferences []struct {
					WaID     string `json:"wa_id"`
					Category string `json:"category"`
					Value    string `json:"value"`
				} `json:"user_preferences"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// ParseCloudWebhook turns a verified Cloud API payload into events. Deliveries are
// batched, and changes for another phone number are ignored because a business
// account can have several.
func ParseCloudWebhook(body []byte, phoneNumberID string) ([]Event, error) {
	var payload cloudWebhook
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("whatsapp: invalid webhook: %w", err)
	}
	var out []Event
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			v := change.Value
			if id := v.Metadata.PhoneNumberID; id != "" && id != phoneNumberID {
				continue
			}
			names := map[string]string{}
			for _, c := range v.Contacts {
				names[c.WaID] = c.Profile.Name
			}
			for _, m := range v.Messages {
				text := m.Text.Body
				switch {
				case m.Button.Text != "":
					text = m.Button.Text
				case m.Interactive.ButtonReply.Title != "":
					text = m.Interactive.ButtonReply.Title
				case m.Interactive.ListReply.Title != "":
					text = m.Interactive.ListReply.Title
				}
				out = append(out, Event{
					Type: EventMessage, MessageID: m.ID, From: digits(m.From),
					Name: names[m.From], Text: text, Kind: m.Type,
				})
			}
			for _, s := range v.Statuses {
				e := Event{Type: EventStatus, MessageID: s.ID, Status: s.Status, RecipientID: digits(s.RecipientID)}
				if len(s.Errors) > 0 {
					e.ErrorCode, e.ErrorTitle = s.Errors[0].Code, s.Errors[0].Title
				}
				out = append(out, e)
			}
			for _, p := range v.UserPreferences {
				// wa_id is left out when the user hides their number, and then there's
				// nobody to apply the preference to.
				if p.Category == "marketing_messages" && p.WaID != "" {
					out = append(out, Event{Type: EventPreference, From: digits(p.WaID), Preference: p.Value})
				}
			}
		}
	}
	return out, nil
}

type whapiWebhook struct {
	Messages []struct {
		ID       string `json:"id"`
		FromMe   bool   `json:"from_me"`
		Type     string `json:"type"`
		ChatID   string `json:"chat_id"`
		From     string `json:"from"`
		FromName string `json:"from_name"`
		Text     struct {
			Body string `json:"body"`
		} `json:"text"`
	} `json:"messages"`
	Statuses []struct {
		ID          string          `json:"id"`
		Status      string          `json:"status"`
		RecipientID string          `json:"recipient_id"`
		Timestamp   json.RawMessage `json:"timestamp"`
	} `json:"statuses"`
	Health *struct {
		Status struct {
			Code int    `json:"code"`
			Text string `json:"text"`
		} `json:"status"`
	} `json:"health"`
	User  json.RawMessage `json:"user"`
	Event struct {
		Type  string `json:"type"`
		Event string `json:"event"`
	} `json:"event"`
}

// whapiPersonalChat is the chat_id suffix of a one-to-one chat. Group and channel
// messages aren't replies from a subscriber.
const whapiPersonalChat = "@s.whatsapp.net"

// ParseWhapiWebhook turns a Whapi payload into events. Messages from our own account
// are skipped, so our sends don't look like replies.
func ParseWhapiWebhook(body []byte) ([]Event, error) {
	var payload whapiWebhook
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("whatsapp: invalid webhook: %w", err)
	}
	var out []Event
	for _, m := range payload.Messages {
		if m.FromMe || !strings.HasSuffix(m.ChatID, whapiPersonalChat) {
			continue
		}
		out = append(out, Event{
			Type: EventMessage, MessageID: m.ID, From: digits(m.From),
			Name: m.FromName, Text: m.Text.Body, Kind: m.Type,
		})
	}
	for _, s := range payload.Statuses {
		out = append(out, Event{
			Type: EventStatus, MessageID: s.ID, Status: s.Status,
			RecipientID: digits(strings.SplitN(s.RecipientID, "@", 2)[0]),
		})
	}
	switch {
	case payload.Event.Type == "users" && payload.Event.Event == "delete":
		out = append(out, Event{Type: EventConnection, Connected: false, Note: "The phone was disconnected. Scan the QR code in Whapi to reconnect."})
	case payload.Event.Type == "users" && payload.Event.Event == "post":
		out = append(out, Event{Type: EventConnection, Connected: true, Note: "The phone is connected."})
	case payload.Health != nil:
		// INIT and LAUNCH pass in seconds, so they don't change the state shown.
		switch status := payload.Health.Status.Text; status {
		case whapiStatusAuth:
			out = append(out, Event{Type: EventConnection, Connected: true, Note: "The phone is connected."})
		case "INIT", "LAUNCH", "":
		default:
			out = append(out, Event{Type: EventConnection, Connected: false, Note: whapiStatusNote(status)})
		}
	}
	return out, nil
}

// InboundText returns text for the timeline. Media isn't downloaded, so it's shown as
// its type.
func (e Event) InboundText() string {
	if text := strings.TrimSpace(e.Text); text != "" {
		return text
	}
	return "[" + e.Kind + "]"
}

// WindowOpen reports whether free-form text is allowed, given when the user last
// messaged.
func WindowOpen(last time.Time, hasInbound bool, now time.Time) bool {
	return hasInbound && now.Sub(last) < ServiceWindow
}
