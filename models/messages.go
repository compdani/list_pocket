package models

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"net/textproto"
	"strings"
	txttpl "text/template"

	null "gopkg.in/volatiletech/null.v6"
)

// ErrUnsendableDestination signals that a messenger refused the recipient
// permanently (e.g. Quo "International Messaging Not Allowed"). Callers
// should treat it as a provider-level STOP for that phone/email and take
// the subscriber out of rotation rather than retry.
var ErrUnsendableDestination = errors.New("destination not sendable")

// Message is the message pushed to a Messenger.
type Message struct {
	From        string
	To          []string
	Subject     string
	ContentType string
	Body        []byte
	AltBody     []byte
	Headers     textproto.MIMEHeader
	Attachments []Attachment

	Subscriber Subscriber

	// Campaign is generally the same instance for a large number of subscribers.
	Campaign *Campaign

	// Messenger is the messenger backend to use: email|postback.
	Messenger string

	// TxMessage is set for persisted transactional sends.
	TxMessage *TransactionalMessage
}

// Attachment represents a file or blob attachment that can be
// sent along with a message by a Messenger.
type Attachment struct {
	Name    string
	Header  textproto.MIMEHeader
	Content []byte
}

// TxMessage subscriber modes.
const (
	TxSubModeDefault  = "default"
	TxSubModeFallback = "fallback"
	TxSubModeExternal = "external"
)

// TxMessage represents an e-mail campaign.
type TxMessage struct {
	SubscriberMode   string   `json:"subscriber_mode"`
	SubscriberEmails []string `json:"subscriber_emails"`
	SubscriberIDs    []int    `json:"subscriber_ids"`

	// Deprecated.
	SubscriberEmail string `json:"subscriber_email"`
	SubscriberID    int    `json:"subscriber_id"`

	TemplateID  int            `json:"template_id"`
	Data        map[string]any `json:"data"`
	FromEmail   string         `json:"from_email"`
	Headers     Headers        `json:"headers"`
	ContentType string         `json:"content_type"`
	Messenger   string         `json:"messenger"`
	Subject     string         `json:"subject"`
	Preheader   string         `json:"preheader"`

	// File attachments added from multi-part form data.
	Attachments []Attachment `json:"-"`

	Body       []byte             `json:"-"`
	Tpl        *template.Template `json:"-"`
	SubjectTpl *txttpl.Template   `json:"-"`
}

type TransactionalMessage struct {
	Base

	UUID string `db:"uuid" json:"uuid"`

	SubscriberID    string                  `db:"subscriber_record_id" json:"subscriber_id,omitempty"`
	SubscriberEmail string                  `db:"subscriber_email" json:"subscriber_email"`
	TemplateID      string                  `db:"template_record_id" json:"template_id,omitempty"`
	TemplateName    string                  `db:"template_name" json:"template_name,omitempty"`
	FromEmail       string                  `db:"from_email" json:"from_email"`
	Subject         string                  `db:"subject" json:"subject"`
	ContentType     string                  `db:"content_type" json:"content_type"`
	Messenger       string                  `db:"messenger" json:"messenger"`
	Status          string                  `db:"status" json:"status"`
	Error           string                  `db:"error" json:"error,omitempty"`
	Body            string                  `db:"body" json:"body,omitempty"`
	Data            JSON                    `db:"data" json:"data,omitempty"`
	Headers         JSON                    `db:"headers" json:"headers,omitempty"`
	Views           int                     `db:"views" json:"views"`
	RawViews        int                     `db:"raw_views" json:"raw_views"`
	SuspectedViews  int                     `db:"suspected_views" json:"suspected_views"`
	Clicks          int                     `db:"clicks" json:"clicks"`
	SentAt          null.Time               `db:"sent_at" json:"sent_at,omitempty"`
	LinkStats       []TransactionalLinkStat `db:"-" json:"link_stats,omitempty"`
}

type TransactionalLinkStat struct {
	URL   string `db:"url" json:"url"`
	Count int    `db:"count" json:"count"`
}

func (m *TxMessage) Render(sub Subscriber, tpl *Template) error {
	data := struct {
		Subscriber Subscriber
		Tx         *TxMessage
	}{sub, m}

	// Render the body.
	b := bytes.Buffer{}
	if err := tpl.Tpl.ExecuteTemplate(&b, BaseTpl, data); err != nil {
		return err
	}
	m.Body = make([]byte, b.Len())
	copy(m.Body, b.Bytes())
	m.Body = ApplyPreheaderToHTML(m.Body, m.ContentType, m.Preheader)
	b.Reset()

	// Was a subject provided in the message?
	var (
		subjTpl *txttpl.Template
		subject = m.Subject
	)
	if subject != "" {
		if strings.Contains(m.Subject, "{{") {
			// If the subject has a template string, render that.
			s, err := txttpl.New(BaseTpl).Funcs(TxAliasTextFuncs(txttpl.FuncMap{}, sub, m)).Parse(m.Subject)
			if err != nil {
				return fmt.Errorf("error compiling subject: %v", err)
			}
			subjTpl = s
		}
	} else {
		// Use the subject from the template.
		subject = tpl.Subject
		subjTpl = tpl.SubjectTpl
	}

	// If the subject is also a template, render that.
	if subjTpl != nil {
		if err := subjTpl.ExecuteTemplate(&b, BaseTpl, data); err != nil {
			return err
		}
		m.Subject = b.String()
		b.Reset()
	} else {
		m.Subject = subject
	}

	return nil
}
