package webhooks

import (
	"encoding/json"
	"testing"
)

func TestBrevo_ProcessBounce(t *testing.T) {
	secret := "test-secret-token-ok"
	b := NewBrevo(secret)

	body := []byte(`{
		"event":"hard_bounce",
		"email":"User@Example.com",
		"ts_event":1604933654,
		"X-Mailin-custom":"550e8400-e29b-41d4-a716-446655440000",
		"reason":"mailbox unavailable"
	}`)

	bs, err := b.ProcessBounce("Bearer "+secret, body)
	if err != nil {
		t.Fatalf("ProcessBounce: %v", err)
	}
	if len(bs) != 1 {
		t.Fatalf("expected 1 bounce, got %d", len(bs))
	}
	if bs[0].Type != "hard" || bs[0].Email != "user@example.com" || bs[0].Source != "brevo" {
		t.Fatalf("unexpected bounce: %+v", bs[0])
	}
	if bs[0].CampaignUUID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("campaign uuid: %q", bs[0].CampaignUUID)
	}
}

func TestBrevo_ProcessBounce_Spam(t *testing.T) {
	b := NewBrevo("s")
	bs, err := b.ProcessBounce("Bearer s", []byte(`{"event":"spam","email":"a@b.co","ts_event":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0].Type != "complaint" {
		t.Fatalf("got %+v", bs)
	}
}

func TestBrevo_ProcessBounce_SkipsUnknown(t *testing.T) {
	b := NewBrevo("s")
	bs, err := b.ProcessBounce("Bearer s", []byte(`{"event":"delivered","email":"a@b.co","ts_event":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 0 {
		t.Fatalf("expected skip, got %+v", bs)
	}
}

func TestBrevo_ProcessBounce_AuthFails(t *testing.T) {
	b := NewBrevo("good")
	_, err := b.ProcessBounce("Bearer bad", []byte(`{"event":"hard_bounce","email":"a@b.co"}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBrevo_ProcessBounce_Array(t *testing.T) {
	b := NewBrevo("k")
	body, _ := json.Marshal([]map[string]any{
		{"event": "opened", "email": "x@y.z", "ts_event": 1},
		{"event": "soft_bounce", "email": "a@b.c", "ts_event": 2},
	})
	bs, err := b.ProcessBounce("Bearer k", body)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0].Email != "a@b.c" || bs[0].Type != "soft" {
		t.Fatalf("got %+v", bs)
	}
}

func TestBrevo_ProcessBounce_ExtractsMessageID(t *testing.T) {
	secret := "test-secret-token-ok"
	b := NewBrevo(secret)

	body := []byte(`{
		"event":"hard_bounce",
		"email":"User@Example.com",
		"ts_event":1604933654,
		"X-Mailin-custom":"550e8400-e29b-41d4-a716-446655440000",
		"message-id":"<camprecid01234567@listpocket.local>",
		"reason":"mailbox unavailable"
	}`)

	bs, err := b.ProcessBounce("Bearer "+secret, body)
	if err != nil {
		t.Fatalf("ProcessBounce: %v", err)
	}
	if len(bs) != 1 {
		t.Fatalf("expected 1 bounce, got %d", len(bs))
	}
	if bs[0].MessageID != "camprecid01234567@listpocket.local" {
		t.Fatalf("message id: %q", bs[0].MessageID)
	}
}
