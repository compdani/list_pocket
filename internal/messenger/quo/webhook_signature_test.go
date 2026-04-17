package quo

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
	"time"
)

func TestVerifyWebhookSignature_ok(t *testing.T) {
	key := []byte("testsecretkey123456789012345678")
	secretB64 := base64.StdEncoding.EncodeToString(key)
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	body := []byte(`{"a":1}`)
	signed := ts + "." + string(body)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(signed))
	sigB64 := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	header := "hmac;1;" + ts + ";" + sigB64
	if err := VerifyWebhookSignature(secretB64, header, body, time.Hour); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyWebhookSignature_badBody(t *testing.T) {
	key := []byte("testsecretkey123456789012345678")
	secretB64 := base64.StdEncoding.EncodeToString(key)
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	body := []byte(`{"a":1}`)
	signed := ts + "." + string(body)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(signed))
	sigB64 := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	header := "hmac;1;" + ts + ";" + sigB64
	if err := VerifyWebhookSignature(secretB64, header, []byte(`{}`), time.Hour); err == nil {
		t.Fatal("expected mismatch")
	}
}
