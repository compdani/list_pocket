package email

import "testing"

func TestParseSMTPTestRequest(t *testing.T) {
	body := []byte(`{
		"name": "email-debug-server",
		"uuid": "b62a0a86-a775-4136-961b-389fb8c374df",
		"enabled": true,
		"host": "sandbox.smtp.mailtrap.io",
		"hello_hostname": "",
		"port": 587,
		"auth_protocol": "login",
		"username": "960a9abd15b6b0",
		"password": "722fa668365ecc",
		"email_headers": [{"X-Env": "staging"}, {"X-Team": "dreia"}],
		"max_conns": 10,
		"max_msg_retries": 2,
		"idle_timeout": "15s",
		"wait_timeout": "5s",
		"tls_type": "STARTTLS",
		"tls_skip_verify": false,
		"strEmailHeaders": "[]",
		"email": " team@dreia.info "
	}`)

	req, to, from, err := ParseSMTPTestRequest(body)
	if err != nil {
		t.Fatalf("ParseSMTPTestRequest: %v", err)
	}

	if to != "team@dreia.info" {
		t.Fatalf("expected trimmed email, got %q", to)
	}

	if from != "" {
		t.Fatalf("expected empty default from email, got %q", from)
	}

	if req.Host != "sandbox.smtp.mailtrap.io" {
		t.Fatalf("expected host to be preserved, got %q", req.Host)
	}

	if req.MaxMessageRetries != 2 {
		t.Fatalf("expected max retries 2, got %d", req.MaxMessageRetries)
	}

	if req.IdleTimeout.String() != "15s" {
		t.Fatalf("expected idle timeout 15s, got %s", req.IdleTimeout)
	}

	if req.PoolWaitTimeout.String() != "5s" {
		t.Fatalf("expected wait timeout 5s, got %s", req.PoolWaitTimeout)
	}

	if req.EmailHeaders["X-Env"] != "staging" || req.EmailHeaders["X-Team"] != "dreia" {
		t.Fatalf("expected flattened headers, got %#v", req.EmailHeaders)
	}
}

func TestParseSMTPTestRequestRejectsBadDuration(t *testing.T) {
	body := []byte(`{
		"host": "sandbox.smtp.mailtrap.io",
		"port": 587,
		"auth_protocol": "login",
		"username": "960a9abd15b6b0",
		"password": "722fa668365ecc",
		"email_headers": [],
		"max_conns": 10,
		"max_msg_retries": 2,
		"idle_timeout": "not-a-duration",
		"wait_timeout": "5s",
		"tls_type": "STARTTLS",
		"tls_skip_verify": false,
		"email": "team@dreia.info"
	}`)

	if _, _, _, err := ParseSMTPTestRequest(body); err == nil {
		t.Fatal("expected bad duration error")
	}
}
