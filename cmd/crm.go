package main

import (
	"net/http"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/messenger/whatsapp"
	"github.com/compdani/list_pocket/models"
	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

const (
	maxConversationReplyChars = 10000
	// The design editors write long HTML, so a formatted reply's source gets more room.
	maxConversationReplySourceChars = 1000000

	quoMessenger = "quo"
)

// Reasons a conversation can't be replied to on a channel, which the reply box explains.
const (
	replyBlockedBlocklisted   = "blocklisted"
	replyBlockedNoEmail       = "no_email"
	replyBlockedNoContact     = "no_contact"
	replyBlockedUnsupported   = "unsupported"
	replyBlockedNotConfigured = "not_configured"
)

// contactFromRoute resolves the {id} in a contact route to a record ID, after checking
// that the user can reach that subscriber through their lists.
func (a *App) contactFromRoute(re *pbcore.RequestEvent) (string, error) {
	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return "", err
	}
	if err := a.hasSubPerm(auth.GetUserRE(re), []int{id}); err != nil {
		return "", err
	}
	sub, err := a.core.GetSubscriber(id, "", "")
	if err != nil {
		return "", err
	}
	return sub.RecordID, nil
}

// GetContactProfile returns a contact's CRM fields.
func (a *App) GetContactProfile(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	out, err := a.core.GetContactProfile(subID)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, out)
}

