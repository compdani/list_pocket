package quo

import (
	"strings"
	"testing"
)

func TestSanitizeSMSBody_stripsHTML(t *testing.T) {
	in := `<!DOCTYPE html><html><body><div style="color:red">Hi <b>Dan</b>,<br/>Reminder&nbsp;&amp; RSVP.</div></body></html>`
	got := sanitizeSMSBody([]byte(in))
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Fatalf("expected no HTML tags, got %q", got)
	}
	if !strings.Contains(got, "Reminder") || !strings.Contains(got, "&") {
		t.Fatalf("expected entity-decoded text, got %q", got)
	}
}

func TestSplitSMSBody_underLimitReturnsOne(t *testing.T) {
	parts := splitSMSBody("hello world")
	if len(parts) != 1 || parts[0] != "hello world" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
}

func TestSplitSMSBody_splitsLongText(t *testing.T) {
	word := "abcdefghij "
	body := strings.Repeat(word, 500)
	if len([]rune(body)) <= maxQuoContentLen {
		t.Fatalf("test body not long enough: %d", len(body))
	}

	parts := splitSMSBody(body)
	if len(parts) < 2 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}
	if len(parts) > maxQuoParts {
		t.Fatalf("parts exceed max (%d): got %d", maxQuoParts, len(parts))
	}
	for i, p := range parts {
		if len([]rune(p)) > maxQuoContentLen {
			t.Fatalf("part %d too long: %d runes", i, len([]rune(p)))
		}
		if !strings.HasPrefix(p, "(") {
			t.Fatalf("part %d missing (n/N) prefix: %q", i, p[:min(20, len(p))])
		}
	}
}

func TestSplitSMSBody_cappedAtMaxParts(t *testing.T) {
	body := strings.Repeat("a", (maxQuoContentLen*maxQuoParts)+10000)
	parts := splitSMSBody(body)
	if len(parts) > maxQuoParts {
		t.Fatalf("expected ≤ %d parts, got %d", maxQuoParts, len(parts))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
