package core

import "testing"

func TestNormalizeAnalyticsDateInputAcceptsSQLiteMillisZulu(t *testing.T) {
	got, err := normalizeAnalyticsDateInput("2026-03-31 17:25:02.095Z", false)
	if err != nil {
		t.Fatalf("expected date to parse, got error: %v", err)
	}
	if got != "2026-03-31 17:25:02" {
		t.Fatalf("expected normalized timestamp %q, got %q", "2026-03-31 17:25:02", got)
	}
}

func TestNormalizeAnalyticsDateInputAcceptsSQLiteMillisOffset(t *testing.T) {
	got, err := normalizeAnalyticsDateInput("2026-03-31 12:25:02.095-05:00", false)
	if err != nil {
		t.Fatalf("expected date to parse, got error: %v", err)
	}
	if got != "2026-03-31 17:25:02" {
		t.Fatalf("expected normalized UTC timestamp %q, got %q", "2026-03-31 17:25:02", got)
	}
}
