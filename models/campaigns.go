package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"strings"
	txttpl "text/template"

	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/jmoiron/sqlx/types"
	"github.com/lib/pq"
	null "gopkg.in/volatiletech/null.v6"
)

const (
	CampaignStatusDraft         = "draft"
	CampaignStatusScheduled     = "scheduled"
	CampaignStatusRunning       = "running"
	CampaignStatusPaused        = "paused"
	CampaignStatusFinished      = "finished"
	CampaignStatusCancelled     = "cancelled"
	CampaignTypeRegular         = "regular"
	CampaignTypeOptin           = "optin"
	CampaignContentTypeRichtext = "richtext"
	CampaignContentTypeHTML     = "html"
	CampaignContentTypeMarkdown = "markdown"
	CampaignContentTypePlain    = "plain"
	CampaignContentTypeVisual   = "visual"
	CampaignContentTypeGrapes   = "grapes_mjml"

	CampaignMessengerQuo = "quo"
)

// IsTextMessenger reports whether the campaign messenger uses SMS/text pipeline (ledger + consent + throttling).
func IsTextMessenger(messenger string) bool {
	return strings.TrimSpace(messenger) == CampaignMessengerQuo
}

// Campaigns represents a slice of Campaigns.
type Campaigns []Campaign

// Campaign represents an e-mail campaign.
type Campaign struct {
	Base
	CampaignMeta

	UUID              string          `db:"uuid" json:"uuid"`
	Type              string          `db:"type" json:"type"`
	Name              string          `db:"name" json:"name"`
	Subject           string          `db:"subject" json:"subject"`
	FromEmail         string          `db:"from_email" json:"from_email"`
	Body              string          `db:"body" json:"body"`
	BodySource        null.String     `db:"body_source" json:"body_source"`
	AltBody           null.String     `db:"altbody" json:"altbody"`
	SendAt            null.Time       `db:"send_at" json:"send_at"`
	Status            string          `db:"status" json:"status"`
	ContentType       string          `db:"content_type" json:"content_type"`
	Tags              pq.StringArray  `db:"tags" json:"tags"`
	IncludeTags       pq.StringArray  `db:"include_tags" json:"include_tags"`
	ExcludeTags       pq.StringArray  `db:"exclude_tags" json:"exclude_tags"`
	Headers           Headers         `db:"headers" json:"headers"`
	Attribs           JSON            `db:"attribs" json:"attribs"`
	TemplateID        null.String     `db:"template_id" json:"template_id"`
	Messenger         string          `db:"messenger" json:"messenger"`
	Archive           bool            `db:"archive" json:"archive"`
	ArchiveSlug       null.String     `db:"archive_slug" json:"archive_slug"`
	ArchiveTemplateID null.String     `db:"archive_template_id" json:"archive_template_id"`
	ArchiveMeta       json.RawMessage `db:"archive_meta" json:"archive_meta"`

	// TemplateBody is joined in from templates by the next-campaigns query.
	TemplateBody        string             `db:"template_body" json:"-"`
	ArchiveTemplateBody string             `db:"archive_template_body" json:"-"`
	Tpl                 *template.Template `json:"-"`
	TextTpl             *txttpl.Template   `json:"-"` // SMS / plain text campaigns (Quo)
	SubjectTpl          *txttpl.Template   `json:"-"`
	AltBodyTpl          *template.Template `json:"-"`

	// List of media (attachment) IDs obtained from the next-campaign query
	// while sending a campaign.
	MediaIDs pq.Int64Array `json:"-" db:"media_id"`

	// Fetched bodies of the attachments.
	Attachments []Attachment `json:"-" db:"-"`

	// Pseudofield for getting the total number of subscribers
	// in searches and queries.
	Total int `db:"total" json:"-"`
}

// CampaignMeta contains fields tracking a campaign's progress.
type CampaignMeta struct {
	CampaignID     int `db:"campaign_id" json:"-"`
	Views          int `db:"views" json:"views"`
	RawViews       int `db:"raw_views" json:"raw_views"`
	SuspectedViews int `db:"suspected_views" json:"suspected_views"`
	Clicks         int `db:"clicks" json:"clicks"`
	Bounces        int `db:"bounces" json:"bounces"`

	// This is a list of {list_id, name} pairs unlike Subscriber.Lists[]
	// because lists can be deleted after a campaign is finished, resulting
	// in null lists data to be returned. For that reason, campaign_lists maintains
	// campaign-list associations with a historical record of id + name that persist
	// even after a list is deleted.
	Lists types.JSONText `db:"lists" json:"lists"`
	Media types.JSONText `db:"media" json:"media"`

	StartedAt null.Time `db:"started_at" json:"started_at"`
	ToSend    int       `db:"to_send" json:"to_send"`
	Sent      int       `db:"sent" json:"sent"`
}

// GetIDs returns the list of campaign IDs.
func (camps Campaigns) GetIDs() []int {
	IDs := make([]int, len(camps))
	for i, c := range camps {
		IDs[i] = c.ID
	}

	return IDs
}

// LoadStats lazy loads campaign stats onto a list of campaigns.
func (camps Campaigns) LoadStats(stmt *pbdb.Query) error {
	var meta []CampaignMeta
	if err := stmt.Select(&meta, pq.Array(camps.GetIDs())); err != nil {
		return err
	}

	if len(camps) != len(meta) {
		return errors.New("campaign stats count does not match")
	}

	for i, c := range meta {
		if c.CampaignID == camps[i].ID {
			camps[i].Lists = c.Lists
			camps[i].Views = c.Views
			camps[i].Clicks = c.Clicks
			camps[i].Bounces = c.Bounces
			camps[i].Media = c.Media
		}
	}

	return nil
}

