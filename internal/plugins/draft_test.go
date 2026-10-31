package plugins

import (
	"strings"
	"testing"
)

func TestDraftGrowsOverSeveralWritesAndSurvivesRestart(t *testing.T) {
	m, host := bootedSample(t)
	files := sourceFiles("1.0.0", sourceServer)
	if _, err := m.WriteDraft("built", files[:1], nil, false); err != nil {
		t.Fatal(err)
	}
	info, err := m.WriteDraft("built", files[1:], nil, false)
	if err != nil || len(info.Files) != 2 || info.BaseVersion != "" {
		t.Fatalf("draft: %+v, %v", info, err)
	}

	// Enabling a plugin restarts the app, and a draft has to outlast that.
	m = New(m.store.app, Options{Dir: m.dir, Log: m.log, Host: host})
	got, err := m.DraftFiles("built")
	if err != nil || len(got) != 2 {
		t.Fatalf("after a restart: %+v, %v", got, err)
	}
	if _, err := m.CheckSource(got); err != nil {
		t.Fatalf("a finished draft should check out: %v", err)
	}
	if m.Loaded("built") != nil {
		t.Fatal("a draft must not load")
	}
	if rows, _ := m.List(); len(rows) != 1 {
		t.Fatalf("a draft must not count as installed: %+v", rows)
	}

	if _, err := m.WriteDraft("built", nil, []string{"server.js"}, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.DraftFiles("built"); len(got) != 1 {
		t.Fatalf("remove: %+v", got)
	}
}

func TestDraftRejectsBadInput(t *testing.T) {
	m, _ := bootedSample(t)
	for name, err := range map[string]error{
		"traversal id": func() error { _, err := m.WriteDraft("../evil", nil, nil, false); return err }(),
		"reserved id":  func() error { _, err := m.WriteDraft("whatsapp", nil, nil, false); return err }(),
		"bad path":     func() error { _, err := m.WriteDraft("built", []SourceFile{{"../x.js", ""}}, nil, false); return err }(),
		"bad ext":      func() error { _, err := m.WriteDraft("built", []SourceFile{{"run.sh", ""}}, nil, false); return err }(),
		"too big": func() error {
			_, err := m.WriteDraft("built", []SourceFile{{"a.txt", strings.Repeat("x", maxSourceBytes+1)}}, nil, false)
			return err
		}(),
		"remove gone": func() error { _, err := m.WriteDraft("built", nil, []string{"gone.js"}, false); return err }(),
		"no draft":    func() error { _, err := m.DraftFiles("built"); return err }(),
	} {
		if err == nil {
			t.Errorf("%s: should fail", name)
		}
	}
}

func TestEditDraftIsExactAndAllOrNothing(t *testing.T) {
	m, _ := bootedSample(t)
	content := "one\ntwo\ntwo\nthree\n"
	if _, err := m.WriteDraft("built", []SourceFile{{"a.js", content}}, nil, false); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		edits []DraftEdit
		want  string
	}{
		"missing":   {[]DraftEdit{{"nope", "x"}}, "isn't in"},
		"ambiguous": {[]DraftEdit{{"two", "x"}}, "appears 2 times"},
		"empty":     {[]DraftEdit{{"", "x"}}, "must not be empty"},
		"second":    {[]DraftEdit{{"one", "ONE"}, {"nope", "x"}}, "edit 2"},
	} {
		if _, err := m.EditDraft("built", "a.js", tc.edits); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
	got, _ := m.DraftFiles("built")
	if got[0].Content != content {
		t.Fatalf("a failed edit must change nothing: %q", got[0].Content)
	}
	if _, err := m.EditDraft("built", "a.js", []DraftEdit{{"one", "ONE"}, {"two\nthree", "2\n3"}}); err != nil {
		t.Fatal(err)
	}
	got, _ = m.DraftFiles("built")
	if got[0].Content != "ONE\ntwo\n2\n3\n" {
		t.Fatalf("edits: %q", got[0].Content)
	}
	if _, err := m.EditDraft("built", "gone.js", []DraftEdit{{"a", "b"}}); err == nil {
		t.Fatal("a file that isn't there should fail")
	}
}

func TestDraftOfInstalledPluginStartsFromIt(t *testing.T) {
	m, _ := bootedSample(t)
	if _, err := m.InstallSource(sourceFiles("1.0.0", "// old\n"+sourceServer)); err != nil {
		t.Fatal(err)
	}
	listing, err := m.ListPluginFiles("built")
	if err != nil || listing.Source != "installed" || listing.BaseVersion != "1.0.0" {
		t.Fatalf("listing: %+v, %v", listing, err)
	}

	// One small edit, with no need to resend any file.
	if _, err := m.EditDraft("built", "server.js", []DraftEdit{{"// old", "// new"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EditDraft("built", "plugin.json", []DraftEdit{{`"version":"1.0.0"`, `"version":"1.1.0"`}}); err != nil {
		t.Fatal(err)
	}
	listing, _ = m.ListPluginFiles("built")
	if listing.Source != "draft" {
		t.Fatalf("listing: %+v", listing)
	}
	read, err := m.ReadPluginFile("built", "server.js", 1, 1)
	if err != nil || read.Source != "draft" || read.Content != "// new\n" || read.EndLine != 1 {
		t.Fatalf("read: %+v, %v", read, err)
	}

	// The installed plugin isn't touched until a proposal is approved.
	installed, _ := m.ReadSource("built", 0)
	for _, f := range installed.Files {
		if f.Path == "server.js" && !strings.Contains(f.Content, "// old") {
			t.Fatal("editing a draft changed the installed plugin")
		}
	}

	files, err := m.DraftFiles("built")
	if err != nil {
		t.Fatal(err)
	}
	if res, err := m.InstallSource(files); err != nil || res.Manifest.Version != "1.1.0" {
		t.Fatalf("install: %+v, %v", res, err)
	}
}

func TestDraftCantUndoANewerInstall(t *testing.T) {
	m, _ := bootedSample(t)
	if _, err := m.InstallSource(sourceFiles("1.0.0", sourceServer)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.WriteDraft("built", []SourceFile{{"extra.md", "x"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.InstallSource(sourceFiles("2.0.0", sourceServer)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.DraftFiles("built"); err == nil || !strings.Contains(err.Error(), "installed after this draft") {
		t.Fatalf("a stale draft should be refused: %v", err)
	}
	if _, err := m.WriteDraft("built", nil, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.DraftFiles("built"); err != nil {
		t.Fatalf("starting over should clear that: %v", err)
	}
}

func TestReadPluginFileLongFilesAndLimits(t *testing.T) {
	m, _ := bootedSample(t)
	line := strings.Repeat("x", 1023) + "\n"
	if _, err := m.WriteDraft("built", []SourceFile{{"big.txt", strings.Repeat(line, 600)}}, nil, false); err != nil {
		t.Fatal(err)
	}
	read, err := m.ReadPluginFile("built", "big.txt", 0, 0)
	if err != nil || !read.Truncated || read.EndLine != 256 || len(read.Content) != 256*1024 {
		t.Fatalf("one read should stop at the limit: end=%d truncated=%v bytes=%d, %v", read.EndLine, read.Truncated, len(read.Content), err)
	}
	read, err = m.ReadPluginFile("built", "big.txt", read.EndLine+1, 300)
	if err != nil || read.Truncated || read.StartLine != 257 || read.EndLine != 300 {
		t.Fatalf("continuing: %+v, %v", read.EndLine, err)
	}
	if _, err := m.ReadPluginFile("built", "big.txt", 9999, 0); err == nil {
		t.Fatal("past the end should fail")
	}
	if _, err := m.ReadPluginFile("built", "gone.txt", 0, 0); err == nil {
		t.Fatal("a missing file should fail")
	}
}
