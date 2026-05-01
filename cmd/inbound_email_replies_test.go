package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestNormalizeSESInboundEmail_Base64MIME(t *testing.T) {
	t.Parallel()

	rawMIME := strings.Join([]string{
		"From: Jane Sender <jane@example.com>",
		"To: replies@example.net",
		"Subject: Re: Campaign Follow-up",
		"Message-ID: <ses-message-id@example.com>",
		"In-Reply-To: <ledger-record-id-12345>",
		"References: <ledger-record-id-12345> <previous-msg-id@example.com>",
		"Date: Fri, 17 Apr 2026 12:34:56 +0000",
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=\"mix-boundary\"",
		"",
		"--mix-boundary",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Thanks, I am interested.",
		"",
		"--mix-boundary",
		"Content-Type: application/octet-stream",
		"Content-Disposition: attachment; filename=\"proof.txt\"",
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte("hello")),
		"",
		"--mix-boundary--",
		"",
	}, "\r\n")

	sesPayload := map[string]any{
		"notificationType": "Received",
		"mail": map[string]any{
			"timestamp": "2026-04-17T12:34:56Z",
			"source":    "jane@example.com",
			"messageId": "ses-original-message-id",
			"headers": []map[string]any{
				{"name": "X-SES-Test", "value": "1"},
			},
			"commonHeaders": map[string]any{
				"from":      []string{"Jane Sender <jane@example.com>"},
				"subject":   "Re: Campaign Follow-up",
				"messageId": "ses-original-message-id",
				"date":      "Fri, 17 Apr 2026 12:34:56 +0000",
			},
		},
		"content": base64.StdEncoding.EncodeToString([]byte(rawMIME)),
	}

	payload, err := json.Marshal(sesPayload)
	if err != nil {
		t.Fatalf("marshal ses payload: %v", err)
	}

	normalized, ok := normalizeSESInboundEmail(payload)
	if !ok {
		t.Fatal("expected SES payload to be recognized")
	}

	if normalized.Provider != "ses" {
		t.Fatalf("provider: got %q want %q", normalized.Provider, "ses")
	}
	if normalized.From != "jane@example.com" {
		t.Fatalf("from: got %q", normalized.From)
	}
	if normalized.MessageID != "ses-message-id@example.com" {
		t.Fatalf("message_id: got %q", normalized.MessageID)
	}
	if normalized.InReplyTo != "ledger-record-id-12345" {
		t.Fatalf("in_reply_to: got %q", normalized.InReplyTo)
	}
	if !strings.Contains(normalized.References, "ledger-record-id-12345") {
		t.Fatalf("references missing expected id: %q", normalized.References)
	}
	if normalized.Subject != "Re: Campaign Follow-up" {
		t.Fatalf("subject: got %q", normalized.Subject)
	}
	if normalized.Text != "Thanks, I am interested." {
		t.Fatalf("text body: got %q", normalized.Text)
	}
	if !normalized.HasAttachments {
		t.Fatal("expected attachment detection to be true")
	}
	if strings.TrimSpace(normalized.BodySnippet) == "" {
		t.Fatal("expected non-empty body snippet")
	}
	if _, ok := normalized.Headers["from"]; !ok {
		t.Fatal("expected normalized headers to include from")
	}
	if normalized.HTML == "" && normalized.Text == "" {
		t.Fatal("expected parsed body (HTML or text) to be non-empty")
	}
	if len(normalized.Attachments) != 1 {
		t.Fatalf("expected one attachment, got %d", len(normalized.Attachments))
	}
	if normalized.Attachments[0].Filename != "proof.txt" {
		t.Fatalf("attachment filename: got %q", normalized.Attachments[0].Filename)
	}
}

func TestNormalizeSESInboundEmail_SNSWrappedPayload(t *testing.T) {
	t.Parallel()

	rawMIME := strings.Join([]string{
		"From: Reply User <reply@example.org>",
		"To: support@example.net",
		"Subject: Re: Thread",
		"Message-ID: <wrapped-message-id@example.org>",
		"In-Reply-To: <ledger-id-99999>",
		"Date: Fri, 17 Apr 2026 09:10:11 +0000",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"wrapped reply",
	}, "\r\n")

	inner := map[string]any{
		"notificationType": "Received",
		"mail": map[string]any{
			"timestamp": "2026-04-17T09:10:11Z",
			"messageId": "fallback-message-id",
		},
		"content": base64.StdEncoding.EncodeToString([]byte(rawMIME)),
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("marshal inner payload: %v", err)
	}

	sns := map[string]any{
		"Type":    "Notification",
		"Message": string(innerJSON),
	}
	outerJSON, err := json.Marshal(sns)
	if err != nil {
		t.Fatalf("marshal sns payload: %v", err)
	}

	normalized, ok := normalizeSESInboundEmail(outerJSON)
	if !ok {
		t.Fatal("expected SNS-wrapped SES payload to be recognized")
	}
	if normalized.Provider != "ses" {
		t.Fatalf("provider: got %q want ses", normalized.Provider)
	}
	if normalized.MessageID != "wrapped-message-id@example.org" {
		t.Fatalf("message_id: got %q", normalized.MessageID)
	}
	if normalized.InReplyTo != "ledger-id-99999" {
		t.Fatalf("in_reply_to: got %q", normalized.InReplyTo)
	}
	if normalized.Text != "wrapped reply" {
		t.Fatalf("text: got %q", normalized.Text)
	}
}