// CompileTemplate compiles a campaign body template into its base
// template and sets the resultant template to Campaign.Tpl.
func (c *Campaign) CompileTemplate(f template.FuncMap) error {
	if IsTextMessenger(c.Messenger) {
		return c.compileTextTemplate(f)
	}
	// If the subject line has a template string, compile it.
	if strings.Contains(c.Subject, "{{") {
		subj := c.Subject
		for _, r := range regTplFuncs {
			subj = r.regExp.ReplaceAllString(subj, r.replace)
		}

		var txtFuncs map[string]any = f
		subjTpl, err := txttpl.New(ContentTpl).Funcs(txtFuncs).Parse(subj)
		if err != nil {
			return fmt.Errorf("error compiling subject: %v", err)
		}
		c.SubjectTpl = subjTpl
	}

	// Compile the base template.
	body := c.TemplateBody

	if body == "" || c.ContentType == CampaignContentTypeVisual {
		body = `{{ template "content" . }}`
	}

	for _, r := range regTplFuncs {
		body = r.regExp.ReplaceAllString(body, r.replace)
	}

	baseTPL, err := template.New(BaseTpl).Funcs(f).Parse(body)
	if err != nil {
		return fmt.Errorf("error compiling base template: %v", err)
	}

	// If the format is markdown, convert Markdown to HTML.
	if c.ContentType == CampaignContentTypeMarkdown {
		var b bytes.Buffer
		if err := markdown.Convert([]byte(c.Body), &b); err != nil {
			return err
		}
		body = b.String()
	} else {
		body = c.Body
	}

	// Compile the campaign message.
	for _, r := range regTplFuncs {
		body = r.regExp.ReplaceAllString(body, r.replace)
	}

	msgTpl, err := template.New(ContentTpl).Funcs(f).Parse(body)
	if err != nil {
		return fmt.Errorf("error compiling message: %v", err)
	}

	out, err := baseTPL.AddParseTree(ContentTpl, msgTpl.Tree)
	if err != nil {
		return fmt.Errorf("error inserting child template: %v", err)
	}
	c.Tpl = out

	if strings.Contains(c.AltBody.String, "{{") {
		b := c.AltBody.String
		for _, r := range regTplFuncs {
			b = r.regExp.ReplaceAllString(b, r.replace)
		}
		bTpl, err := template.New(ContentTpl).Funcs(f).Parse(b)
		if err != nil {
			return fmt.Errorf("error compiling alt plaintext message: %v", err)
		}
		c.AltBodyTpl = bTpl
	}

	return nil
}

func (c *Campaign) compileTextTemplate(f template.FuncMap) error {
	txtFuncs := txttpl.FuncMap{}
	for k, v := range f {
		txtFuncs[k] = v
	}
	if strings.Contains(c.Subject, "{{") {
		subj := c.Subject
		for _, r := range regTplFuncs {
			subj = r.regExp.ReplaceAllString(subj, r.replace)
		}
		subjTpl, err := txttpl.New(ContentTpl).Funcs(txtFuncs).Parse(subj)
		if err != nil {
			return fmt.Errorf("error compiling subject: %v", err)
		}
		c.SubjectTpl = subjTpl
	}
	body := c.TemplateBody
	if body == "" {
		body = `{{ template "content" . }}`
	}
	for _, r := range regTplFuncs {
		body = r.regExp.ReplaceAllString(body, r.replace)
	}
	baseTPL, err := txttpl.New(BaseTpl).Funcs(txtFuncs).Parse(body)
	if err != nil {
		return fmt.Errorf("error compiling base template: %v", err)
	}
	msgBody := c.Body
	for _, r := range regTplFuncs {
		msgBody = r.regExp.ReplaceAllString(msgBody, r.replace)
	}
	msgTpl, err := txttpl.New(ContentTpl).Funcs(txtFuncs).Parse(msgBody)
	if err != nil {
		return fmt.Errorf("error compiling message: %v", err)
	}
	out, err := baseTPL.AddParseTree(ContentTpl, msgTpl.Tree)
	if err != nil {
		return fmt.Errorf("error inserting child template: %v", err)
	}
	c.TextTpl = out
	c.Tpl = nil
	c.AltBodyTpl = nil
	return nil
}

// ConvertContent converts a campaign's body from one format to another,
// for example, Markdown to HTML.
func (c *Campaign) ConvertContent(from, to string) (string, error) {
	body := c.Body
	for _, r := range regTplFuncs {
		body = r.regExp.ReplaceAllString(body, r.replace)
	}

	// If the format is markdown, convert Markdown to HTML.
	var out string
	if from == CampaignContentTypeMarkdown &&
		(to == CampaignContentTypeHTML || to == CampaignContentTypeRichtext) {
		var b bytes.Buffer
		if err := markdown.Convert([]byte(c.Body), &b); err != nil {
			return out, err
		}
		out = b.String()
	} else {
		return out, errors.New("unknown formats to convert")
	}

	return out, nil
}

func (c *Campaign) Preheader() string {
	if c == nil || c.Attribs == nil {
		return ""
	}

	v, ok := c.Attribs["preheader"]
	if !ok {
		return ""
	}

	s, ok := v.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(s)
}
