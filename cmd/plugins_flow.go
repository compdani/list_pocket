package main

import (
	"context"

	"github.com/compdani/list_pocket/internal/flow"
	"github.com/compdani/list_pocket/internal/flow/tools"
	"github.com/compdani/list_pocket/internal/plugins"
)

// pluginTool adapts a plugin's tool to the workflow tool interface.
type pluginTool struct {
	plugin *plugins.Plugin
	spec   plugins.ToolManifest
}

func (t pluginTool) Name() string                { return t.plugin.AddonName(t.spec.Name) }
func (t pluginTool) Description() string         { return t.spec.Description }
func (t pluginTool) InputSchema() map[string]any { return t.spec.InputSchema }
func (t pluginTool) RequiresApproval() bool      { return t.spec.NeedsApproval() }
func (t pluginTool) ReadOnly() bool              { return t.spec.ReadOnly }
func (t pluginTool) Invoke(ctx context.Context, call tools.Call) (any, error) {
	return t.plugin.CallTool(ctx, t.spec.Name, call.Args, call.Config, call.RunID)
}

// registerPluginAddons adds the running plugins' tools and events to the workflow
// catalog. Enabling or disabling a plugin restarts the app, so this only runs at start.
// A plugin named after a built-in namespace can't replace what's already there.
func (a *App) registerPluginAddons(registry *tools.Registry) {
	for _, p := range a.plugins.LoadedPlugins() {
		for _, spec := range p.Manifest.Tools {
			tool := pluginTool{plugin: p, spec: spec}
			if _, taken := registry.Get(tool.Name()); taken || tool.Name() == tools.WebSearch {
				a.log.Printf("plugin %s: tool %s is already in use, so it wasn't added", p.Manifest.ID, tool.Name())
				continue
			}
			registry.Register(tool)
		}
		for _, event := range p.Manifest.Events {
			if name := p.AddonName(event.Name); !flow.RegisterEvent(name, event.Description) {
				a.log.Printf("plugin %s: event %s is already in use, so it wasn't added", p.Manifest.ID, name)
			}
		}
	}
	a.plugins.SetEventSink(a.emitFlowEvent)
}
