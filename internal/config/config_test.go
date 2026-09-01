package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestEnvKey(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"LISTPOCKET_APP__ADDRESS", "app.address"},
		{"LISTPOCKET_static_dir", "static-dir"},
		{"LISTPOCKET_I18N_DIR", "i18n-dir"},
		{"LISTPOCKET_db__ssl_mode", "db.ssl_mode"},
	}
	for _, tt := range tests {
		if got := EnvKey(tt.in, "LISTPOCKET_"); got != tt.want {
			t.Fatalf("EnvKey(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMergeOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.toml")
	if err := os.WriteFile(path, []byte("[app]\naddress = \"from-file\"\nlang = \"en\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	fs.String("app.address", "from-flag", "")
	fs.String("static-dir", "from-flag-static", "")
	if err := fs.Parse([]string{"--app.address=from-flag", "--static-dir=from-flag-static"}); err != nil {
		t.Fatal(err)
	}

	c := New()
	if err := c.LoadFlags(fs); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadTOMLFile(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LISTPOCKET_APP__ADDRESS", "from-env")
	t.Setenv("LISTPOCKET_APP__LANG", "fr")
	if err := c.LoadEnv("LISTPOCKET_"); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadMap(map[string]any{"app.lang": "es"}); err != nil {
		t.Fatal(err)
	}

	if got := c.String("app.address"); got != "from-env" {
		t.Fatalf("address=%q, want from-env (env over file over flags)", got)
	}
	if got := c.String("app.lang"); got != "es" {
		t.Fatalf("lang=%q, want es (map over env)", got)
	}
	if got := c.String("static-dir"); got != "from-flag-static" {
		t.Fatalf("static-dir=%q, want from-flag-static", got)
	}
}

func TestSlicesAndUnmarshalJSON(t *testing.T) {
	c := New()
	if err := c.LoadMap(map[string]any{
		"smtp": []any{
			map[string]any{"enabled": true, "host": "a.example", "port": float64(25)},
			map[string]any{"enabled": false, "host": "b.example"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	items := c.Slices("smtp")
	if len(items) != 2 {
		t.Fatalf("len(slices)=%d", len(items))
	}
	if !items[0].Bool("enabled") || items[1].Bool("enabled") {
		t.Fatalf("enabled flags: %v %v", items[0].Bool("enabled"), items[1].Bool("enabled"))
	}

	var s struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := items[0].UnmarshalJSONTag("", &s); err != nil {
		t.Fatal(err)
	}
	if s.Host != "a.example" || s.Port != 25 {
		t.Fatalf("unmarshaled %#v", s)
	}
}

func TestUnmarshalFlatAndDuration(t *testing.T) {
	c := New()
	if err := c.LoadMap(map[string]any{
		"appearance.admin.custom_css":         "body{}",
		"app.message_sliding_window_duration": "15s",
	}); err != nil {
		t.Fatal(err)
	}

	var appearance struct {
		AdminCSS []byte `config:"admin.custom_css"`
	}
	if err := c.UnmarshalFlat("appearance", &appearance); err != nil {
		t.Fatal(err)
	}
	if string(appearance.AdminCSS) != "body{}" {
		t.Fatalf("css=%q", appearance.AdminCSS)
	}
	if c.Duration("app.message_sliding_window_duration") != 15*time.Second {
		t.Fatalf("duration=%s", c.Duration("app.message_sliding_window_duration"))
	}
}

func TestSetExistsBool(t *testing.T) {
	c := New()
	if c.Exists("app.lang") {
		t.Fatal("expected missing")
	}
	if err := c.Set("app.lang", "en"); err != nil {
		t.Fatal(err)
	}
	if !c.Exists("app.lang") || c.String("app.lang") != "en" {
		t.Fatalf("lang=%q exists=%v", c.String("app.lang"), c.Exists("app.lang"))
	}
	if err := c.Set("passive", true); err != nil {
		t.Fatal(err)
	}
	if !c.Bool("passive") {
		t.Fatal("expected passive true")
	}
}

func TestLoadTOMLFileMissing(t *testing.T) {
	c := New()
	if err := c.LoadTOMLFile("no-such-file.toml"); err == nil {
		t.Fatal("expected error")
	}
}
