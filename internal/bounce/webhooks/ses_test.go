package webhooks

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/compdani/list_pocket/models"
)

func makeSignedSESNotification(t *testing.T, s *SES, notifType string, mailHeaders []map[string]string, headersTruncated bool) []byte {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	certTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "ses-test",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	certURL := "https://sns.us-east-1.amazonaws.com/SimpleNotificationService-abcd1234.pem"
	s.certs["/SimpleNotificationService-abcd1234.pem"] = cert

	mailPayload := map[string]any{
		"eventType":        notifType,
		"notificationType": notifType,
		"bounce": map[string]any{
			"bounceType": "Permanent",
			"bouncedRecipients": []map[string]any{
				{"status": "5.1.1"},
			},
		},
		"mail": map[string]any{
			"timestamp":        "2026-04-22T10:15:30.123456789Z",
			"headersTruncated": headersTruncated,
			"destination":      []string{"User@Example.com"},
			"headers":          mailHeaders,
		},
	}

	mailJSON, err := json.Marshal(mailPayload)
	if err != nil {
		t.Fatalf("marshal mail payload: %v", err)
	}

	n := sesNotif{
		Message:          string(mailJSON),
		MessageId:        "sns-message-id-1",
		SigningCertURL:   certURL,
		Timestamp:        "2026-04-22T10:15:31.000Z",
		TopicArn:         "arn:aws:sns:us-east-1:123456789012:test-topic",
		Type:             "Notification",
		SignatureVersion: "1",
	}

	digest := sha1.Sum(s.buildSignature(n))
	signature, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA1, digest[:])
	if err != nil {
		t.Fatalf("sign notification: %v", err)
	}
	n.Signature = base64.StdEncoding.EncodeToString(signature)

	body, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal notification envelope: %v", err)
	}
	return body
}

func TestSESProcessBounceMixedIdentifierPayloads(t *testing.T) {
	tests := []struct {
		name            string
		headers         []map[string]string
		headersCut      bool
		expectCampaign  string
		expectMessageID string
	}{
		{
			name: "campaign record id with message id",
			headers: []map[string]string{
				{"name": models.EmailHeaderCampaignUUID, "value": "camprecid01234567"},
				{"name": models.EmailHeaderMessageId, "value": "<camprecid01234567@listpocket.local>"},
			},
			expectCampaign:  "camprecid01234567",
			expectMessageID: "camprecid01234567@listpocket.local",
		},
		{
			name: "campaign uuid with message id",
			headers: []map[string]string{
				{"name": models.EmailHeaderCampaignUUID, "value": "550e8400-e29b-41d4-a716-446655440000"},
				{"name": "message-id", "value": "<550e8400-e29b-41d4-a716-446655440000@listpocket.local>"},
			},
			expectCampaign:  "550e8400-e29b-41d4-a716-446655440000",
			expectMessageID: "550e8400-e29b-41d4-a716-446655440000@listpocket.local",
		},
		{
			name: "campaign record id without message id",
			headers: []map[string]string{
				{"name": models.EmailHeaderCampaignUUID, "value": "camprecid98765432"},
			},
			expectCampaign:  "camprecid98765432",
			expectMessageID: "",
		},
		{
			name: "campaign uuid without message id",
			headers: []map[string]string{
				{"name": models.EmailHeaderCampaignUUID, "value": "f47ac10b-58cc-4372-a567-0e02b2c3d479"},
			},
			expectCampaign:  "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			expectMessageID: "",
		},
		{
			name: "headers truncated drops campaign and message ids",
			headers: []map[string]string{
				{"name": models.EmailHeaderCampaignUUID, "value": "camprecid01234567"},
				{"name": models.EmailHeaderMessageId, "value": "<camprecid01234567@listpocket.local>"},
			},
			headersCut:      true,
			expectCampaign:  "",
			expectMessageID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSES()
			payload := makeSignedSESNotification(t, s, "Bounce", tt.headers, tt.headersCut)

			out, err := s.ProcessBounce(payload)
			if err != nil {
				t.Fatalf("ProcessBounce returned error: %v", err)
			}
			if out.Source != "ses" {
				t.Fatalf("source=%q, want ses", out.Source)
			}
			if out.Type != models.BounceTypeHard {
				t.Fatalf("type=%q, want %q", out.Type, models.BounceTypeHard)
			}
			if out.Email != "user@example.com" {
				t.Fatalf("email=%q, want user@example.com", out.Email)
			}
			if out.CampaignUUID != tt.expectCampaign {
				t.Fatalf("campaign=%q, want %q", out.CampaignUUID, tt.expectCampaign)
			}
			if out.MessageID != tt.expectMessageID {
				t.Fatalf("message_id=%q, want %q", out.MessageID, tt.expectMessageID)
			}
		})
	}
}
