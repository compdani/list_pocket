package quo

import (
	"strings"
	"testing"

	"github.com/compdani/list_pocket/models"
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

func TestMessengerReusesHTTPClient(t *testing.T) {
	m := NewMessenger(func() models.TextMessagingSettings {
		return models.TextMessagingSettings{
			Providers: []models.TextMessagingProvider{
				{ID: "quo", Enabled: true, APIKey: "key-1", From: "+15551234567"},
			},
		}
	})

	c1, err := m.client()
	if err != nil {
		t.Fatal(err)
	}
	c2, err := m.client()
	if err != nil {
		t.Fatal(err)
	}
	if c1.HTTP == nil || c1.HTTP != c2.HTTP {
		t.Fatal("expected the same http.Client across Push calls")
	}
	if c1 != c2 {
		t.Fatal("expected cached Client pointer when settings are unchanged")
	}

	m.loadSettings = func() models.TextMessagingSettings {
		return models.TextMessagingSettings{
			Providers: []models.TextMessagingProvider{
				{ID: "quo", Enabled: true, APIKey: "key-2", From: "+15551234567"},
			},
		}
	}
	c3, err := m.client()
	if err != nil {
		t.Fatal(err)
	}
	if c3 == c1 {
		t.Fatal("expected a new Client after API key change")
	}
	if c3.HTTP != c1.HTTP {
		t.Fatal("expected HTTP client reuse after settings change")
	}
	if c3.APIKey != "key-2" {
		t.Fatalf("APIKey = %q, want key-2", c3.APIKey)
	}
}
