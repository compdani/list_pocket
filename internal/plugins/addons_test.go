package plugins

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const addonManifest = `{
  "schemaVersion": 1, "id": "sample", "name": "Sample", "version": "1.0.0",
  "server": {"concurrency": 1, "timeoutMs": 300},
  "ui": {"entry": "ui/index.js"},
  "settings": [{"key": "secret", "type": "secret", "label": "Secret"}],
  "webhooks": [{"path": "hook", "verify": "none"}],
  "tools": [
    {"name": "echo", "description": "Echoes.", "readOnly": true, "inputSchema": {"type": "object", "properties": {"color": {"type": "string", "x-ui": "sample.color"}}}},
    {"name": "write", "description": "Changes something."},
    {"name": "unchecked", "description": "Low risk.", "requiresApproval": false},
    {"name": "boom", "description": "Throws."},
    {"name": "spin", "description": "Never ends."},
    {"name": "nothing", "description": "Returns nothing."},
    {"name": "unserializable", "description": "Returns a loop."}
  ],
  "events": [{"name": "happened", "description": "It happened. event: {n}"}],
  "agentPresets": [{"name": "helper", "description": "Helps.", "systemPrompt": "Help.", "tools": ["sample.echo"], "maxSteps": 3}],
  "fieldEditors": [{"name": "color"}]
}`

const addonServer = `
plugin.tool('echo', (input, ctx) => ({ input, config: ctx.config, run: ctx.runId }));
plugin.tool('write', () => ({ ok: true }));
plugin.tool('unchecked', () => ({ ok: true }));
plugin.tool('boom', () => { throw new Error('failed with ' + host.settings.get().secret); });
plugin.tool('spin', () => { while (true) {} });
plugin.tool('nothing', () => {});
plugin.tool('unserializable', () => { const a = {}; a.self = a; return a; });
plugin.webhook('hook', { handle(req) {
  if (req.query.undeclared) host.workflows.emit('other', {});
  host.workflows.emit('happened', { n: 1, nested: { list: [1, 2] } });
} });
`

func addonManager(t *testing.T) *Manager {
	t.Helper()
	app := testApp(t)
	host := &fakeHost{}
	m := testManager(t, app, host, false)
	if err := m.Boot(); err != nil {
		t.Fatal(err)
	}
	r := zipBytes(t, map[string]string{"plugin.json": addonManifest, "server.js": addonServer, "ui/index.js": "export default () => {}"})
	if _, err := m.Install(r, r.Size()); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := m.SetEnabled("sample", true); err != nil {
		t.Fatal(err)
	}
	if err := host.SavePluginSettings("sample", map[string]any{"secret": "s3cret"}); err != nil {
		t.Fatal(err)
	}
	m = testManager(t, app, host, false)
	if err := m.Boot(); err != nil {
		t.Fatal(err)
	}
	if m.Loaded("sample") == nil {
		rows, _ := m.List()
		t.Fatalf("sample did not load: %+v", rows[0])
	}
	return m
}

