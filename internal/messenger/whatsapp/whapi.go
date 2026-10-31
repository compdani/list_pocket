package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// whapiProvider is Whapi.Cloud, which drives a phone linked by QR code. It isn't an
// official WhatsApp API. It has no templates or service window, and it's the only
// provider that can post to WhatsApp Channels.
type whapiProvider struct {
	token, base string
	http        *http.Client
}

func (p *whapiProvider) Capabilities() Capabilities {
	return Capabilities{ChannelPosts: true}
}

func (p *whapiProvider) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var parsed struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &parsed)
		return &APIError{HTTPStatus: resp.StatusCode, Code: parsed.Error.Code, Message: parsed.Error.Message}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// whapiSendResponse allows for both places Whapi's docs show the message ID.
type whapiSendResponse struct {
	Sent    bool `json:"sent"`
	Message struct {
		ID string `json:"id"`
	} `json:"message"`
	SentMessage struct {
		ID string `json:"id"`
	} `json:"sent_message"`
}

func (p *whapiProvider) post(ctx context.Context, path string, payload map[string]any) (SendResult, error) {
	var resp whapiSendResponse
	if err := p.do(ctx, http.MethodPost, path, payload, &resp); err != nil {
		return SendResult{}, err
	}
	id := resp.Message.ID
	if id == "" {
		id = resp.SentMessage.ID
	}
	if id == "" {
		return SendResult{}, errors.New("whatsapp: response has no message ID")
	}
	return SendResult{MessageID: id}, nil
}

// SendText sends to bare digits, because Whapi's recipient format has no plus sign.
func (p *whapiProvider) SendText(ctx context.Context, to, body string) (SendResult, error) {
	return p.post(ctx, "/messages/text", map[string]any{"to": digits(to), "body": body})
}

func (p *whapiProvider) SendTemplate(context.Context, string, TemplateSend) (SendResult, error) {
	return SendResult{}, ErrUnsupported
}

func (p *whapiProvider) ListTemplates(context.Context) ([]Template, error) {
	return nil, ErrUnsupported
}

func (p *whapiProvider) ListChannels(ctx context.Context) ([]Channel, error) {
	var resp struct {
		Newsletters []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"newsletters"`
	}
	if err := p.do(ctx, http.MethodGet, "/newsletters", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]Channel, 0, len(resp.Newsletters))
	for _, n := range resp.Newsletters {
		out = append(out, Channel{ID: n.ID, Name: n.Name})
	}
	return out, nil
}

func (p *whapiProvider) PostToChannel(ctx context.Context, post ChannelPost) (SendResult, error) {
	switch post.Kind {
	case PostText:
		return p.post(ctx, "/messages/text", map[string]any{"to": post.ChannelID, "body": post.Body})
	case PostImage:
		return p.post(ctx, "/messages/image", map[string]any{"to": post.ChannelID, "media": post.MediaURL, "caption": post.Body})
	case PostVideo:
		return p.post(ctx, "/messages/video", map[string]any{"to": post.ChannelID, "media": post.MediaURL, "caption": post.Body})
	case PostLink:
		return p.post(ctx, "/messages/link_preview", map[string]any{"to": post.ChannelID, "body": post.Body, "title": post.Title})
	}
	return SendResult{}, fmt.Errorf("whatsapp: unknown post kind %q", post.Kind)
}

// whapiStatusAuth is the only health status where the phone is linked and working.
const whapiStatusAuth = "AUTH"

// whapiStatusNote explains a health status that isn't AUTH.
func whapiStatusNote(status string) string {
	switch status {
	case "QR":
		return "The phone isn't linked. Scan the QR code in Whapi."
	case "STOP":
		return "The Whapi channel is deactivated. Check its subscription in Whapi."
	case "SYNC_ERROR":
		return "Whapi can't sync with the phone. Sends still work, but channel lists don't. Log out of the channel in Whapi and link the phone again."
	case "INIT", "LAUNCH":
		return "The Whapi channel is still starting. Try again in a minute."
	}
	return fmt.Sprintf("Whapi reports status %q instead of a linked phone.", status)
}

// CheckConnection needs the status, because a token can be valid with no phone linked.
func (p *whapiProvider) CheckConnection(ctx context.Context) error {
	var health struct {
		Status struct {
			Text string `json:"text"`
		} `json:"status"`
	}
	if err := p.do(ctx, http.MethodGet, "/health", nil, &health); err != nil {
		return err
	}
	if health.Status.Text != whapiStatusAuth {
		return errors.New(whapiStatusNote(health.Status.Text))
	}
	return nil
}
