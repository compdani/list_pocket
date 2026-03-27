package core

import (
	"strings"
	"testing"
	"time"

	"github.com/compdani/list_pocket/models"
)

func TestClassifyPrivacyOpenMarksFastAppleOpenAsSuspected(t *testing.T) {
	referenceAt := time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC)
	event := models.OpenEvent{
		IPAddress: "17.1.2.3",
		UserAgent: "Mozilla/5.0 AppleWebKit/605.1.15",
		OpenedAt:  referenceAt.Add(45 * time.Second),
	}

	suspected, meta, err := classifyPrivacyOpen(event, referenceAt, "transactional_sent_at")
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if !suspected {
		t.Fatal("expected fast Apple open to be marked as suspected")
	}
	if !strings.Contains(meta, "apple_mail_fast_open") {
		t.Fatalf("expected metadata to record suspected reason, got %q", meta)
	}
}

func TestClassifyPrivacyOpenLeavesDelayedAppleOpenUnconfirmed(t *testing.T) {
	referenceAt := time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC)
	event := models.OpenEvent{
		IPAddress: "17.1.2.3",
		UserAgent: "Mozilla/5.0 AppleWebKit/605.1.15",
		OpenedAt:  referenceAt.Add(10 * time.Minute),
	}

	suspected, meta, err := classifyPrivacyOpen(event, referenceAt, "transactional_sent_at")
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if suspected {
		t.Fatal("expected delayed Apple open to remain unsuspicious")
	}
	if strings.Contains(meta, "apple_mail_fast_open") {
		t.Fatalf("did not expect suspected reason in metadata, got %q", meta)
	}
}

func TestClassifyPrivacyOpenMarksFastGoogleImageProxyOpenAsSuspected(t *testing.T) {
	referenceAt := time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC)
	event := models.OpenEvent{
		IPAddress: "66.249.80.1",
		UserAgent: "Mozilla/5.0 (Windows NT 5.1; rv:11.0) Gecko Firefox/11.0 (via ggpht.com GoogleImageProxy)",
		OpenedAt:  referenceAt.Add(30 * time.Second),
	}

	suspected, meta, err := classifyPrivacyOpen(event, referenceAt, "campaign_send_at")
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if !suspected {
		t.Fatal("expected fast GoogleImageProxy open to be marked as suspected")
	}
	if !strings.Contains(meta, "apple_mail_fast_open") {
		t.Fatalf("expected metadata to use MPP bucket reason, got %q", meta)
	}
}

func TestClassifyPrivacyOpenMarksFastBrevoOpenAsSuspected(t *testing.T) {
	referenceAt := time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC)
	event := models.OpenEvent{
		IPAddress: "185.41.28.4",
		UserAgent: "Brevo/1.0",
		OpenedAt:  referenceAt.Add(20 * time.Second),
	}

	suspected, meta, err := classifyPrivacyOpen(event, referenceAt, "campaign_send_at")
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if !suspected {
		t.Fatal("expected fast Brevo open to be marked as suspected")
	}
	if !strings.Contains(meta, "apple_mail_fast_open") {
		t.Fatalf("expected metadata to use MPP bucket reason, got %q", meta)
	}
}

func TestCampaignPrivacyReferencePrefersSendAt(t *testing.T) {
	sendAt := time.Date(2026, time.March, 23, 11, 59, 0, 0, time.UTC)
	startedAt := time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC)

	refAt, refType := campaignPrivacyReference(sendAt, startedAt)
	if !refAt.Equal(sendAt) {
		t.Fatalf("expected send_at reference %s, got %s", sendAt, refAt)
	}
	if refType != "campaign_send_at" {
		t.Fatalf("expected campaign_send_at reference type, got %q", refType)
	}
}

func TestCampaignPrivacyReferenceFallsBackToStartedAt(t *testing.T) {
	startedAt := time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC)

	refAt, refType := campaignPrivacyReference(time.Time{}, startedAt)
	if !refAt.Equal(startedAt) {
		t.Fatalf("expected started_at reference %s, got %s", startedAt, refAt)
	}
	if refType != "campaign_started_at" {
		t.Fatalf("expected campaign_started_at reference type, got %q", refType)
	}
}

func TestCampaignPrivacyReferenceWithLedgerPrefersSubscriberSentAt(t *testing.T) {
	ledgerSentAt := time.Date(2026, time.March, 23, 12, 3, 0, 0, time.UTC)
	sendAt := time.Date(2026, time.March, 23, 11, 59, 0, 0, time.UTC)
	startedAt := time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC)

	refAt, refType := campaignPrivacyReferenceWithLedger(ledgerSentAt, sendAt, startedAt)
	if !refAt.Equal(ledgerSentAt) {
		t.Fatalf("expected ledger sent_at reference %s, got %s", ledgerSentAt, refAt)
	}
	if refType != "campaign_subscriber_sent_at" {
		t.Fatalf("expected campaign_subscriber_sent_at reference type, got %q", refType)
	}
}
