package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestForwardemailProcessBounceExtractsMessageIDAndCampaignHeadersCaseInsensitive(t *testing.T) {
	key := []byte("secret-key")
	f := NewForwardemail(key)

	body := []byte(`{
		"recipient":"User@Example.com",
		"bounced_at":"2026-04-22T10:15:30.1234569Z",
		"headers":{
			"x-listpocket-campaign":"camprecid01234567",
			"message-id":"<camprecid01234567@listpocket.local>"
		},
		"bounce":{
			"category":"recipient"
		}
	}`)

	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	out, err := f.ProcessBounce(sig, body)
	if err != nil {
		t.Fatalf("ProcessBounce returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 bounce, got %d", len(out))
	}
	if out[0].CampaignUUID != "camprecid01234567" {
		t.Fatalf("campaign=%q", out[0].CampaignUUID)
	}
	if out[0].MessageID != "camprecid01234567@listpocket.local" {
		t.Fatalf("message_id=%q", out[0].MessageID)
	}
}
