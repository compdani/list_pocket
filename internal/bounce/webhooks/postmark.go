package webhooks

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/models"
)

type postmarkNotif struct {
	RecordType    string            `json:"RecordType"`
	MessageStream string            `json:"MessageStream"`
	ID            int               `json:"ID"`
	Type          string            `json:"Type"`
	TypeCode      int               `json:"TypeCode"`
	Name          string            `json:"Name"`
	Tag           string            `json:"Tag"`
	MessageID     string            `json:"MessageID"`
	Metadata      map[string]string `json:"Metadata"`
	ServerID      int               `json:"ServerID"`
	Description   string            `json:"Description"`
	Details       string            `json:"Details"`
	Email         string            `json:"Email"`
	From          string            `json:"From"`
	BouncedAt     time.Time         `json:"BouncedAt"` // "2019-11-05T16:33:54.9070259Z"
	DumpAvailable bool              `json:"DumpAvailable"`
	Inactive      bool              `json:"Inactive"`
	CanActivate   bool              `json:"CanActivate"`
	Subject       string            `json:"Subject"`
	Content       string            `json:"Content"`
}

// Postmark handles webhook notifications (mainly bounce notifications).
type Postmark struct {
	user     []byte
	password []byte
}

func NewPostmark(username, password string) *Postmark {
	return &Postmark{
		user:     []byte(username),
		password: []byte(password),
	}
}

// ProcessBounce processes Postmark bounce notifications and returns one object.
func (p *Postmark) ProcessBounce(b []byte, r *http.Request) ([]models.Bounce, error) {
	if err := p.checkBasicAuth(r); err != nil {
		return nil, err
	}

	var n postmarkNotif
	if err := json.Unmarshal(b, &n); err != nil {
		return nil, fmt.Errorf("error unmarshalling postmark notification: %v", err)
	}

	// Ignore irrelevant messages.
	if n.RecordType != "Bounce" && n.RecordType != "SpamComplaint" {
		return nil, nil
	}

	supportedBounceType := true
	typ := models.BounceTypeHard
	switch n.Type {
	case "HardBounce", "BadEmailAddress", "ManuallyDeactivated":
		typ = models.BounceTypeHard
	case "SoftBounce", "Transient", "DnsError", "SpamNotification", "VirusNotification", "DMARCPolicy":
		typ = models.BounceTypeSoft
	case "SpamComplaint":
		typ = models.BounceTypeComplaint
	default:
		supportedBounceType = false
	}

	if !supportedBounceType {
		return nil, fmt.Errorf("unsupported bounce type: %v", n.Type)
	}

	// Look for the campaign ID in headers.
	campUUID := ""
	if v, ok := n.Metadata[models.EmailHeaderCampaignUUID]; ok {
		campUUID = v
	}

	messageID := normalizeMessageID(n.MessageID)
	if messageID == "" {
		if v, ok := n.Metadata[models.EmailHeaderMessageId]; ok {
			messageID = normalizeMessageID(v)
		}
	}

	return []models.Bounce{{
		Email:        strings.ToLower(n.Email),
		CampaignUUID: campUUID,
		MessageID:    messageID,
		Type:         typ,
		Source:       "postmark",
		Meta:         json.RawMessage(b),
		CreatedAt:    n.BouncedAt,
	}}, nil
}

func (p *Postmark) checkBasicAuth(r *http.Request) error {
	if len(p.user) == 0 || len(p.password) == 0 {
		return nil
	}
	if r == nil {
		return apperr.New(http.StatusUnauthorized, "unauthorized")
	}

	username, password, ok := r.BasicAuth()
	if !ok ||
		subtle.ConstantTimeCompare([]byte(username), p.user) != 1 ||
		subtle.ConstantTimeCompare([]byte(password), p.password) != 1 {
		return apperr.New(http.StatusUnauthorized, "unauthorized")
	}
	return nil
}
