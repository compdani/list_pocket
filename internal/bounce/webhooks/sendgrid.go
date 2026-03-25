package webhooks

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
)

type sendgridNotif struct {
	Email                string `json:"email"`
	Timestamp            int64  `json:"timestamp"`
	Event                string `json:"event"`
	BounceClassification string `json:"bounce_classification"`

	// SendGrid flattens all X-headers and adds them to the bounce
	// event notification.
	CampaignUUID string `json:"XListpocketCampaign"`
	// Some SendGrid webhook payloads may keep the dashes in the flattened X-header name.
	// Support that variant too.
	CampaignUUIDDashed string `json:"X-Listpocket-Campaign"`
}

func (n *sendgridNotif) UnmarshalJSON(data []byte) error {
	// Use a map to robustly handle:
	// - timestamp being encoded as a JSON number
	// - SendGrid's flattened header field name being slightly different
	//   (e.g. `XListpocketCampaign` vs `X-Listpocket-Campaign`).
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if v, ok := raw["email"].(string); ok {
		n.Email = v
	}
	if v, ok := raw["event"].(string); ok {
		n.Event = v
	}
	if v, ok := raw["bounce_classification"].(string); ok {
		n.BounceClassification = v
	}

	if v, ok := raw["timestamp"]; ok {
		ts, ok := sendgridAnyToInt64(v)
		if ok {
			n.Timestamp = ts
		}
	}

	// First try the two most likely flattened header field names.
	if v, ok := raw["XListpocketCampaign"].(string); ok {
		n.CampaignUUID = strings.TrimSpace(v)
	}
	if n.CampaignUUID == "" {
		if v, ok := raw["X-Listpocket-Campaign"].(string); ok {
			n.CampaignUUID = strings.TrimSpace(v)
		}
	}

	// Fallback: scan all keys for a normalized match.
	if n.CampaignUUID == "" {
		for k, v := range raw {
			if normalizeSendgridHeaderKey(k) != "xlistpocketcampaign" {
				continue
			}
			switch t := v.(type) {
			case string:
				n.CampaignUUID = strings.TrimSpace(t)
			default:
				n.CampaignUUID = strings.TrimSpace(fmt.Sprint(v))
			}
			if n.CampaignUUID != "" {
				break
			}
		}
	}

	return nil
}

// Sendgrid handles Sendgrid/SNS webhook notifications including confirming SNS topic subscription
// requests and bounce notifications.
type Sendgrid struct {
	pubKey *ecdsa.PublicKey
}

// NewSendgrid returns a new Sendgrid instance.
func NewSendgrid(key string) (*Sendgrid, error) {
	pubKey, err := parseSendgridPublicKey(key)
	if err != nil {
		return nil, err
	}

	ecdsaPubKey, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("sendgrid webhook key is not an ECDSA public key")
	}

	return &Sendgrid{pubKey: ecdsaPubKey}, nil
}

func parseSendgridPublicKey(key string) (any, error) {
	key = normalizeSendgridKey(key)
	if key == "" {
		return nil, errors.New("sendgrid webhook key is empty")
	}

	// Try PEM first as that's the usual format from SendGrid.
	if block, _ := pem.Decode([]byte(key)); block != nil {
		pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("invalid sendgrid PEM public key: %w", err)
		}
		return pubKey, nil
	}

	// Fallback to raw base64-encoded DER.
	sigB, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		// Also accept URL-safe/base64-without-padding variants.
		sigB, err = base64.RawStdEncoding.DecodeString(key)
		if err != nil {
			sigB, err = base64.RawURLEncoding.DecodeString(key)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid sendgrid webhook key: expected PEM or base64 DER: %w", err)
		}
	}

	pubKey, err := x509.ParsePKIXPublicKey(sigB)
	if err != nil {
		return nil, fmt.Errorf("invalid sendgrid DER public key: %w", err)
	}
	return pubKey, nil
}

func normalizeSendgridKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}

	// If the key was saved as a quoted/escaped string, unquote first.
	if len(key) >= 2 && key[0] == '"' && key[len(key)-1] == '"' {
		if unq, err := strconv.Unquote(key); err == nil {
			key = unq
		}
	}

	// Support literal escaped newlines from env/config strings.
	key = strings.ReplaceAll(key, `\n`, "\n")
	return strings.TrimSpace(key)
}

func normalizeSendgridHeaderKey(key string) string {
	// SendGrid flattens X-headers, but the flattened JSON field name can vary (e.g. `XListpocketCampaign`
	// vs `X-Listpocket-Campaign`). Normalize by stripping non-alphanumerics and lowercasing.
	key = strings.ToLower(key)
	var sb strings.Builder
	sb.Grow(len(key))
	for i := 0; i < len(key); i++ {
		c := key[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

func sendgridAnyToInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case json.Number:
		i, err := t.Int64()
		if err == nil {
			return i, true
		}
	}
	return 0, false
}

// ProcessBounce processes Sendgrid bounce notifications and returns one or more Bounce objects.
func (s *Sendgrid) ProcessBounce(sig, timestamp string, b []byte) ([]models.Bounce, error) {
	if err := s.verifyNotif(sig, timestamp, b); err != nil {
		return nil, err
	}

	var notifs []sendgridNotif
	if err := json.Unmarshal(b, &notifs); err != nil {
		return nil, fmt.Errorf("error unmarshalling Sendgrid notification: %v", err)
	}

	out := make([]models.Bounce, 0, len(notifs))
	for _, n := range notifs {
		if n.Event != "bounce" && n.Event != "dropped" {
			continue
		}

		typ := models.BounceTypeHard
		if n.BounceClassification == "technical" || n.BounceClassification == "content" {
			typ = models.BounceTypeSoft
		}

		bn := models.Bounce{
			CampaignUUID: strings.TrimSpace(n.CampaignUUID),
			Email:        strings.ToLower(strings.TrimSpace(n.Email)),
			Type:         typ,
			Meta:         json.RawMessage(b),
			Source:       "sendgrid",
			CreatedAt:    time.Unix(n.Timestamp, 0),
		}
		out = append(out, bn)
	}

	return out, nil
}

// verifyNotif verifies the signature on a notification payload.
func (s *Sendgrid) verifyNotif(sig, timestamp string, b []byte) error {
	sig = strings.TrimSpace(sig)

	sigLen := len(sig)
	tsLen := len(timestamp)

	sigB, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		// Also accept URL-safe/base64-without-padding variants from proxies.
		sigB, err = base64.RawStdEncoding.DecodeString(sig)
		if err != nil {
			sigB, err = base64.RawURLEncoding.DecodeString(sig)
			if err != nil {
				return fmt.Errorf("sendgrid signature base64 decode failed (sig_len=%d ts_len=%d): %w", sigLen, tsLen, err)
			}
		}
	}

	ecdsaSig := struct {
		R *big.Int
		S *big.Int
	}{}

	if _, err := asn1.Unmarshal(sigB, &ecdsaSig); err != nil {
		return fmt.Errorf("sendgrid signature ASN.1 unmarshal failed (sig_len=%d sigB_len=%d): %v", sigLen, len(sigB), err)
	}

	h := sha256.New()
	h.Write([]byte(timestamp))
	h.Write(b)
	hash := h.Sum(nil)

	if !ecdsa.Verify(s.pubKey, hash, ecdsaSig.R, ecdsaSig.S) {
		return errors.New("invalid signature")
	}

	return nil
}
