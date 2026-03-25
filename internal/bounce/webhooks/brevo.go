package webhooks

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
)

// Brevo handles Brevo (Sendinblue) transactional e-mail webhooks.
// Configure the notify URL as POST {rootURL}/webhooks/service/brevo with Bearer auth
// matching the token stored in settings (see Brevo "Bearer token authorization").
type Brevo struct {
	secret string
}

var brevoCampaignUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// NewBrevo returns a Brevo webhook processor. secret is the Bearer token you configure in Brevo.
func NewBrevo(secret string) *Brevo {
	return &Brevo{secret: strings.TrimSpace(secret)}
}

type brevoEvent struct {
	Event         string `json:"event"`
	Email         string `json:"email"`
	TSEvent       int64  `json:"ts_event"`
	TS            int64  `json:"ts"`
	TSEpoch       int64  `json:"ts_epoch"`
	XMailinCustom string `json:"X-Mailin-custom"`
	Reason        string `json:"reason"`
}

// ProcessBounce parses a Brevo transactional webhook body. authorization must be
// "Authorization: Bearer <token>" matching the configured secret.
func (b *Brevo) ProcessBounce(authorization string, body []byte) ([]models.Bounce, error) {
	if b.secret == "" {
		return nil, errors.New("brevo webhook token not configured")
	}
	if err := verifyBearerToken(authorization, b.secret); err != nil {
		return nil, err
	}

	events, err := parseBrevoEvents(body)
	if err != nil {
		return nil, err
	}

	out := make([]models.Bounce, 0, len(events))
	for _, e := range events {
		typ, ok := brevoEventToBounceType(e.Event)
		if !ok {
			continue
		}
		email := strings.TrimSpace(strings.ToLower(e.Email))
		if email == "" {
			continue
		}

		meta := json.RawMessage(bytes.TrimSpace(body))
		if len(events) > 1 {
			raw, err := json.Marshal(e)
			if err == nil {
				meta = raw
			}
		}

		campUUID := ""
		if s := strings.TrimSpace(e.XMailinCustom); brevoCampaignUUID.MatchString(s) {
			campUUID = strings.ToLower(s)
		}

		out = append(out, models.Bounce{
			CampaignUUID: campUUID,
			Email:        email,
			Type:         typ,
			Meta:         meta,
			Source:       "brevo",
			CreatedAt:    brevoEventTime(e),
		})
	}

	return out, nil
}

func verifyBearerToken(authorization, secret string) error {
	tok, ok := parseBearerToken(authorization)
	if !ok {
		return errors.New("missing or invalid Authorization bearer token")
	}
	if len(tok) != len(secret) {
		return errors.New("invalid bearer token")
	}
	if subtle.ConstantTimeCompare([]byte(tok), []byte(secret)) != 1 {
		return errors.New("invalid bearer token")
	}
	return nil
}

func parseBearerToken(h string) (string, bool) {
	h = strings.TrimSpace(h)
	const prefix = "Bearer "
	if len(h) < len(prefix) {
		return "", false
	}
	if !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	t := strings.TrimSpace(h[len(prefix):])
	if t == "" {
		return "", false
	}
	return t, true
}

func parseBrevoEvents(body []byte) ([]brevoEvent, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, errors.New("empty brevo webhook body")
	}
	if body[0] == '[' {
		var evts []brevoEvent
		if err := json.Unmarshal(body, &evts); err != nil {
			return nil, err
		}
		return evts, nil
	}
	var e brevoEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, err
	}
	return []brevoEvent{e}, nil
}

func brevoEventToBounceType(event string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(event)) {
	case "hard_bounce", "blocked", "invalid_email", "error":
		return models.BounceTypeHard, true
	case "soft_bounce", "deferred":
		return models.BounceTypeSoft, true
	case "spam":
		return models.BounceTypeComplaint, true
	default:
		return "", false
	}
}

func brevoEventTime(e brevoEvent) time.Time {
	switch {
	case e.TSEvent > 0:
		return time.Unix(e.TSEvent, 0).UTC()
	case e.TS > 0:
		return time.Unix(e.TS, 0).UTC()
	case e.TSEpoch > 0:
		// Brevo documents ts_epoch as milliseconds UTC for many events.
		return time.UnixMilli(e.TSEpoch).UTC()
	default:
		return time.Now().UTC()
	}
}
