package main

import "testing"

func TestFindPocketBaseCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantIdx int
		wantCmd string
	}{
		{"empty", nil, -1, ""},
		{"flags only", []string{"--passive", "--config", "config.toml"}, -1, ""},
		{"serve", []string{"serve"}, 0, "serve"},
		{"serve after flags", []string{"--passive", "serve", "--http=1"}, 1, "serve"},
		{"migrate", []string{"migrate", "up"}, 0, "migrate"},
		{"config value not command", []string{"--config", "config.toml", "migrate"}, 2, "migrate"},
		{"unknown positional", []string{"config.toml"}, -1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, cmd := findPocketBaseCommand(tt.args)
			if idx != tt.wantIdx || cmd != tt.wantCmd {
				t.Fatalf("got (%d, %q), want (%d, %q)", idx, cmd, tt.wantIdx, tt.wantCmd)
			}
		})
	}
}

func TestHasCLIFlag(t *testing.T) {
	if !hasCLIFlag([]string{"--http=127.0.0.1:9000"}, "http") {
		t.Fatal("expected --http= form")
	}
	if !hasCLIFlag([]string{"--http", "127.0.0.1:9000"}, "http") {
		t.Fatal("expected --http form")
	}
	if hasCLIFlag([]string{"--https=1"}, "http") {
		t.Fatal("https should not match http")
	}
}
