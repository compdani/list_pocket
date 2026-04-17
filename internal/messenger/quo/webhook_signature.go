package quo

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const openPhoneSigHeader = "openphone-signature"

// OpenPhoneSignatureHeader returns the canonical header name for webhook verification.
func OpenPhoneSignatureHeader() string { return openPhoneSigHeader }

// VerifyWebhookSignature checks the OpenPhone/Quo `openphone-signature` header:
// `hmac;1;<unix_ms>;<base64_hmac>`, signed data is `<timestamp>.<raw_body>`.
func VerifyWebhookSignature(signingSecretB64, sigHeader string, rawBody []byte, maxSkew time.Duration) error {
	secretB64 := strings.TrimSpace(signingSecretB64)
	if secretB64 == "" {
		return errors.New("missing webhook signing secret")
	}
	sigHeader = strings.TrimSpace(sigHeader)
	if sigHeader == "" {
		return errors.New("missing signature header")
	}
	parts := strings.Split(sigHeader, ";")
	if len(parts) != 4 || parts[0] != "hmac" || parts[1] != "1" {
		return errors.New("invalid signature header format")
	}
	tsStr := parts[2]
	wantB64 := parts[3]
	ms, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil || ms <= 0 {
		return errors.New("invalid signature timestamp")
	}
	if maxSkew > 0 {
		t := time.UnixMilli(ms)
		if d := time.Since(t); d > maxSkew || d < -maxSkew {
			return fmt.Errorf("signature timestamp outside allowed skew (%v)", maxSkew)
		}
	}
	key, err := base64.StdEncoding.DecodeString(secretB64)
	if err != nil {
		return fmt.Errorf("decode signing secret: %w", err)
	}
	signed := tsStr + "." + string(rawBody)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(signed))
	got := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(got), []byte(wantB64)) {
		return errors.New("signature mismatch")
	}
	return nil
}
