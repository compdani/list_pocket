package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxResponseBytes = 8 << 20

// cloudProvider is Meta's WhatsApp Cloud API.
type cloudProvider struct {
	token, base, phoneID, wabaID string
	http                         *http.Client
}

func (p *cloudProvider) Capabilities() Capabilities {
	return Capabilities{Templates: true, Window: true}
}

// do sends a request and decodes a success into out. An error body becomes *APIError.
func (p *cloudProvider) do(ctx context.Context, method, target string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
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

type cloudSendResponse struct {
	Contacts []struct {
		WaID string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

func (p *cloudProvider) send(ctx context.Context, payload map[string]any) (SendResult, error) {
	payload["messaging_product"] = "whatsapp"
	payload["recipient_type"] = "individual"
	var resp cloudSendResponse
	if err := p.do(ctx, http.MethodPost, p.base+"/"+url.PathEscape(p.phoneID)+"/messages", payload, &resp); err != nil {
		return SendResult{}, err
	}
	if len(resp.Messages) == 0 {
		return SendResult{}, errors.New("whatsapp: response has no message ID")
	}
	out := SendResult{MessageID: resp.Messages[0].ID}
	if len(resp.Contacts) > 0 {
		out.RecipientID = resp.Contacts[0].WaID
	}
	return out, nil
}

func (p *cloudProvider) SendText(ctx context.Context, to, body string) (SendResult, error) {
	return p.send(ctx, map[string]any{"to": to, "type": "text", "text": map[string]any{"body": body}})
}

func (p *cloudProvider) SendTemplate(ctx context.Context, to string, tpl TemplateSend) (SendResult, error) {
	template := map[string]any{"name": tpl.Name, "language": map[string]any{"code": tpl.Language}}
	if len(tpl.Components) > 0 {
		template["components"] = tpl.Components
	}
	return p.send(ctx, map[string]any{"to": to, "type": "template", "template": template})
}

// ListTemplates returns approved templates, following every page.
func (p *cloudProvider) ListTemplates(ctx context.Context) ([]Template, error) {
	if p.wabaID == "" {
		return nil, errors.New("whatsapp: business account ID is not set")
	}
	out := []Template{}
	next := p.base + "/" + url.PathEscape(p.wabaID) + "/message_templates?status=approved&limit=100"
	for next != "" {
		var page struct {
			Data   []Template `json:"data"`
			Paging struct {
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := p.do(ctx, http.MethodGet, next, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Data...)
		// Only follow a next link on our own API host, because the token goes with it.
		next = ""
		if strings.HasPrefix(page.Paging.Next, p.base+"/") {
			next = page.Paging.Next
		}
	}
	return out, nil
}

func (p *cloudProvider) ListChannels(context.Context) ([]Channel, error) {
	return nil, ErrUnsupported
}

func (p *cloudProvider) PostToChannel(context.Context, ChannelPost) (SendResult, error) {
	return SendResult{}, ErrUnsupported
}

func (p *cloudProvider) CheckConnection(ctx context.Context) error {
	if p.phoneID == "" {
		return fmt.Errorf("whatsapp: phone number ID is not set")
	}
	return p.do(ctx, http.MethodGet, p.base+"/"+url.PathEscape(p.phoneID)+"?fields=id", nil, nil)
}
