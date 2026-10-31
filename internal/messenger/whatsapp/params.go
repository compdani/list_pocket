package whatsapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/compdani/list_pocket/models"
)

// CampaignFieldsAttrib is the campaign attribs key holding channel options, and
// TxTemplateKey is where a transactional message's data names its template.
const (
	CampaignFieldsAttrib = "channel_fields"
	TxTemplateKey        = "whatsapp_template"
)

// TemplateChoice is what an admin picked and filled in for a template.
type TemplateChoice struct {
	Name       string            `json:"name"`
	Language   string            `json:"language"`
	Components []ComponentChoice `json:"components"`
}

// ComponentChoice is the filled-in values of one template component.
type ComponentChoice struct {
	Type string `json:"type"`
	// SubType and Index identify a button.
	SubType string `json:"sub_type,omitempty"`
	Index   string `json:"index,omitempty"`
	// Format is image, video or document for a media header, and Link is its URL.
	Format     string        `json:"format,omitempty"`
	Link       string        `json:"link,omitempty"`
	Parameters []ParamChoice `json:"parameters,omitempty"`
}

// ParamChoice is one template variable. Value can contain placeholders, and Fallback
// is used when they render empty. ParameterName is set for named-format templates.
type ParamChoice struct {
	ParameterName string `json:"parameter_name,omitempty"`
	Value         string `json:"value"`
	Fallback      string `json:"fallback"`
}

// TemplateSend is a template message ready for the API.
type TemplateSend struct {
	Name       string
	Language   string
	Components []SendComponent
}

// SendComponent is the API's shape for a filled-in template component.
type SendComponent struct {
	Type       string          `json:"type"`
	SubType    string          `json:"sub_type,omitempty"`
	Index      string          `json:"index,omitempty"`
	Parameters []SendParameter `json:"parameters,omitempty"`
}

// SendParameter is one rendered value. Media holds a media header's link.
type SendParameter struct {
	Type          string     `json:"type"`
	ParameterName string     `json:"parameter_name,omitempty"`
	Text          string     `json:"text,omitempty"`
	Image         *MediaLink `json:"image,omitempty"`
	Video         *MediaLink `json:"video,omitempty"`
	Document      *MediaLink `json:"document,omitempty"`
}

// MediaLink points at media by URL.
type MediaLink struct {
	Link string `json:"link"`
}

// ErrNoTemplate means a campaign or message reached a templates-only provider
// without a template.
var ErrNoTemplate = errors.New("whatsapp: no template selected")

var rePlaceholder = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*\}\}`)

// renderValue fills a value's placeholders from the subscriber. Unknown placeholders
// render empty, so the fallback applies. An empty result is an error because the API
// rejects empty parameters.
func renderValue(p ParamChoice, sub models.Subscriber) (string, error) {
	out := rePlaceholder.ReplaceAllStringFunc(p.Value, func(match string) string {
		key := rePlaceholder.FindStringSubmatch(match)[1]
		switch {
		case key == "name":
			return sub.Name
		case key == "first_name":
			return sub.FirstName
		case key == "last_name":
			return sub.LastName
		case strings.HasPrefix(key, "attribs."):
			if v, ok := sub.Attribs[strings.TrimPrefix(key, "attribs.")]; ok && v != nil {
				return fmt.Sprint(v)
			}
		}
		return ""
	})
	out = strings.TrimSpace(out)
	if out == "" {
		out = strings.TrimSpace(p.Fallback)
	}
	if out == "" {
		return "", fmt.Errorf("whatsapp: template value %q is empty for this subscriber and has no fallback", p.Value)
	}
	return out, nil
}

// BuildTemplate renders a choice for one subscriber.
func BuildTemplate(c TemplateChoice, sub models.Subscriber) (TemplateSend, error) {
	if strings.TrimSpace(c.Name) == "" {
		return TemplateSend{}, ErrNoTemplate
	}
	language := strings.TrimSpace(c.Language)
	if language == "" {
		return TemplateSend{}, fmt.Errorf("whatsapp: template %q has no language", c.Name)
	}
	out := TemplateSend{Name: c.Name, Language: language}
	for _, comp := range c.Components {
		send := SendComponent{Type: comp.Type, SubType: comp.SubType, Index: comp.Index}
		if comp.Link != "" {
			param, err := mediaParameter(comp.Format, comp.Link)
			if err != nil {
				return TemplateSend{}, err
			}
			send.Parameters = append(send.Parameters, param)
		}
		for _, p := range comp.Parameters {
			text, err := renderValue(p, sub)
			if err != nil {
				return TemplateSend{}, err
			}
			send.Parameters = append(send.Parameters, SendParameter{Type: "text", ParameterName: p.ParameterName, Text: text})
		}
		if len(send.Parameters) > 0 {
			out.Components = append(out.Components, send)
		}
	}
	return out, nil
}

func mediaParameter(format, link string) (SendParameter, error) {
	media := &MediaLink{Link: link}
	switch strings.ToLower(format) {
	case "image":
		return SendParameter{Type: "image", Image: media}, nil
	case "video":
		return SendParameter{Type: "video", Video: media}, nil
	case "document":
		return SendParameter{Type: "document", Document: media}, nil
	}
	return SendParameter{}, fmt.Errorf("whatsapp: unsupported media header %q", format)
}

// ChoiceFromMessage finds the template for a message: a campaign's channel fields,
// or a transactional message's data.
func ChoiceFromMessage(msg models.Message) (TemplateChoice, error) {
	var raw any
	switch {
	case msg.Campaign != nil:
		if fields, ok := msg.Campaign.Attribs[CampaignFieldsAttrib].(map[string]any); ok {
			raw = fields["template"]
		}
	case msg.TxMessage != nil:
		raw = msg.TxMessage.Data[TxTemplateKey]
	}
	if raw == nil {
		return TemplateChoice{}, ErrNoTemplate
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return TemplateChoice{}, err
	}
	var out TemplateChoice
	if err := json.Unmarshal(encoded, &out); err != nil {
		return TemplateChoice{}, fmt.Errorf("whatsapp: invalid template selection: %w", err)
	}
	return out, nil
}
