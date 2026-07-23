package main

import (
	"encoding/json"
	"errors"
	"github.com/compdani/list_pocket/internal/apperr"
	pbcore "github.com/pocketbase/pocketbase/core"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/bounce/webhooks"
	"github.com/compdani/list_pocket/models"
)

// GetBounce handles retrieval of a specific bounce record by ID.
func (a *App) GetBounce(re *pbcore.RequestEvent) error {
	// Fetch one bounce from the DB.
	id := strings.TrimSpace(pathParam(re, "id"))
	out, err := a.core.GetBounce(id)
	if err != nil {
		return err
	}

	return okJSON(re, out)
}

// GetBounces handles retrieval of bounce records.
func (a *App) GetBounces(re *pbcore.RequestEvent) error {
	var (
		source  = re.Request.FormValue("source")
		orderBy = re.Request.FormValue("order_by")
		order   = re.Request.FormValue("order")

		pg = a.pg.NewFromURL(re.Request.URL.Query())
	)
	campIDs, err := a.core.ResolveCampaignIDs(nil, getQueryStrings("campaign_id", re.Request.URL.Query()))
	if err != nil {
		return apperr.BadRequest(a.i18n.T("globals.messages.invalidID"))
	}
	campID := 0
	if len(campIDs) > 0 {
		campID = campIDs[0]
	}

	// Query and fetch bounces from the DB.
	res, total, err := a.core.QueryBounces(campID, 0, source, orderBy, order, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}

	// No results.
	if len(res) == 0 {
		return okJSON(re, models.PageResults{Results: []models.Bounce{}})
	}

	out := models.PageResults{
		Results: res,
		Total:   total,
		Page:    pg.Page,
		PerPage: pg.PerPage,
	}

	return okJSON(re, out)
}

// GetSubscriberBounces retrieves a subscriber's bounce records.
func (a *App) GetSubscriberBounces(re *pbcore.RequestEvent) error {
	subID, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}

	// Query and fetch bounces from the DB.
	out, _, err := a.core.QueryBounces(0, subID, "", "", "", 0, 1000)
	if err != nil {
		return err
	}

	return okJSON(re, out)
}

// DeleteBounces handles bounce deletion of a list.
func (a *App) DeleteBounces(re *pbcore.RequestEvent) error {
	all, _ := strconv.ParseBool(queryParam(re, "all"))

	var ids []string
	if !all {
		ids = getQueryStrings("id", re.Request.URL.Query())
		if len(ids) == 0 {
			return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidID"))
		}
	}

	// Delete bounces from the DB.
	if err := a.core.DeleteBounces(ids, all); err != nil {
		return err
	}

	return okJSON(re, true)
}

// DeleteBounce handles bounce deletion of a single bounce record.
func (a *App) DeleteBounce(re *pbcore.RequestEvent) error {
	// Delete bounces from the DB.
	id := strings.TrimSpace(pathParam(re, "id"))
	if err := a.core.DeleteBounces([]string{id}, false); err != nil {
		return err
	}

	return okJSON(re, true)
}

// BlocklistBouncedSubscribers handles blocklisting of all bounced subscribers.
func (a *App) BlocklistBouncedSubscribers(re *pbcore.RequestEvent) error {
	if err := a.core.BlocklistBouncedSubscribers(); err != nil {
		return err
	}

	return okJSON(re, true)
}