func TestAddonManifestRejects(t *testing.T) {
	base := `"schemaVersion":1,"id":"sample","name":"S","version":"1"`
	for name, tc := range map[string]struct{ manifest, want string }{
		"tool name":         {`"tools":[{"name":"Bad Name","description":"d"}]`, "invalid or duplicate tool"},
		"duplicate tool":    {`"tools":[{"name":"a","description":"d"},{"name":"a","description":"d"}]`, "invalid or duplicate tool"},
		"no description":    {`"tools":[{"name":"a"}]`, "needs a description"},
		"schema not obj":    {`"tools":[{"name":"a","description":"d","inputSchema":{"type":"string"}}]`, "object schema"},
		"long wire name":    {`"tools":[{"name":"` + strings.Repeat("a", 60) + `","description":"d"}]`, "characters together"},
		"own editor gone":   {`"tools":[{"name":"a","description":"d","inputSchema":{"type":"object","properties":{"x":{"x-ui":"sample.gone"}}}}]`, "not provided"},
		"bad widget":        {`"tools":[{"name":"a","description":"d","inputSchema":{"type":"object","properties":{"x":{"x-ui":"wysiwyg"}}}}]`, "unknown x-ui widget"},
		"event name":        {`"events":[{"name":"a.b","description":"d"}]`, "invalid or duplicate event"},
		"event no desc":     {`"events":[{"name":"a"}]`, "needs a description"},
		"preset no prompt":  {`"agentPresets":[{"name":"a"}]`, "needs a systemPrompt"},
		"preset steps":      {`"agentPresets":[{"name":"a","systemPrompt":"p","maxSteps":99}]`, "maxSteps"},
		"preset schema":     {`"agentPresets":[{"name":"a","systemPrompt":"p","outputSchema":{}}]`, "needs a type"},
		"editor without ui": {`"fieldEditors":[{"name":"a"}]`, "need a ui entry"},
		"editor name":       {`"ui":{"entry":"ui/i.js"},"fieldEditors":[{"name":"A"}]`, "invalid or duplicate field editor"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseManifest([]byte("{" + base + "," + tc.manifest + "}"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}

	// Another plugin's editor may just not be enabled.
	if _, err := ParseManifest([]byte(`{` + base + `,"tools":[{"name":"a","description":"d","inputSchema":{"type":"object","properties":{"x":{"x-ui":"other.editor"}}}}]}`)); err != nil {
		t.Fatalf("another plugin's editor: %v", err)
	}
}

func TestToolApprovalDefaults(t *testing.T) {
	man, err := ParseManifest([]byte(addonManifest))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"echo": false, "write": true, "unchecked": false}
	for _, tool := range man.Tools {
		if needed, ok := want[tool.Name]; ok && tool.NeedsApproval() != needed {
			t.Errorf("%s: NeedsApproval = %v, want %v", tool.Name, tool.NeedsApproval(), needed)
		}
	}
}

func TestToolHandlerIsRequired(t *testing.T) {
	app := testApp(t)
	host := &fakeHost{}
	m := testManager(t, app, host, false)
	if err := m.Boot(); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schemaVersion":1,"id":"sample","name":"S","version":"1","tools":[{"name":"a","description":"d"}]}`
	r := zipBytes(t, map[string]string{"plugin.json": manifest, "server.js": ""})
	if _, err := m.Install(r, r.Size()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetEnabled("sample", true); err != nil {
		t.Fatal(err)
	}
	m = testManager(t, app, host, false)
	if err := m.Boot(); err != nil {
		t.Fatal(err)
	}
	rows, _ := m.List()
	if m.Loaded("sample") != nil || !strings.Contains(rows[0].LastError, `tool "a" has no handler`) {
		t.Fatalf("should fail to load: %+v", rows[0])
	}
}

func TestCallTool(t *testing.T) {
	m := addonManager(t)
	p := m.Loaded("sample")
	ctx := context.Background()

	res, err := p.CallTool(ctx, "echo", map[string]any{"color": "red"}, map[string]any{"k": "v"}, "run1")
	if err != nil {
		t.Fatal(err)
	}
	out := res.(map[string]any)
	if out["input"].(map[string]any)["color"] != "red" || out["config"].(map[string]any)["k"] != "v" || out["run"] != "run1" {
		t.Fatalf("handler didn't get its arguments: %+v", out)
	}

	if res, err := p.CallTool(ctx, "nothing", nil, nil, ""); err != nil || res != nil {
		t.Fatalf("no result: %v, %v", res, err)
	}

	_, err = p.CallTool(ctx, "boom", nil, nil, "")
	if err == nil || !strings.Contains(err.Error(), "failed with") || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("a thrown error must arrive without the secret: %v", err)
	}

	if _, err = p.CallTool(ctx, "unserializable", nil, nil, ""); err == nil {
		t.Fatal("a result that isn't JSON should fail")
	}

	if _, err = p.CallTool(ctx, "spin", nil, nil, ""); !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v, want a timeout", err)
	}
	// The pool replaced the stuck VM, so the plugin still works.
	if _, err = p.CallTool(ctx, "write", nil, nil, ""); err != nil {
		t.Fatalf("after a timeout: %v", err)
	}
}

func TestEmitEvent(t *testing.T) {
	m := addonManager(t)

	// Nothing is listening yet, which is an error rather than a lost event.
	if rec := webhookRequest(t, m, "POST", "sample", "hook", "/", "", nil); rec.Code != 500 {
		t.Fatalf("emit with no engine: %d", rec.Code)
	}

	var names []string
	var payload map[string]any
	m.SetEventSink(func(name string, p map[string]any) { names = append(names, name); payload = p })
	if rec := webhookRequest(t, m, "POST", "sample", "hook", "/", "", nil); rec.Code != 200 {
		t.Fatalf("emit: %d %s", rec.Code, rec.Body)
	}
	if len(names) != 1 || names[0] != "sample.happened" {
		t.Fatalf("events: %v", names)
	}
	// Numbers and lists arrive as plain JSON, as the engine stores them.
	if payload["n"] != float64(1) || len(payload["nested"].(map[string]any)["list"].([]any)) != 2 {
		t.Fatalf("payload: %#v", payload)
	}

	names = nil
	if rec := webhookRequest(t, m, "POST", "sample", "hook", "/?undeclared=1", "", nil); rec.Code != 500 || len(names) != 0 {
		t.Fatalf("an undeclared event must be refused: %d %v", rec.Code, names)
	}
}

func TestPresets(t *testing.T) {
	m := addonManager(t)
	preset, ok := m.Preset("sample.helper")
	if !ok || preset["system_prompt"] != "Help." || preset["max_steps"] != 3 {
		t.Fatalf("preset: %+v, %v", preset, ok)
	}
	for _, name := range []string{"sample.gone", "other.helper", "helper", ""} {
		if _, ok := m.Preset(name); ok {
			t.Errorf("%q shouldn't be found", name)
		}
	}
	if got := m.Presets(); len(got) != 1 || got[0].Name != "sample.helper" || got[0].Description != "Helps." {
		t.Fatalf("presets: %+v", got)
	}
}

func TestFieldEditorsReachTheBrowser(t *testing.T) {
	m := addonManager(t)
	ui := m.UIManifests(false)
	if len(ui) != 1 || len(ui[0].FieldEditors) != 1 || ui[0].FieldEditors[0] != "color" {
		t.Fatalf("ui manifests: %+v", ui)
	}
}
