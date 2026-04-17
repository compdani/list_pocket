package quo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
)

const defaultAPIBase = "https://api.openphone.com"

// ErrUnsendableDestination is re-exported for callers that only import the
// quo package. Errors returned from SendText wrap models.ErrUnsendableDestination
// so errors.Is works against either value.
var ErrUnsendableDestination = models.ErrUnsendableDestination

// quoErrorResponse mirrors the Quo error envelope. See
// https://www.openphone.com/api/docs for the schema.
type quoErrorResponse struct {
	Code    string `json:"code"`
	Status  int    `json:"status"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// isUnsendableDestinationCode classifies provider error codes that indicate
// the destination phone is permanently not reachable for this account.
// Additional codes can be added here if we see them in the wild.
func isUnsendableDestinationCode(code string) bool {
	switch strings.TrimSpace(code) {
	case
		"0203400", // Unreachable `to` phone number (not SMS-reachable per provider).
		"0203403", // International Messaging Not Allowed.
		"0203404", // Phone number is not messageable (landline/VoIP).
		"0203405": // Destination carrier blocks our number (A2P filtering).
		return true
	}
	return false
}

// Client calls Quo / OpenPhone HTTP API.
type Client struct {
	APIKey  string
	From    string
	BaseURL string
	HTTP    *http.Client
}

func NewClientFromSettings(s models.TextMessagingSettings) (*Client, error) {
	p := s.QuoProvider()
	if p == nil || !p.Enabled {
		return nil, errors.New("quo provider is not enabled")
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, errors.New("missing quo api_key")
	}
	if strings.TrimSpace(p.From) == "" {
		return nil, errors.New("missing quo from")
	}
	return &Client{
		APIKey:  strings.TrimSpace(p.APIKey),
		From:    strings.TrimSpace(p.From),
		BaseURL: defaultAPIBase,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

type sendMessageRequest struct {
	Content string   `json:"content"`
	From    string   `json:"from"`
	To      []string `json:"to"`
}

// SendText sends a single SMS (one recipient).
func (c *Client) SendText(ctx context.Context, toE164 string, body []byte) error {
	if c == nil {
		return errors.New("nil client")
	}
	to := strings.TrimSpace(toE164)
	if to == "" {
		return errors.New("empty recipient")
	}
	payload := sendMessageRequest{
		Content: strings.TrimSpace(string(body)),
		From:    c.From,
		To:      []string{to},
	}
	if payload.Content == "" {
		return errors.New("empty message body")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode == http.StatusTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if d, err := strconv.Atoi(strings.TrimSpace(ra)); err == nil && d > 0 && d < 3600 {
				time.Sleep(time.Duration(d) * time.Second)
			}
		}
		return fmt.Errorf("quo rate limited: %s", strings.TrimSpace(string(rb)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
			var ee quoErrorResponse
			if jerr := json.Unmarshal(rb, &ee); jerr == nil && isUnsendableDestinationCode(ee.Code) {
				return fmt.Errorf("quo send failed: %d %s: %w",
					resp.StatusCode, strings.TrimSpace(string(rb)), ErrUnsendableDestination)
			}
		}
		return fmt.Errorf("quo send failed: %d %s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	return nil
}
