package txemail

import (
	"errors"
	"fmt"
	"log"
	"net/textproto"
	"strings"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/manager"
	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
)

type Request struct {
	TemplateID       string
	TemplateLegacyID int

	SubscriberID    string
	SubscriberEmail string
	SubscriberName  string
	FirstName       string
	LastName        string
	Phone           string
	Attribs         models.JSON

	Data        map[string]any
	FromEmail   string
	Headers     models.Headers
	ContentType string
	Messenger   string
	Subject     string
	Preheader   string
	ContentTpl  string
	Attachments []models.Attachment
}

type Sender struct {
	Core             *core.Core
	Manager          *manager.Manager
	DefaultFromEmail string
	ResolveFromEmail func(string) string
	Log              *log.Logger
}

func (s *Sender) Send(req Request) (models.TransactionalMessage, error) {
	if s.Core == nil || s.Manager == nil {
		return models.TransactionalMessage{}, fmt.Errorf("transactional sender is not initialized")
	}

	messengerName := strings.TrimSpace(req.Messenger)
	if messengerName == "" {
		messengerName = "email"
	}
	pluginChannel := strings.HasPrefix(messengerName, models.ChannelMessengerPrefix)

	email := strings.TrimSpace(req.SubscriberEmail)
	if email == "" && !pluginChannel {
		return models.TransactionalMessage{}, fmt.Errorf("subscriber email is required")
	}
	if models.IsPlaceholderEmail(email) && !pluginChannel {
		return models.TransactionalMessage{}, fmt.Errorf("subscriber has no email address")
	}

	// A plugin channel sends to the subscriber's own addresses, and only with consent.
	var channelTo []string
	if pluginChannel {
		stored, found, err := s.Core.GetSubscriberForChannel(strings.TrimSpace(req.SubscriberID))
		if err != nil {
			return models.TransactionalMessage{}, err
		}
		if !found {
			return models.TransactionalMessage{}, fmt.Errorf("channel %s needs an existing subscriber", messengerName)
		}
		channelTo, err = s.Core.ChannelRecipients(stored, messengerName)
		if err != nil {
			return models.TransactionalMessage{}, err
		}
		if len(channelTo) == 0 {
			return models.TransactionalMessage{}, fmt.Errorf("%w: %s", ErrNoChannelConsent, messengerName)
		}
	}

	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "html"
	}

	fromEmail := strings.TrimSpace(req.FromEmail)
	if fromEmail == "" && s.ResolveFromEmail != nil {
		fromEmail = strings.TrimSpace(s.ResolveFromEmail(messengerName))
	}
	if fromEmail == "" {
		fromEmail = s.DefaultFromEmail
	}
	if fromEmail == "" && !pluginChannel {
		return models.TransactionalMessage{}, fmt.Errorf("no from email configured for messenger %q", messengerName)
	}

	var (
		tpl                 models.Template
		err                 error
		usingInlineTemplate bool
	)
	switch {
	case req.TemplateLegacyID > 0:
		var tplRef *models.Template
		tplRef, err = s.Manager.GetTpl(req.TemplateLegacyID)
		if err == nil {
			tpl = *tplRef
		}
	case strings.TrimSpace(req.TemplateID) != "":
		tpl, err = s.Core.GetTemplate(strings.TrimSpace(req.TemplateID), false)
	default:
		usingInlineTemplate = true
		tpl = models.Template{
			Name:    "Workflow inline template",
			Type:    models.TemplateTypeTx,
			Subject: strings.TrimSpace(req.Subject),
			Body:    `{{ define "base" }}` + req.ContentTpl + `{{ end }}`,
		}
	}
	if err != nil {
		return models.TransactionalMessage{}, err
	}

	txUUID, err := uuid.NewV4()
	if err != nil {
		return models.TransactionalMessage{}, err
	}

	sub := models.Subscriber{
		Base:      models.Base{RecordID: strings.TrimSpace(req.SubscriberID)},
		Email:     email,
		Name:      strings.TrimSpace(req.SubscriberName),
		FirstName: strings.TrimSpace(req.FirstName),
		LastName:  strings.TrimSpace(req.LastName),
		Phone:     strings.TrimSpace(req.Phone),
		Attribs:   req.Attribs,
	}
	sub.NormalizeName()

	tx := models.TxMessage{
		Data:        req.Data,
		FromEmail:   fromEmail,
		Headers:     req.Headers,
		ContentType: contentType,
		Messenger:   messengerName,
		Subject:     req.Subject,
		Preheader:   req.Preheader,
		Attachments: req.Attachments,
	}

	record := models.TransactionalMessage{
		UUID:            txUUID.String(),
		SubscriberID:    sub.RecordID,
		SubscriberEmail: sub.Email,
		TemplateID:      tpl.RecordID,
		TemplateName:    tpl.Name,
		FromEmail:       fromEmail,
		ContentType:     contentType,
		Messenger:       messengerName,
		Status:          "queued",
		Data:            models.JSON(req.Data),
		Headers:         headersJSON(req.Headers),
	}

	renderTpl := tpl
	if !usingInlineTemplate && strings.TrimSpace(req.ContentTpl) != "" {
		renderTpl.Body = renderTpl.Body + `{{ define "content" }}` + req.ContentTpl + `{{ end }}`
	}
	if err := renderTpl.Compile(models.TxAliasTemplateFuncs(s.Manager.TxTemplateFuncs(&record), sub, &tx)); err != nil {
		return models.TransactionalMessage{}, err
	}
	if err := tx.Render(sub, &renderTpl); err != nil {
		return models.TransactionalMessage{}, err
	}

	record.Subject = tx.Subject
	record.Body = string(tx.Body)

	record, err = s.Manager.CreateTransactionalMessage(record)
	if err != nil {
		return models.TransactionalMessage{}, err
	}

	msg := models.Message{
		Subscriber:  sub,
		To:          recipients(channelTo, sub.Email),
		From:        fromEmail,
		Subject:     tx.Subject,
		ContentType: contentType,
		Messenger:   messengerName,
		Body:        tx.Body,
		TxMessage:   &record,
	}
	msg.Attachments = append(msg.Attachments, req.Attachments...)

	if len(req.Headers) != 0 {
		msg.Headers = make(textproto.MIMEHeader, len(req.Headers))
		for _, set := range req.Headers {
			for hdr, val := range set {
				msg.Headers.Add(hdr, val)
			}
		}
	}

	if err := s.Manager.PushMessage(msg); err != nil {
		if s.Log != nil {
			s.Log.Printf("error queueing transactional message %s: %v", record.RecordID, err)
		}
		_ = s.Manager.UpdateTransactionalMessageStatus(record.RecordID, "failed", err.Error(), false)
		return models.TransactionalMessage{}, err
	}

	return record, nil
}

func headersJSON(headers models.Headers) models.JSON {
	if len(headers) == 0 {
		return models.JSON{}
	}
	return models.JSON{"sets": headers}
}

// ErrNoChannelConsent means the subscriber hasn't agreed to a plugin channel, or
// has no address on it. Callers should report it as a bad request, not a failure.
var ErrNoChannelConsent = errors.New("subscriber has no consent or address on channel")

func recipients(channelTo []string, email string) []string {
	if len(channelTo) > 0 {
		return channelTo
	}
	return []string{email}
}
