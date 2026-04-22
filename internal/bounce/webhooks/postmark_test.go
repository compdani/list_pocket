package webhooks

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestPostmarkProcessBounceExtractsMessageID(t *testing.T) {
	p := NewPostmark("user", "pass")
	e := echo.New()
	req := httptest.NewRequest("POST", "/", nil)
	req.SetBasicAuth("user", "pass")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	body := []byte(`{
		"RecordType":"Bounce",
		"Type":"HardBounce",
		"MessageID":"<camprecid01234567@listpocket.local>",
		"Email":"User@Example.com",
		"BouncedAt":"2026-04-22T10:15:30.1234569Z",
		"Metadata":{
			"X-Listpocket-Campaign":"550e8400-e29b-41d4-a716-446655440000"
		}
	}`)

	out, err := p.ProcessBounce(body, ctx)
	if err != nil {
		t.Fatalf("ProcessBounce returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 bounce, got %d", len(out))
	}
	if out[0].MessageID != "camprecid01234567@listpocket.local" {
		t.Fatalf("message_id=%q", out[0].MessageID)
	}
	if out[0].Email != "user@example.com" {
		t.Fatalf("email=%q", out[0].Email)
	}
	if !strings.EqualFold(out[0].CampaignUUID, "550e8400-e29b-41d4-a716-446655440000") {
		t.Fatalf("campaign=%q", out[0].CampaignUUID)
	}
}
