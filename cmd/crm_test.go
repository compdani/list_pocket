package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/models"

	"github.com/compdani/list_pocket/internal/apperr"
)

func TestEmailReplyTo(t *testing.T) {
	stranger := core.ConversationThread{Channel: models.ChannelEmail, Address: "them@example.com"}
	tests := []struct {
		name   string
		thread core.ConversationThread
		sub    models.Subscriber
		hasSub bool
		to     string
		reason string
	}{
		{name: "stranger is answered at their address", thread: stranger, to: "them@example.com"},
		{name: "a stranger's bad address is refused", thread: core.ConversationThread{Channel: models.ChannelEmail, Address: "not an address"}, reason: replyBlockedNoEmail},
		{name: "a stranger on another channel has no email", thread: core.ConversationThread{Channel: models.ChannelSMS, Address: "+1555"}, reason: replyBlockedNoEmail},
		{name: "contact is written to at their own address", thread: stranger, sub: models.Subscriber{Email: "contact@example.com"}, hasSub: true, to: "contact@example.com"},
		{name: "blocklisted contact is refused", thread: stranger, sub: models.Subscriber{Email: "contact@example.com", Status: models.SubscriberStatusBlockListed}, hasSub: true, reason: replyBlockedBlocklisted},
		{name: "placeholder email is refused", thread: stranger, sub: models.Subscriber{Email: "x@" + models.PlaceholderEmailDomain}, hasSub: true, reason: replyBlockedNoEmail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			to, reason := emailReplyTo(tt.thread, tt.sub, tt.hasSub)
			if to != tt.to || reason != tt.reason {
				t.Fatalf("got %q, %q", to, reason)
			}
		})
	}
}

func TestBuildEmailReply_threadsAndStripsHeaderInjection(t *testing.T) {
	inbound := models.InboundEmailReplyEvent{
		Subject:    "Hello\r\nBcc: evil@example.com",
		MessageID:  "<id@x>\r\nBcc: evil@example.com",
		References: "<root@x>",
	}
	msg := buildEmailReply("to@example.com", "from@example.com", "", "typed {{ .Subscriber.Email }} text", inbound, true)

	if strings.ContainsAny(msg.Subject, "\r\n") || !strings.HasPrefix(msg.Subject, "Re: Hello") {
		t.Fatalf("subject: %q", msg.Subject)
	}
	for _, name := range []string{"In-Reply-To", "References"} {
		if v := msg.Headers.Get(name); v == "" || strings.ContainsAny(v, "\r\n") {
			t.Fatalf("%s: %q", name, v)
		}
	}
	if !strings.HasPrefix(msg.Headers.Get("References"), "<root@x> <id@x>") {
		t.Fatalf("references: %q", msg.Headers.Get("References"))
	}
	if string(msg.Body) != "typed {{ .Subscriber.Email }} text" || msg.ContentType != models.CampaignContentTypePlain {
		t.Fatalf("staff text must be sent exactly as typed: %q %q", msg.Body, msg.ContentType)
	}
}

