package main

import (
	"reflect"
	"testing"
)

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

func TestStripListPocketFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"docker serve --config", []string{"serve", "--config", "/app/config.toml"}, []string{"serve"}},
		{"config equals form", []string{"--config=/app/config.toml", "serve"}, []string{"serve"}},
		{"keeps pb flags", []string{"serve", "--http=0.0.0.0:9000", "--passive"}, []string{"serve", "--http=0.0.0.0:9000"}},
		{"static-dir and i18n-dir", []string{"--static-dir", "s", "--i18n-dir=i", "serve"}, []string{"serve"}},
		{"stop at double dash", []string{"--config", "c.toml", "--", "--config"}, []string{"--", "--config"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripListPocketFlags(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
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
