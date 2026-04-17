package manager

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/compdani/list_pocket/models"
)

// NewCampaignMessage creates and returns a CampaignMessage that is made available
// to message templates while they're compiled. It represents a message from
// a campaign that's bound to a single Subscriber.
func (m *Manager) NewCampaignMessage(c *models.Campaign, s models.Subscriber) (CampaignMessage, error) {
	msg := CampaignMessage{
		Campaign:   c,
		Subscriber: s,

		subject:  c.Subject,
		from:     c.FromEmail,
		to:       s.Email,
		unsubURL: fmt.Sprintf(m.cfg.UnsubURL, c.RecordID, s.RecordID),
	}
	if models.IsTextMessenger(c.Messenger) {
		msg.to = strings.TrimSpace(s.Phone)
		// For SMS, campaigns.from_email doubles as the sender phone override.
		// Leave blank here if it doesn't look like a phone (e.g. legacy email
		// value from converted campaigns); the messenger falls back to the
		// provider default when msg.From is empty.
		msg.from = strings.TrimSpace(c.FromEmail)
		if msg.from != "" && strings.Contains(msg.from, "@") {
			msg.from = ""
		}
	}

	if err := msg.render(); err != nil {
		return msg, err
	}

	return msg, nil
}

// render takes a Message, executes its pre-compiled Campaign.Tpl
// and applies the resultant bytes to Message.body to be used in messages.
func (m *CampaignMessage) render() error {
	out := bytes.Buffer{}

	// Render the subject if it's a template.
	if m.Campaign.SubjectTpl != nil {
		if err := m.Campaign.SubjectTpl.ExecuteTemplate(&out, models.ContentTpl, m); err != nil {
			return err
		}
		m.subject = out.String()
		out.Reset()
	}

	// Compile the main template.
	if m.Campaign.TextTpl != nil {
		if err := m.Campaign.TextTpl.ExecuteTemplate(&out, models.BaseTpl, m); err != nil {
			return err
		}
	} else if m.Campaign.Tpl != nil {
		if err := m.Campaign.Tpl.ExecuteTemplate(&out, models.BaseTpl, m); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("campaign has no compiled template")
	}
	m.body = models.ApplyPreheaderToHTML(out.Bytes(), m.Campaign.ContentType, m.Campaign.Preheader())

	// Is there an alt body?
	if m.Campaign.ContentType != models.CampaignContentTypePlain && m.Campaign.AltBody.Valid {
		if m.Campaign.AltBodyTpl != nil {
			b := bytes.Buffer{}
			if err := m.Campaign.AltBodyTpl.ExecuteTemplate(&b, models.ContentTpl, m); err != nil {
				return err
			}
			m.altBody = b.Bytes()
		} else {
			m.altBody = []byte(m.Campaign.AltBody.String)
		}
	}

	return nil
}

// Subject returns a copy of the message subject
func (m *CampaignMessage) Subject() string {
	return m.subject
}

// Body returns a copy of the message body.
func (m *CampaignMessage) Body() []byte {
	out := make([]byte, len(m.body))
	copy(out, m.body)
	return out
}

// AltBody returns a copy of the message's alt body.
func (m *CampaignMessage) AltBody() []byte {
	out := make([]byte, len(m.altBody))
	copy(out, m.altBody)
	return out
}
