package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/compdani/list_pocket/internal/config"
)

func TestInitSMTPMessengersKeepsCredentialsOutOfLogs(t *testing.T) {
	oldKo, oldOut := ko, lo.Writer()
	t.Cleanup(func() {
		ko = oldKo
		lo.SetOutput(oldOut)
	})

	ko = config.New()
	if err := ko.LoadMap(map[string]any{"smtp": []any{map[string]any{
		"enabled":       true,
		"name":          "primary",
		"host":          "smtp.example.com",
		"port":          587,
		"max_conns":     1,
		"username":      "AKIATESTUSERNAME",
		"password":      "test-smtp-password",
		"auth_protocol": "login",
		"tls_type":      "none",
	}}}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	lo.SetOutput(io.MultiWriter(&out, os.Stderr))

	initSMTPMessengers()

	logged := out.String()
	if !strings.Contains(logged, "smtp.example.com") || !strings.Contains(logged, "primary") {
		t.Fatalf("the log should still identify the server: %q", logged)
	}
	for _, secret := range []string{"AKIATESTUSERNAME", "test-smtp-password"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("%s reached the log: %q", secret, logged)
		}
	}
}