func TestFormatEmailReply(t *testing.T) {
	msg := models.Message{ContentType: models.CampaignContentTypePlain, Body: []byte("<b>as typed</b>")}
	text, err := formatEmailReply(&msg, "")
	if err != nil || text != "<b>as typed</b>" || string(msg.Body) != text || msg.ContentType != models.CampaignContentTypePlain {
		t.Fatalf("no format must leave the text alone: %q %q %v", text, msg.Body, err)
	}

	msg = models.Message{
		ContentType: models.CampaignContentTypePlain,
		Body:        []byte(`<p onclick="x()">Hello <a href="https://example.com@TrackLink">there</a></p><script>x()</script>`),
	}
	text, err = formatEmailReply(&msg, models.CampaignContentTypeRichtext)
	if err != nil {
		t.Fatal(err)
	}
	body := string(msg.Body)
	if msg.ContentType != models.CampaignContentTypeHTML || strings.Contains(body, "script") || strings.Contains(body, "onclick") {
		t.Fatalf("not sanitized: %q %q", msg.ContentType, body)
	}
	if strings.Contains(body, "@TrackLink") || !strings.Contains(body, "https://example.com") {
		t.Fatalf("link: %q", body)
	}
	if text != "Hello there (https://example.com)" || string(msg.AltBody) != text {
		t.Fatalf("text: %q alt: %q", text, msg.AltBody)
	}

	// Indented HTML from the editor reads as ordinary text.
	msg = models.Message{Body: []byte("<p>\n    <strong>\n        Hello\n    </strong>\n    there\n</p>")}
	if text, err = formatEmailReply(&msg, models.CampaignContentTypeRichtext); err != nil || text != "Hello there" {
		t.Fatalf("text: %q %v", text, err)
	}

	msg = models.Message{Body: []byte("hi")}
	if _, err := formatEmailReply(&msg, "binary"); err == nil {
		t.Fatal("an unknown format must be refused")
	}
}

func TestEmailReplySubject_doesNotStackPrefixes(t *testing.T) {
	if got := emailReplySubject("RE: hi"); got != "RE: hi" {
		t.Fatal(got)
	}
	if got := emailReplySubject("hi"); got != "Re: hi" {
		t.Fatal(got)
	}
	if got := emailReplySubject("  "); got != "" {
		t.Fatal(got)
	}
}

func TestRelevantReplyOptions_hidesUnusableChannelsNotInThread(t *testing.T) {
	options := []conversationReplyOption{
		{Channel: models.ChannelEmail, Available: true},
		{Channel: models.ChannelSMS, Reason: replyBlockedNotConfigured},
		{Channel: models.ChannelWhatsApp, Reason: replyBlockedNoContact},
	}
	got := relevantReplyOptions(options, core.ConversationThread{})
	if len(got) != 1 || got[0].Channel != models.ChannelEmail {
		t.Fatalf("got %+v", got)
	}

	// They stay when the person used that channel, so the reply box can say why not.
	thread := core.ConversationThread{Messages: []core.ConversationMessage{{Channel: models.ChannelSMS}}}
	got = relevantReplyOptions(options, thread)
	if len(got) != 2 || got[1].Channel != models.ChannelSMS {
		t.Fatalf("got %+v", got)
	}
}

func TestFlowSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Welcome New Contacts!": "welcome_new_contacts",
		"  a--b  ":              "a_b",
		"9 lives":               "workflow_9_lives",
		"":                      "workflow_",
	} {
		if got := flowSlug(in); got != want {
			t.Errorf("flowSlug(%q) = %q, want %q", in, got, want)
		}
	}
	if got := flowSlug(strings.Repeat("a", 100)); len(got) > 60 {
		t.Errorf("too long: %d", len(got))
	}
}

func TestAIChoiceValidate(t *testing.T) {
	var none *aiChoice
	if err := none.validate(); err != nil {
		t.Fatalf("no choice means the default: %v", err)
	}
	for _, good := range []aiChoice{{Provider: "api", Model: "m"}, {Provider: "chatgpt", Model: "m"}} {
		if err := good.validate(); err != nil {
			t.Errorf("%+v: %v", good, err)
		}
	}
	for _, bad := range []aiChoice{{Provider: "other", Model: "m"}, {Provider: "api"}, {Model: "m"}} {
		if err := bad.validate(); err == nil {
			t.Errorf("%+v should be refused", bad)
		}
	}
}

func TestApplyRefused(t *testing.T) {
	if !applyRefused(apperr.BadRequest("no placeholder")) {
		t.Fatal("a refused input should leave the proposal open to correct")
	}
	for _, err := range []error{nil, apperr.Internal("write failed"), apperr.Forbidden("no"), errors.New("plain")} {
		if applyRefused(err) {
			t.Fatalf("%v may have written, so it must not be retried", err)
		}
	}
}
