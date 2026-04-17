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
		return fmt.Errorf("quo send failed: %d %s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	return nil
}