func TestNormalizeSESInboundEmail_InvalidBase64ReturnsFalse(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"notificationType": "Received",
		"mail": map[string]any{
			"timestamp": "2026-04-17T12:00:00Z",
		},
		"content": "%%%not-base64%%%",
	}

	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if _, ok := normalizeSESInboundEmail(b); ok {
		t.Fatal("expected invalid SES content to return ok=false")
	}
}

func TestParseSNSControlType(t *testing.T) {
	t.Parallel()

	sub := []byte(`{"Type":"SubscriptionConfirmation","Message":"confirm"}`)
	typeName, ok := parseSNSControlType(sub)
	if !ok || typeName != "SubscriptionConfirmation" {
		t.Fatalf("expected SubscriptionConfirmation, got type=%q ok=%v", typeName, ok)
	}

	unsub := []byte(`{"Type":"UnsubscribeConfirmation","Message":"confirm"}`)
	typeName, ok = parseSNSControlType(unsub)
	if !ok || typeName != "UnsubscribeConfirmation" {
		t.Fatalf("expected UnsubscribeConfirmation, got type=%q ok=%v", typeName, ok)
	}

	notif := []byte(`{"Type":"Notification","Message":"{}"}`)
	if typeName, ok = parseSNSControlType(notif); ok {
		t.Fatalf("expected Notification not treated as control, got type=%q", typeName)
	}
}

func TestNormalizeSESInboundEmail_IgnoresNonNotificationSNSWrapper(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"Type":    "SubscriptionConfirmation",
		"Message": `{"notificationType":"Received","content":"abc"}`,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if _, ok := normalizeSESInboundEmail(b); ok {
		t.Fatal("expected non-notification SNS wrapper to be ignored")
	}
}

func TestParseSESNotificationType(t *testing.T) {
	t.Parallel()

	direct := []byte(`{"notificationType":"Received","mail":{"timestamp":"2026-04-20T15:00:00Z"},"content":"YWJj"}`)
	if got := parseSESNotificationType(direct); got != "Received" {
		t.Fatalf("direct type: got %q want %q", got, "Received")
	}

	wrapped := []byte(`{"Type":"Notification","Message":"{\"notificationType\":\"Bounce\",\"mail\":{}}"}`)
	if got := parseSESNotificationType(wrapped); got != "Bounce" {
		t.Fatalf("wrapped type: got %q want %q", got, "Bounce")
	}

	invalid := []byte(`{"Type":"Notification","Message":"not-json"}`)
	if got := parseSESNotificationType(invalid); got != "" {
		t.Fatalf("invalid type: got %q want empty", got)
	}
}

func TestLogInboundEmailWebhookRequestDoesNotLogBody(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	app := &App{log: log.New(&logs, "", 0)}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/ses", strings.NewReader(""))
	req.Header.Set(echo.HeaderContentType, "text/plain; charset=UTF-8")
	req.Header.Set("X-Amz-Sns-Message-Type", "Notification")
	req.Header.Set("X-Amz-Sns-Topic-Arn", "arn:aws:sns:us-east-1:123456789012:ses-received")
	req.Header.Set("X-Amz-Sns-Message-Id", "sns-message-id")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/webhooks/ses")

	body := []byte(`{"Type":"Notification","Message":"{\"notificationType\":\"Received\",\"content\":\"secret-body\"}"}`)
	app.logInboundEmailWebhookRequest(c, body)

	line := logs.String()
	if strings.Contains(line, "body=") || strings.Contains(line, "secret-body") {
		t.Fatalf("expected log to omit raw body, got: %s", line)
	}
	for _, want := range []string{
		`size=`,
		`sns_type="Notification"`,
		`sns_message_id="sns-message-id"`,
		`ses_notification_type="Received"`,
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected log to contain %s, got: %s", want, line)
		}
	}
}

func TestInboundWebhookBodyPreviewForLogTruncates(t *testing.T) {
	t.Parallel()

	body := []byte(strings.Repeat("a", 600))
	preview, truncated := inboundWebhookBodyPreviewForLog(body)
	if !truncated {
		t.Fatal("expected preview to report truncation")
	}
	if len(preview) != 512 {
		t.Fatalf("preview length: got %d want 512", len(preview))
	}
}
