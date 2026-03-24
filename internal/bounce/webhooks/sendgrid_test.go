package webhooks

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"math/big"
	"testing"
	"time"

	"github.com/compdani/list_pocket/models"
)

func signSendgridPayload(t *testing.T, priv *ecdsa.PrivateKey, timestamp, payload []byte) string {
	t.Helper()

	h := sha256.New()
	h.Write(timestamp)
	h.Write(payload)
	hash := h.Sum(nil)

	r, s, err := ecdsa.Sign(rand.Reader, priv, hash)
	if err != nil {
		t.Fatalf("failed signing payload: %v", err)
	}

	der, err := asn1.Marshal(struct {
		R, S *big.Int
	}{R: r, S: s})
	if err != nil {
		t.Fatalf("failed marshaling signature: %v", err)
	}

	return base64.StdEncoding.EncodeToString(der)
}

func TestSendgridProcessBounceHandlesBounceAndDropped(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	sg := &Sendgrid{pubKey: &priv.PublicKey}
	payload := []byte(`[
		{"email":"User@Example.com","timestamp":1710000000,"event":"bounce","bounce_classification":"technical","XListpocketCampaign":"cmp-1"},
		{"email":"drop@example.com","timestamp":1710000060,"event":"dropped","bounce_classification":"invalid","XListpocketCampaign":"cmp-2"},
		{"email":"ok@example.com","timestamp":1710000120,"event":"delivered","XListpocketCampaign":"cmp-3"}
	]`)
	timestamp := []byte("1710001000")
	sig := signSendgridPayload(t, priv, timestamp, payload)

	got, err := sg.ProcessBounce(sig, string(timestamp), payload)
	if err != nil {
		t.Fatalf("ProcessBounce returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 bounce records, got %d", len(got))
	}

	if got[0].Email != "user@example.com" {
		t.Fatalf("expected lower-cased bounce email, got %q", got[0].Email)
	}
	if got[0].Type != models.BounceTypeSoft {
		t.Fatalf("expected technical bounce to be soft, got %q", got[0].Type)
	}
	if got[0].CampaignUUID != "cmp-1" {
		t.Fatalf("expected bounce campaign cmp-1, got %q", got[0].CampaignUUID)
	}

	if got[1].Email != "drop@example.com" {
		t.Fatalf("expected dropped email to be preserved/lower-cased, got %q", got[1].Email)
	}
	if got[1].Type != models.BounceTypeHard {
		t.Fatalf("expected dropped event to be hard bounce, got %q", got[1].Type)
	}
	if got[1].CampaignUUID != "cmp-2" {
		t.Fatalf("expected dropped campaign cmp-2, got %q", got[1].CampaignUUID)
	}

	if !got[0].CreatedAt.Equal(time.Unix(1710000000, 0)) {
		t.Fatalf("unexpected created_at for bounce event: %s", got[0].CreatedAt)
	}
	if !got[1].CreatedAt.Equal(time.Unix(1710000060, 0)) {
		t.Fatalf("unexpected created_at for dropped event: %s", got[1].CreatedAt)
	}
}

func TestSendgridProcessBounceHandlesURLSafeSignature(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	sg := &Sendgrid{pubKey: &priv.PublicKey}
	payload := []byte(`[{"email":"drop@example.com","timestamp":1710000060,"event":"dropped","bounce_classification":"invalid","XListpocketCampaign":"cmp-2"}]`)
	timestamp := []byte("1710001000")
	stdSig := signSendgridPayload(t, priv, timestamp, payload)
	stdSigBytes, err := base64.StdEncoding.DecodeString(stdSig)
	if err != nil {
		t.Fatalf("failed to decode std signature: %v", err)
	}
	urlSig := base64.RawURLEncoding.EncodeToString(stdSigBytes)

	got, err := sg.ProcessBounce(urlSig, string(timestamp), payload)
	if err != nil {
		t.Fatalf("ProcessBounce returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 bounce record, got %d", len(got))
	}
	if got[0].Type != models.BounceTypeHard {
		t.Fatalf("expected dropped event to be hard bounce, got %q", got[0].Type)
	}
}
