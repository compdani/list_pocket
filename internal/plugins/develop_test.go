package plugins

import (
	"errors"
	"strings"
	"testing"
)

func sourceFiles(version, server string) []SourceFile {
	return []SourceFile{
		{Path: "plugin.json", Content: `{"schemaVersion":1,"id":"built","name":"Built","version":"` + version + `",
		  "webhooks":[{"path":"open","verify":"none"}],
		  "tools":[{"name":"write","description":"Writes.","requiresApproval":false}]}`},
		{Path: "server.js", Content: server},
	}
}

const sourceServer = `plugin.webhook('open', { handle() {} }); plugin.tool('write', () => ({ ok: true }));`

func TestCheckSourceRejects(t *testing.T) {
	m, _ := bootedSample(t)
	good := sourceFiles("1.0.0", sourceServer)
	for name, tc := range map[string]struct {
		files []SourceFile
		want  string
	}{
		"nothing":      {nil, "needs 1-"},
		"escape":       {append(good[:2:2], SourceFile{"../evil.js", ""}), "relative path"},
		"absolute":     {append(good[:2:2], SourceFile{"/etc/evil.js", ""}), "relative path"},
		"backslash":    {append(good[:2:2], SourceFile{`a\b.js`, ""}), "relative path"},
		"binary ext":   {append(good[:2:2], SourceFile{"run.sh", ""}), "relative path"},
		"not text":     {append(good[:2:2], SourceFile{"a.js", "\x00"}), "isn't text"},
		"duplicate":    {append(good[:2:2], good[1]), "appears twice"},
		"too big":      {append(good[:2:2], SourceFile{"a.txt", strings.Repeat("x", maxSourceBytes+1)}), "over"},
		"manifest":     {good[1:], "plugin.json is missing"},
		"bad manifest": {[]SourceFile{{"plugin.json", `{}`}, good[1]}, "invalid plugin.json"},
		"no server":    {good[:1], "server.js is missing"},
		"syntax":       {sourceFiles("1.0.0", "function ("), "compile server.js"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := m.CheckSource(tc.files)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

// Checking and installing must never run a script: a person hasn't enabled it yet.
func TestSourceDoesNotRunBeforeEnable(t *testing.T) {
	m, _ := bootedSample(t)
	server := sourceServer + ` host.kv.set('ran', true); throw new Error('ran');`
	if _, err := m.CheckSource(sourceFiles("1.0.0", server)); err != nil {
		t.Fatalf("a script that would throw when run should still pass: %v", err)
	}
	res, err := m.InstallSource(sourceFiles("1.0.0", server))
	if err != nil {
		t.Fatal(err)
	}
	if res.NeedsRestart || m.Loaded("built") != nil {
		t.Fatalf("a new plugin must stay off: %+v", res)
	}
	rows, _ := m.List()
	for _, row := range rows {
		if row.Manifest.ID == "built" && (row.Enabled || row.Status != StatusDisabled) {
			t.Fatalf("a new plugin must stay disabled: %+v", row)
		}
	}
	if _, ran, err := m.store.kvGet("built", "ran"); err != nil || ran {
		t.Fatalf("the script ran: %v, %v", ran, err)
	}
}

func TestSourceWarningsAndUpgrade(t *testing.T) {
	m, _ := bootedSample(t)
	checked, err := m.CheckSource(sourceFiles("1.0.0", sourceServer))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(checked.Warnings, " ")
	if checked.Replaces != "" || !strings.Contains(joined, `Webhook "open"`) || !strings.Contains(joined, `Tool "write"`) {
		t.Fatalf("a new plugin's risks should be pointed out: %+v", checked)
	}

	if _, err := m.InstallSource(sourceFiles("1.0.0", sourceServer)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetEnabled("built", true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CheckSource(sourceFiles("1.0.0", sourceServer)); err == nil || !strings.Contains(err.Error(), "bump the version") {
		t.Fatalf("the running version can't be replaced: %v", err)
	}
	checked, err = m.CheckSource(sourceFiles("1.1.0", sourceServer))
	if err != nil {
		t.Fatal(err)
	}
	if checked.Replaces != "1.0.0" || !strings.Contains(strings.Join(checked.Warnings, " "), "next time the app restarts") {
		t.Fatalf("an upgrade should say what it replaces and when: %+v", checked)
	}
	res, err := m.InstallSource(sourceFiles("1.1.0", sourceServer))
	if err != nil || !res.NeedsRestart {
		t.Fatalf("upgrade: %+v, %v", res, err)
	}
}

func TestReadSource(t *testing.T) {
	m, host := bootedSample(t)
	if err := host.SavePluginSettings("sample", map[string]any{"secret": "s3cret"}); err != nil {
		t.Fatal(err)
	}
	src, err := m.ReadSource("sample", 0)
	if err != nil {
		t.Fatal(err)
	}
	if src.Version != "1.0.0" || len(src.Files) != 2 {
		t.Fatalf("source: %+v", src)
	}
	for _, f := range src.Files {
		if strings.Contains(f.Content, "s3cret") {
			t.Fatal("stored secrets aren't source")
		}
	}
	if _, err := m.ReadSource("gone", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}

	r := zipBytes(t, map[string]string{"plugin.json": `{"schemaVersion":1,"id":"withimage","name":"I","version":"1"}`, "server.js": "", "ui/logo.png": "\x89PNG"})
	if _, err := m.Install(r, r.Size()); err != nil {
		t.Fatal(err)
	}
	src, err = m.ReadSource("withimage", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Binary) != 1 || src.Binary[0] != "ui/logo.png" {
		t.Fatalf("other files should be listed, not read: %+v", src)
	}

	// A text-only upgrade must not lose them.
	manifest := `{"schemaVersion":1,"id":"withimage","name":"I","version":"2"}`
	if _, err := m.InstallSource([]SourceFile{{"plugin.json", manifest}, {"server.js", ""}}); err != nil {
		t.Fatal(err)
	}
	src, err = m.ReadSource("withimage", 0)
	if err != nil || src.Version != "2" || len(src.Binary) != 1 {
		t.Fatalf("the image should carry over: %+v, %v", src, err)
	}

	// A limit leaves out what doesn't fit, and says so.
	src, err = m.ReadSource("sample", 100)
	if err != nil || len(src.Omitted) == 0 {
		t.Fatalf("over the limit: %+v, %v", src, err)
	}
}
