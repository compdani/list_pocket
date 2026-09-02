package assets

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestRemapAndRead(t *testing.T) {
	embed := fstest.MapFS{
		"config.toml.sample":                &fstest.MapFile{Data: []byte("sample")},
		"permissions.json":                  &fstest.MapFile{Data: []byte("[]")},
		"static/email-templates/base.html":  &fstest.MapFile{Data: []byte("email")},
		"static/public/templates/home.html": &fstest.MapFile{Data: []byte("home")},
		"i18n/en.json":                      &fstest.MapFile{Data: []byte("{}")},
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "email-templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "email-templates", "base.html"), []byte("overlay"), 0o644); err != nil {
		t.Fatal(err)
	}

	fsys, err := New(embed, Opt{StaticDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	b, err := ReadFile(fsys, "/permissions.json")
	if err != nil || string(b) != "[]" {
		t.Fatalf("permissions: %s %v", b, err)
	}
	b, err = ReadFile(fsys, "public/templates/home.html")
	if err != nil || string(b) != "home" {
		t.Fatalf("public remap: %s %v", b, err)
	}
	b, err = ReadFile(fsys, "/static/email-templates/base.html")
	if err != nil || string(b) != "overlay" {
		t.Fatalf("static overlay: %s %v", b, err)
	}

	matches, err := Glob(fsys, "/i18n/*.json")
	if err != nil || len(matches) != 1 || matches[0] != "i18n/en.json" {
		t.Fatalf("glob=%v err=%v", matches, err)
	}

	sub, err := Sub(fsys, "/public/templates")
	if err != nil {
		t.Fatal(err)
	}
	f, err := sub.Open("home.html")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestDiskWinsOverEmbed(t *testing.T) {
	embed := fstest.MapFS{
		"i18n/en.json": &fstest.MapFile{Data: []byte(`{"embed":true}`)},
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"disk":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys, err := New(embed, Opt{I18nDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ReadFile(fsys, "i18n/en.json")
	if err != nil || string(b) != `{"disk":true}` {
		t.Fatalf("overlay: %s %v", b, err)
	}
}

func TestMissingOptionalDirs(t *testing.T) {
	embed := fstest.MapFS{
		"permissions.json": &fstest.MapFile{Data: []byte("[]")},
	}
	fsys, err := New(embed, Opt{FrontendDir: "nope", StaticDir: "nope", I18nDir: "nope"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile(fsys, "permissions.json"); err != nil {
		t.Fatal(err)
	}
}