// UpdateContactProfile changes a contact's owner or company.
func (a *App) UpdateContactProfile(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	var in core.ContactProfileUpdate
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.core.UpdateContactProfile(subID, in)
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// GetCRMAssignees lists the people contacts and tasks can be assigned to.
func (a *App) GetCRMAssignees(re *pbcore.RequestEvent) error {
	out, err := a.core.ListCRMAssignees()
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

func userDisplayName(user auth.User) string {
	if user.Name != "" {
		return user.Name
	}
	return user.Username
}

func int64PathParam(re *pbcore.RequestEvent, name string) (int64, error) {
	id, err := strconv.ParseInt(pathParam(re, name), 10, 64)
	if err != nil || id < 1 {
		return 0, apperr.BadRequest("invalid ID")
	}
	return id, nil
}

// GetContactNotes lists a contact's notes.
func (a *App) GetContactNotes(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	out, err := a.core.ListContactNotes(subID)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// CreateContactNote adds a note to a contact.
func (a *App) CreateContactNote(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	user := auth.GetUserRE(re)
	out, err := a.core.AddContactNote(subID, in.Body, user.RecordID, userDisplayName(user))
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// UpdateContactNote edits or pins a note.
func (a *App) UpdateContactNote(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	noteID, err := int64PathParam(re, "noteId")
	if err != nil {
		return err
	}
	var in struct {
		Body   *string `json:"body"`
		Pinned *bool   `json:"pinned"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.core.UpdateContactNote(subID, noteID, in.Body, in.Pinned)
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// DeleteContactNote removes a note.
func (a *App) DeleteContactNote(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	noteID, err := int64PathParam(re, "noteId")
	if err != nil {
		return err
	}
	if err := a.core.DeleteContactNote(subID, noteID); err != nil {
		return err
	}
	return okJSON(re, true)
}

// GetContactTasks lists one contact's tasks.
func (a *App) GetContactTasks(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	out, err := a.core.ListContactTasks(core.ContactTaskQuery{Subscriber: subID})
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// GetAllContactTasks lists tasks across contacts. ?assignee=me means the current user.
func (a *App) GetAllContactTasks(re *pbcore.RequestEvent) error {
	assignee := strings.TrimSpace(queryParam(re, "assignee"))
	if assignee == "me" {
		assignee = auth.GetUserRE(re).RecordID
	}
	limit, _ := strconv.Atoi(queryParam(re, "limit"))
	out, err := a.core.ListContactTasks(core.ContactTaskQuery{
		Assignee: assignee,
		Status:   strings.TrimSpace(queryParam(re, "status")),
		Limit:    limit,
	})
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// CreateContactTask adds a task to a contact.
func (a *App) CreateContactTask(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	var in core.ContactTaskInput
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.core.AddContactTask(subID, in, auth.GetUserRE(re).RecordID)
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// UpdateContactTask edits, finishes or reopens a task.
func (a *App) UpdateContactTask(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	taskID, err := int64PathParam(re, "taskId")
	if err != nil {
		return err
	}
	var in core.ContactTaskInput
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.core.UpdateContactTask(subID, taskID, in)
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// DeleteContactTask removes a task.
func (a *App) DeleteContactTask(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	taskID, err := int64PathParam(re, "taskId")
	if err != nil {
		return err
	}
	if err := a.core.DeleteContactTask(subID, taskID); err != nil {
		return err
	}
	return okJSON(re, true)
}

// GetConversations lists conversations across channels.
func (a *App) GetConversations(re *pbcore.RequestEvent) error {
	limit, _ := strconv.Atoi(queryParam(re, "limit"))
	offset, _ := strconv.Atoi(queryParam(re, "offset"))
	out, err := a.core.ListConversations(core.ConversationQuery{
		View:    strings.TrimSpace(queryParam(re, "view")),
		Channel: strings.TrimSpace(queryParam(re, "channel")),
		Search:  strings.TrimSpace(queryParam(re, "search")),
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// conversationReplyOption says whether a conversation can be replied to on a channel.
type conversationReplyOption struct {
	Channel        string `json:"channel"`
	Available      bool   `json:"available"`
	Reason         string `json:"reason,omitempty"`
	To             string `json:"to,omitempty"`
	WindowClosesAt string `json:"window_closes_at,omitempty"`
}

func (a *App) conversationContact(thread core.ConversationThread) (models.Subscriber, bool, error) {
	if thread.Subscriber == "" {
		return models.Subscriber{}, false, nil
	}
	return a.core.GetSubscriberForChannel(thread.Subscriber)
}

// emailReplyTo works out where an email reply goes. A contact is always written to at
// their own address, and only senders who aren't contacts are answered at the address
// they wrote from.
func emailReplyTo(thread core.ConversationThread, sub models.Subscriber, hasSub bool) (string, string) {
	if hasSub {
		if sub.Status == models.SubscriberStatusBlockListed {
			return "", replyBlockedBlocklisted
		}
		if strings.TrimSpace(sub.Email) == "" || models.IsPlaceholderEmail(sub.Email) {
			return "", replyBlockedNoEmail
		}
		return sub.Email, ""
	}
	if thread.Channel != models.ChannelEmail {
		return "", replyBlockedNoEmail
	}
	addr, err := mail.ParseAddress(thread.Address)
	if err != nil {
		return "", replyBlockedNoEmail
	}
	return addr.Address, ""
}

func (a *App) smsReplyTo(sub models.Subscriber, hasSub bool) (string, string, error) {
	switch {
	case !a.manager.HasMessenger(quoMessenger):
		return "", replyBlockedNotConfigured, nil
	case !hasSub:
		return "", replyBlockedNoContact, nil
	case sub.Status == models.SubscriberStatusBlockListed:
		return "", replyBlockedBlocklisted, nil
	case strings.TrimSpace(sub.Phone) == "":
		return "", whatsapp.ReplyNoPhone, nil
	}
	optedOut, err := a.core.HasSMSOptOut(sub)
	if err != nil {
		return "", "", err
	}
	if optedOut {
		return "", whatsapp.ReplyOptedOut, nil
	}
	return strings.TrimSpace(sub.Phone), "", nil
}

func (a *App) conversationReplyOptions(thread core.ConversationThread, sub models.Subscriber, hasSub bool) ([]conversationReplyOption, error) {
	option := func(channel, to, reason string) conversationReplyOption {
		return conversationReplyOption{Channel: channel, Available: reason == "", Reason: reason, To: to}
	}
	to, reason := emailReplyTo(thread, sub, hasSub)
	out := []conversationReplyOption{option(models.ChannelEmail, to, reason)}

	to, reason, err := a.smsReplyTo(sub, hasSub)
	if err != nil {
		return nil, err
	}
	out = append(out, option(models.ChannelSMS, to, reason))

	wa := option(models.ChannelWhatsApp, "", replyBlockedNoContact)
	if hasSub {
		t, err := a.whatsapp.GetThread(sub)
		if err != nil {
			return nil, err
		}
		switch {
		case !t.Configured:
			wa.Reason = replyBlockedNotConfigured
		case !t.HasPhone:
			wa.Reason = whatsapp.ReplyNoPhone
		case t.OptedOut:
			wa.Reason = whatsapp.ReplyOptedOut
		case t.RequiresWindow && !t.WindowOpen:
			wa.Reason = whatsapp.ReplyWindowClosed
		default:
			wa.Available, wa.Reason, wa.To = true, "", strings.TrimSpace(sub.Phone)
			if t.WindowClosesAt != nil {
				wa.WindowClosesAt = t.WindowClosesAt.UTC().Format("2006-01-02T15:04:05Z")
			}
		}
	}
	out = append(out, wa)

	// Other plugin channels show in the thread, but replies to them aren't offered here.
	seen := map[string]bool{models.ChannelEmail: true, models.ChannelSMS: true, models.ChannelWhatsApp: true}
	for _, m := range thread.Messages {
		if !seen[m.Channel] {
			seen[m.Channel] = true
			out = append(out, option(m.Channel, "", replyBlockedUnsupported))
		}
	}
	return relevantReplyOptions(out, thread), nil
}

// relevantReplyOptions drops channels that can't be used and aren't part of this
// conversation. Someone who only uses email has no need to hear SMS isn't set up.
func relevantReplyOptions(options []conversationReplyOption, thread core.ConversationThread) []conversationReplyOption {
	out := make([]conversationReplyOption, 0, len(options))
	for _, o := range options {
		hidden := o.Reason == replyBlockedNotConfigured || o.Reason == replyBlockedNoContact
		if !hidden || threadUsesChannel(thread, o.Channel) {
			out = append(out, o)
		}
	}
	return out
}

func threadUsesChannel(thread core.ConversationThread, channel string) bool {
	for _, m := range thread.Messages {
		if m.Channel == channel {
			return true
		}
	}
	return false
}

// GetConversationThread returns a conversation's messages and how it can be answered.
func (a *App) GetConversationThread(re *pbcore.RequestEvent) error {
	thread, err := a.core.GetConversationThread(queryParam(re, "key"))
	if err != nil {
		return err
	}
	sub, hasSub, err := a.conversationContact(thread)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	options, err := a.conversationReplyOptions(thread, sub, hasSub)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	out := map[string]any{
		"key":           thread.Key,
		"address":       thread.Address,
		"channel":       thread.Channel,
		"messages":      thread.Messages,
		"reply_options": options,
		"contact":       nil,
	}
	if hasSub {
		email := sub.Email
		if models.IsPlaceholderEmail(email) {
			email = ""
		}
		profile, err := a.core.GetContactProfile(sub.RecordID)
		if err != nil {
			return apperr.Internal(err.Error())
		}
		boards, err := a.core.GetContactBoards(sub.RecordID)
		if err != nil {
			return apperr.Internal(err.Error())
		}
		out["contact"] = map[string]any{
			"id": sub.RecordID, "name": sub.Name, "email": email, "phone": sub.Phone,
			"status": sub.Status, "boards": boards, "owner": profile.Owner,
			"owner_name": profile.OwnerName, "company": profile.Company,
		}
	}
	return okJSON(re, out)
}

// MarkConversationRead records that the team has seen a conversation.
func (a *App) MarkConversationRead(re *pbcore.RequestEvent) error {
	var in struct {
		Key string `json:"key"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Key) == "" {
		return apperr.BadRequest("a conversation key is required")
	}
	if err := a.core.MarkConversationRead(in.Key); err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, true)
}

// SetConversationClosed closes or reopens a conversation.
func (a *App) SetConversationClosed(re *pbcore.RequestEvent) error {
	var in struct {
		Key    string `json:"key"`
		Closed bool   `json:"closed"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Key) == "" {
		return apperr.BadRequest("a conversation key is required")
	}
	if err := a.core.SetConversationClosed(in.Key, in.Closed); err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, true)
}

// oneLineHeader keeps a value from an inbound email, which anyone can write, from
// adding headers to the reply.
func oneLineHeader(v string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(v))
}

func emailReplySubject(subject string) string {
	subject = oneLineHeader(subject)
	if subject == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

// buildEmailReply makes the reply message. Staff text is sent as plain text and never
// goes through the template engine, so it's delivered exactly as typed.
func buildEmailReply(to, from, subject, body string, inbound models.InboundEmailReplyEvent, hasInbound bool) models.Message {
	msg := models.Message{
		From: from, To: []string{to}, Subject: oneLineHeader(subject),
		ContentType: models.CampaignContentTypePlain, Body: []byte(body), Messenger: emailMsgr,
	}
	if !hasInbound {
		return msg
	}
	if msg.Subject == "" {
		msg.Subject = emailReplySubject(inbound.Subject)
	}
	if id := oneLineHeader(inbound.MessageID); id != "" {
		msg.Headers = textproto.MIMEHeader{}
		msg.Headers.Set("In-Reply-To", id)
		refs := oneLineHeader(inbound.References)
		if refs != "" {
			refs += " "
		}
		msg.Headers.Set("References", refs+id)
	}
	return msg
}

// formatEmailReply applies a format picked in the reply editor to the message, with the
// same sanitizing as a workflow email. It returns the text to show in the thread.
func formatEmailReply(msg *models.Message, contentType string) (string, error) {
	if !isFormattedEmail(contentType) {
		return string(msg.Body), nil
	}
	htmlBody, text, err := flowEmailHTML(contentType, string(msg.Body))
	if err != nil {
		return "", apperr.BadRequest(err.Error())
	}
	msg.ContentType, msg.Body, msg.AltBody = models.CampaignContentTypeHTML, htmlBody, []byte(text)
	return text, nil
}

func isFormattedEmail(contentType string) bool {
	return contentType != "" && contentType != models.CampaignContentTypePlain
}

// sendConversationReply answers a conversation by email or SMS. A non-empty reason means
// it was refused for a reason the person replying can act on.
func (a *App) sendConversationReply(key, channel, subject, body, contentType string) (string, error) {
	body = strings.TrimSpace(body)
	if isFormattedEmail(contentType) {
		if channel != models.ChannelEmail {
			return "", apperr.BadRequest("only an email reply can have a format")
		}
		if body == "" || len([]rune(body)) > maxConversationReplySourceChars {
			return "", apperr.BadRequest("a formatted reply must be 1 to 1000000 characters")
		}
	} else if body == "" || len([]rune(body)) > maxConversationReplyChars {
		return "", apperr.BadRequest("a reply must be 1 to 10000 characters")
	}
	thread, err := a.core.GetConversationThread(key)
	if err != nil {
		return "", err
	}
	sub, hasSub, err := a.conversationContact(thread)
	if err != nil {
		return "", apperr.Internal(err.Error())
	}

	var (
		msg      models.Message
		to       string
		reason   string
		recorded = body
	)
	switch channel {
	case models.ChannelEmail:
		if to, reason = emailReplyTo(thread, sub, hasSub); reason != "" {
			return reason, nil
		}
		inbound, hasInbound, err := a.core.LatestConversationEmail(thread)
		if err != nil {
			return "", err
		}
		msg = buildEmailReply(to, a.defaultCampaignFromEmail(emailMsgr), subject, body, inbound, hasInbound)
		if msg.Subject == "" {
			return "", apperr.BadRequest("an email needs a subject")
		}
		if recorded, err = formatEmailReply(&msg, contentType); err != nil {
			return "", err
		}
	case models.ChannelSMS:
		if to, reason, err = a.smsReplyTo(sub, hasSub); err != nil {
			return "", apperr.Internal(err.Error())
		}
		if reason != "" {
			return reason, nil
		}
		msg = models.Message{To: []string{to}, ContentType: models.CampaignContentTypePlain, Body: []byte(body), Messenger: quoMessenger}
	default:
		return replyBlockedUnsupported, nil
	}
	msg.Subscriber = sub

	if err := a.manager.PushMessage(msg); err != nil {
		return "", apperr.Internal(err.Error())
	}
	if err := a.core.RecordConversationReply(channel, sub.RecordID, strings.ToLower(to), msg.Subject, recorded, security.RandomString(24)); err != nil {
		// The message is already on its way, so saying it failed would invite a resend.
		a.log.Printf("error recording conversation reply: %v", err)
	}
	if err := a.core.MarkConversationRead(thread.Key); err != nil {
		a.log.Printf("error marking conversation read: %v", err)
	}
	return "", nil
}

// SendConversationReply answers a conversation by email or SMS. WhatsApp replies go
// through their own route, which has its own permission. Refusals carry a reason code,
// so the reply box can explain them.
func (a *App) SendConversationReply(re *pbcore.RequestEvent) error {
	return a.conversationReplyRequest(re, true)
}

// SendPlainConversationReply is the assistant's route. The person approving a proposal
// reads its text, which HTML could hide things from, so it never takes a format.
func (a *App) SendPlainConversationReply(re *pbcore.RequestEvent) error {
	return a.conversationReplyRequest(re, false)
}

func (a *App) conversationReplyRequest(re *pbcore.RequestEvent, allowFormat bool) error {
	var in struct {
		Key         string `json:"key"`
		Channel     string `json:"channel"`
		Subject     string `json:"subject"`
		Body        string `json:"body"`
		ContentType string `json:"content_type"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	if !allowFormat {
		in.ContentType = ""
	}
	reason, err := a.sendConversationReply(in.Key, in.Channel, in.Subject, in.Body, in.ContentType)
	if err != nil {
		return err
	}
	if reason != "" {
		return re.JSON(http.StatusConflict, map[string]any{"message": reason, "reason": reason})
	}
	return okJSON(re, true)
}
