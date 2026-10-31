package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/compdani/list_pocket/internal/flow/schema"
	"github.com/dop251/goja"
)

const (
	// maxAddonBytes caps a tool's result and an event's payload.
	maxAddonBytes = 1 << 20
	// A tool's name reaches the model as <plugin>__<tool>, and providers allow 64 characters.
	maxWireToolName = 64
	maxPresetSteps  = 50
)

// ToolManifest declares a tool workflows and agents can use, as <plugin>.<name>.
type ToolManifest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	ReadOnly    bool           `json:"readOnly,omitempty"`
	// RequiresApproval defaults to true unless the tool is read-only.
	RequiresApproval *bool `json:"requiresApproval,omitempty"`
}

// NeedsApproval reports whether a person must approve each call.
func (t ToolManifest) NeedsApproval() bool {
	return t.RequiresApproval != nil && *t.RequiresApproval
}

// EventManifest declares an event workflows can start on or wait for, as <plugin>.<name>.
type EventManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PresetManifest declares a ready-made agent, as <plugin>.<name>. A workflow's own
// settings on the node override it.
type PresetManifest struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	SystemPrompt string         `json:"systemPrompt"`
	Tools        []string       `json:"tools,omitempty"`
	MaxSteps     int            `json:"maxSteps,omitempty"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	AI           map[string]any `json:"ai,omitempty"`
}

// FieldEditorManifest declares a browser component that edits one tool input, picked
// with "x-ui": "<plugin>.<name>". The UI module registers it with host.fields.add.
type FieldEditorManifest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (m *Manifest) normalizeAddons() error {
	if err := m.normalizeFieldEditors(); err != nil {
		return err
	}
	if err := m.normalizeTools(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, e := range m.Events {
		if !reKey.MatchString(e.Name) || seen[e.Name] {
			return fmt.Errorf("invalid or duplicate event %q", e.Name)
		}
		seen[e.Name] = true
		if strings.TrimSpace(e.Description) == "" {
			return fmt.Errorf("event %q needs a description", e.Name)
		}
	}
	seen = map[string]bool{}
	for _, p := range m.AgentPresets {
		if !reKey.MatchString(p.Name) || seen[p.Name] {
			return fmt.Errorf("invalid or duplicate agent preset %q", p.Name)
		}
		seen[p.Name] = true
		if strings.TrimSpace(p.SystemPrompt) == "" {
			return fmt.Errorf("agent preset %q needs a systemPrompt", p.Name)
		}
		if p.MaxSteps < 0 || p.MaxSteps > maxPresetSteps {
			return fmt.Errorf("agent preset %q maxSteps must be 0-%d", p.Name, maxPresetSteps)
		}
		if p.OutputSchema != nil && p.OutputSchema["type"] == nil {
			return fmt.Errorf("agent preset %q outputSchema needs a type", p.Name)
		}
	}
	return nil
}

func (m *Manifest) normalizeFieldEditors() error {
	seen := map[string]bool{}
	for _, f := range m.FieldEditors {
		if !schema.IsPluginField(m.ID+"."+f.Name) || seen[f.Name] {
			return fmt.Errorf("invalid or duplicate field editor %q", f.Name)
		}
		seen[f.Name] = true
	}
	if len(m.FieldEditors) > 0 && m.UI == nil {
		return errors.New("field editors need a ui entry")
	}
	return nil
}

func (m *Manifest) normalizeTools() error {
	// Another plugin's editor may simply not be enabled, which the UI handles by
	// falling back to a plain input. A missing editor of this plugin's own is a mistake.
	knownField := func(widget string) bool {
		own, name, _ := strings.Cut(widget, ".")
		return own != m.ID || m.declaresFieldEditor(name)
	}
	seen := map[string]bool{}
	for i := range m.Tools {
		t := &m.Tools[i]
		if !reKey.MatchString(t.Name) || seen[t.Name] {
			return fmt.Errorf("invalid or duplicate tool %q", t.Name)
		}
		seen[t.Name] = true
		if len(m.ID)+len("__")+len(t.Name) > maxWireToolName {
			return fmt.Errorf("tool %q: the plugin id and tool name can be %d characters together", t.Name, maxWireToolName-len("__"))
		}
		if strings.TrimSpace(t.Description) == "" {
			return fmt.Errorf("tool %q needs a description", t.Name)
		}
		if t.InputSchema == nil {
			t.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		if t.InputSchema["type"] != "object" {
			return fmt.Errorf("tool %q inputSchema must be an object schema", t.Name)
		}
		if errs := schema.CheckWidgets(t.InputSchema, knownField); len(errs) > 0 {
			return fmt.Errorf("tool %q inputSchema: %s", t.Name, errs[0])
		}
		if t.RequiresApproval == nil {
			needed := !t.ReadOnly
			t.RequiresApproval = &needed
		}
	}
	return nil
}

func (m *Manifest) declaresFieldEditor(name string) bool {
	for _, f := range m.FieldEditors {
		if f.Name == name {
			return true
		}
	}
	return false
}

func (v *vm) declaresTool(name string) bool {
	for _, t := range v.plugin.Manifest.Tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func (v *vm) declaresEvent(name string) bool {
	for _, e := range v.plugin.Manifest.Events {
		if e.Name == name {
			return true
		}
	}
	return false
}

// hostWorkflows is host.workflows.
func (v *vm) hostWorkflows() *goja.Object {
	obj := v.rt.NewObject()
	_ = obj.Set("emit", func(name string, payload map[string]any) {
		if !v.declaresEvent(name) {
			v.throw(fmt.Errorf("event %q isn't declared in %s", name, manifestFile))
		}
		sink := v.plugin.mgr.eventSink()
		if sink == nil {
			v.throw(errors.New("workflows aren't running yet; emit events from a handler, not while the script loads"))
		}
		clean, err := plainJSON(payload)
		if err != nil {
			v.throw(fmt.Errorf("event %q: %w", name, err))
		}
		out, _ := clean.(map[string]any)
		if out == nil {
			out = map[string]any{}
		}
		sink(v.plugin.AddonName(name), out)
	})
	return obj
}

// plainJSON turns a value from a script into plain JSON values, which is what the
// workflow engine stores and its expressions expect.
func plainJSON(val any) (any, error) {
	raw, err := json.Marshal(val)
	if err != nil {
		return nil, fmt.Errorf("the value can't be converted to JSON: %w", err)
	}
	if len(raw) > maxAddonBytes {
		return nil, fmt.Errorf("the value is over %d bytes", maxAddonBytes)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AddonName is the catalog name of one of this plugin's add-ons.
func (p *Plugin) AddonName(name string) string {
	return p.Manifest.ID + "." + name
}

// CallTool runs one of the plugin's tools. input has already been checked against the
// tool's schema. Handlers get (input, { config, runId }).
func (p *Plugin) CallTool(ctx context.Context, name string, input, config map[string]any, runID string) (any, error) {
	if input == nil {
		input = map[string]any{}
	}
	if config == nil {
		config = map[string]any{}
	}
	ctx, cancel := contextWithTimeout(ctx, acquireTimeout+p.timeout)
	defer cancel()
	res, err := p.call(ctx, func(v *vm) (goja.Value, error) {
		handler, ok := v.tools[name]
		if !ok {
			return nil, fmt.Errorf("tool %q has no handler", name)
		}
		call := v.rt.NewObject()
		_ = call.Set("config", config)
		_ = call.Set("runId", runID)
		return handler(goja.Undefined(), v.rt.ToValue(input), call)
	})
	if err != nil {
		if errors.Is(err, ErrBusy) || errors.Is(err, ErrTimeout) {
			return nil, err
		}
		// This text reaches run history and the model, so it must not hold a secret.
		return nil, errors.New(p.redact(err.Error()))
	}
	out, err := plainJSON(res)
	if err != nil {
		return nil, fmt.Errorf("tool %s: %w", p.AddonName(name), err)
	}
	return out, nil
}

// Preset finds a running plugin's agent preset by "<plugin>.<name>". Its keys are the
// workflow agent's settings. It implements flow.PresetSource.
func (m *Manager) Preset(name string) (map[string]any, bool) {
	id, short, ok := strings.Cut(name, ".")
	if !ok {
		return nil, false
	}
	p := m.Loaded(id)
	if p == nil {
		return nil, false
	}
	for _, preset := range p.Manifest.AgentPresets {
		if preset.Name != short {
			continue
		}
		out := map[string]any{"system_prompt": preset.SystemPrompt}
		if len(preset.Tools) > 0 {
			out["tools"] = preset.Tools
		}
		if preset.MaxSteps > 0 {
			out["max_steps"] = preset.MaxSteps
		}
		if preset.OutputSchema != nil {
			out["output_schema"] = preset.OutputSchema
		}
		if preset.AI != nil {
			out["ai"] = preset.AI
		}
		return out, true
	}
	return nil, false
}

// PresetInfo describes an agent preset for pickers.
type PresetInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Presets lists the running plugins' agent presets.
func (m *Manager) Presets() []PresetInfo {
	out := []PresetInfo{}
	for _, p := range m.LoadedPlugins() {
		for _, preset := range p.Manifest.AgentPresets {
			out = append(out, PresetInfo{Name: p.AddonName(preset.Name), Description: preset.Description})
		}
	}
	return out
}

// SetEventSink says where host.workflows.emit delivers events. The workflow engine
// starts after plugins load, so it's set separately.
func (m *Manager) SetEventSink(sink func(name string, payload map[string]any)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sink = sink
}

func (m *Manager) eventSink() func(string, map[string]any) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sink
}