// BounceWebhook renders the HTML preview of a template.
func (a *App) BounceWebhook(re *pbcore.RequestEvent) error {
	// Read the request body instead of using bindJSON(re, ) to read to save the entire raw request as meta.
	rawReq, err := io.ReadAll(re.Request.Body)
	if err != nil {
		a.log.Printf("error reading ses notification body: %v", err)
		return apperr.BadRequest(a.i18n.Ts("globals.messages.internalError"))
	}

	var (
		service = pathParam(re, "service")

		bounces []models.Bounce
	)
	switch true {
	// Native internal webhook.
	case service == "":
		var b models.Bounce
		if err := json.Unmarshal(rawReq, &b); err != nil {
			return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidData") + ":" + err.Error())
		}

		if bv, err := a.validateBounceFields(b); err != nil {
			return err
		} else {
			b = bv
		}

		if len(b.Meta) == 0 {
			b.Meta = json.RawMessage("{}")
		}

		if b.CreatedAt.Year() == 0 {
			b.CreatedAt = time.Now()
		}

		bounces = append(bounces, b)

	// Amazon SES.
	case service == "ses" && a.cfg.BounceSESEnabled:
		switch re.Request.Header.Get("X-Amz-Sns-Message-Type") {
		// SNS webhook registration confirmation. Only after these are processed will the endpoint
		// start getting bounce notifications.
		case "SubscriptionConfirmation", "UnsubscribeConfirmation":
			if err := a.bounce.SES.ProcessSubscription(rawReq); err != nil {
				a.log.Printf("error processing SNS (SES) subscription: %v", err)
				return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
			}

		// Bounce notification.
		case "Notification":
			if strings.EqualFold(parseSESNotificationType(rawReq), "Received") {
				id, err := a.processInboundEmailReplyWebhookBody(re, rawReq)
				if err != nil {
					a.log.Printf("ses service webhook: inbound email processing failed err=%v", err)
					return err
				}
				a.log.Printf("ses service webhook: inbound email processed id=%q", id)
				return okJSON(re, true)
			}

			b, err := a.bounce.SES.ProcessBounce(rawReq)
			if err != nil {
				if errors.Is(err, webhooks.ErrNotificationNotBounce) {
					return okJSON(re, true)
				}
				a.log.Printf("error processing SES notification: %v", err)
				return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
			}
			bounces = append(bounces, b)

		default:
			return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
		}

	// SendGrid.
	case service == "sendgrid" && a.cfg.BounceSendgridEnabled:
		var (
			sig = re.Request.Header.Get("X-Twilio-Email-Event-Webhook-Signature")
			ts  = re.Request.Header.Get("X-Twilio-Email-Event-Webhook-Timestamp")
		)

		// Sendgrid sends multiple bounces.
		bs, err := a.bounce.Sendgrid.ProcessBounce(sig, ts, rawReq)
		if err != nil {
			a.log.Printf("error processing sendgrid notification: %v", err)
			return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
		}
		bounces = append(bounces, bs...)

	// Postmark.
	case service == "postmark" && a.cfg.BouncePostmarkEnabled:
		bs, err := a.bounce.Postmark.ProcessBounce(rawReq, re.Request)
		if err != nil {
			a.log.Printf("error processing postmark notification: %v", err)
			if _, _, ok := asHTTPError(err); ok {
				return err
			}

			return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
		}
		bounces = append(bounces, bs...)

	// ForwardEmail.
	case service == "forwardemail" && a.cfg.BounceForwardemailEnabled:
		var (
			sig = re.Request.Header.Get("X-Webhook-Signature")
		)

		bs, err := a.bounce.Forwardemail.ProcessBounce(sig, rawReq)
		if err != nil {
			a.log.Printf("error processing forwardemail notification: %v", err)
			if _, _, ok := asHTTPError(err); ok {
				return err
			}

			return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
		}
		bounces = append(bounces, bs...)

	// Brevo (Sendinblue) transactional webhooks — Bearer token must match settings.
	case service == "brevo" && a.cfg.BounceBrevoEnabled:
		if a.bounce.Brevo == nil {
			return apperr.BadRequest(a.i18n.Ts("bounces.unknownService"))
		}
		authz := re.Request.Header.Get("Authorization")
		bs, err := a.bounce.Brevo.ProcessBounce(authz, rawReq)
		if err != nil {
			a.log.Printf("error processing brevo notification: %v", err)
			return apperr.BadRequest(a.i18n.Ts("globals.messages.invalidData"))
		}
		bounces = append(bounces, bs...)

	default:
		return apperr.BadRequest(a.i18n.Ts("bounces.unknownService"))
	}

	// Insert bounces into the DB.
	for _, b := range bounces {
		if err := a.bounce.Record(b); err != nil {
			a.log.Printf("error recording bounce: %v", err)
		}
	}

	return okJSON(re, true)
}

func (a *App) validateBounceFields(b models.Bounce) (models.Bounce, error) {
	if b.Email == "" && b.SubscriberUUID == "" {
		return b, apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "email / subscriber_uuid"))
	}

	if b.SubscriberUUID != "" && !reUUID.MatchString(b.SubscriberUUID) {
		return b, apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "subscriber_uuid"))
	}

	if b.Email != "" {
		em, err := a.importer.SanitizeEmail(b.Email)
		if err != nil {
			return b, apperr.BadRequest(err.Error())
		}
		b.Email = em
	}

	if b.Type != models.BounceTypeHard && b.Type != models.BounceTypeSoft && b.Type != models.BounceTypeComplaint {
		return b, apperr.BadRequest(a.i18n.Ts("globals.messages.invalidFields", "name", "type"))
	}

	return b, nil
}
