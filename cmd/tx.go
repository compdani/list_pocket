package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/compdani/list_pocket/internal/manager"
	"github.com/compdani/list_pocket/internal/txemail"
	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
)

// SendTxMessage handles the sending of a transactional message.
func (a *App) SendTxMessage(c echo.Context) error {
	var m models.TxMessage

	// If it's a multipart form, there may be file attachments.
	if strings.HasPrefix(c.Request().Header.Get("Content-Type"), "multipart/form-data") {
		form, err := c.MultipartForm()
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.invalidFields", "name", err.Error()))
		}

		data, ok := form.Value["data"]
		if !ok || len(data) != 1 {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "data"))
		}

		// Parse the JSON data.
		if err := json.Unmarshal([]byte(data[0]), &m); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.invalidFields", "name", fmt.Sprintf("data: %s", err.Error())))
		}

		// Attach files.
		for _, f := range form.File["file"] {
			file, err := f.Open()
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError,
					a.i18n.Ts("globals.messages.invalidFields", "name", fmt.Sprintf("file: %s", err.Error())))
			}
			defer file.Close()

			b, err := io.ReadAll(file)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError,
					a.i18n.Ts("globals.messages.invalidFields", "name", fmt.Sprintf("file: %s", err.Error())))
			}

			m.Attachments = append(m.Attachments, models.Attachment{
				Name:    f.Filename,
				Header:  manager.MakeAttachmentHeader(f.Filename, "base64", f.Header.Get("Content-Type")),
				Content: b,
			})
		}

	} else if err := c.Bind(&m); err != nil {
		return err
	}

	// Validate fields.
	if r, err := a.validateTxMessage(m); err != nil {
		return err
	} else {
		m = r
	}

	var (
		num      = len(m.SubscriberEmails)
		isEmails = true
	)
	if len(m.SubscriberIDs) > 0 {
		num = len(m.SubscriberIDs)
		isEmails = false
	}

	notFound := []string{}
	for n := range num {
		var sub models.Subscriber

		if m.SubscriberMode == models.TxSubModeExternal {
			// `external`: Always create an ephemeral "subscriber" and don't
			// lookup in the DB.
			sub = models.Subscriber{
				Email: m.SubscriberEmails[n],
			}
		} else {
			// Default/fallback mode: lookup subscriber in DB.
			var (
				subID    int
				subEmail string
			)

			if !isEmails {
				subID = m.SubscriberIDs[n]
			} else {
				subEmail = m.SubscriberEmails[n]
			}

			var err error
			sub, err = a.core.GetSubscriber(subID, "", subEmail)
			if err != nil {
				if er, ok := err.(*echo.HTTPError); ok && er.Code == http.StatusBadRequest {
					// `fallback`: Create an ephemeral "subscriber" if the subscriber wasn't found.
					if m.SubscriberMode == models.TxSubModeFallback {
						sub = models.Subscriber{
							Email: subEmail,
						}
					} else {
						// `default`: log error and continue.
						notFound = append(notFound, fmt.Sprintf("%v", er.Message))
						continue
					}
				} else {
					return err
				}
			}
		}

		if _, err := a.newTransactionalSender().Send(txemail.Request{
			TemplateLegacyID: m.TemplateID,
			SubscriberID:     sub.RecordID,
			SubscriberEmail:  sub.Email,
			SubscriberName:   sub.Name,
			FirstName:        sub.FirstName,
			LastName:         sub.LastName,
			Phone:            sub.Phone,
			Attribs:          sub.Attribs,
			Data:             m.Data,
			FromEmail:        m.FromEmail,
			Headers:          m.Headers,
			ContentType:      m.ContentType,
			Messenger:        m.Messenger,
			Subject:          m.Subject,
			Attachments:      m.Attachments,
		}); err != nil {
			a.log.Printf("error sending transactional message (%s): %v", sub.Email, err)
			return err
		}
	}

	if len(notFound) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, strings.Join(notFound, "; "))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// validateTxMessage validates the tx message fields.
func (a *App) validateTxMessage(m models.TxMessage) (models.TxMessage, error) {
	if len(m.SubscriberEmails) > 0 && m.SubscriberEmail != "" {
		return m, echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.invalidFields", "name", "do not send `subscriber_email`"))
	}
	if len(m.SubscriberIDs) > 0 && m.SubscriberID != 0 {
		return m, echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.invalidFields", "name", "do not send `subscriber_id`"))
	}

	if m.SubscriberEmail != "" {
		m.SubscriberEmails = append(m.SubscriberEmails, m.SubscriberEmail)
	}

	if m.SubscriberID != 0 {
		m.SubscriberIDs = append(m.SubscriberIDs, m.SubscriberID)
	}

	// Validate subscriber_mode.
	if m.SubscriberMode == "" {
		m.SubscriberMode = models.TxSubModeDefault
	}

	switch m.SubscriberMode {
	case models.TxSubModeDefault:
		// Need subscriber_emails OR subscriber_ids, but not both.
		if (len(m.SubscriberEmails) == 0 && len(m.SubscriberIDs) == 0) || (len(m.SubscriberEmails) > 0 && len(m.SubscriberIDs) > 0) {
			return m, echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.invalidFields", "name", "send subscriber_emails OR subscriber_ids"))
		}
	case models.TxSubModeFallback, models.TxSubModeExternal:
		// `fallback` and `external` can only use subscriber_emails.
		if len(m.SubscriberIDs) > 0 {
			return m, echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.invalidFields", "name", "subscriber_ids not allowed in fallback or external mode"))
		}
		if len(m.SubscriberEmails) == 0 {
			return m, echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.invalidFields", "name", "subscriber_emails"))
		}
	default:
		return m, echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.invalidFields", "name", "subscriber_mode"))
	}

	for n, email := range m.SubscriberEmails {
		if email != "" {
			em, err := a.importer.SanitizeEmail(email)
			if err != nil {
				return m, echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			m.SubscriberEmails[n] = em
		}
	}

	if m.FromEmail == "" {
		m.FromEmail = a.cfg.FromEmail
	}

	if m.Messenger == "" {
		m.Messenger = emailMsgr
	} else if !a.manager.HasMessenger(m.Messenger) {
		return m, echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("campaigns.fieldInvalidMessenger", "name", m.Messenger))
	}

	return m, nil
}
