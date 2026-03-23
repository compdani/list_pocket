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
