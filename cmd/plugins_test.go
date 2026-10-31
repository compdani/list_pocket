package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/compdani/list_pocket/internal/plugins"
)

func TestMergePluginPermissions(t *testing.T) {
	cfg := &Config{
		PermissionsRaw: json.RawMessage(`[{"group":"lists","permissions":["lists:get"]}]`),
		Permissions:    map[string]struct{}{"lists:get": {}},
	}
	err := mergePluginPermissions(cfg, []plugins.PermissionGroup{
		{Group: "plugin_sample", Permissions: []string{"plugin_sample:manage"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Permissions["plugin_sample:manage"]; !ok {
		t.Fatal("plugin permission missing from the lookup map")
	}
	if _, ok := cfg.Permissions["lists:get"]; !ok {
		t.Fatal("existing permission lost")
	}
	raw := string(cfg.PermissionsRaw)
	if !strings.Contains(raw, `"group":"lists"`) || !strings.Contains(raw, `"group":"plugin_sample"`) {
		t.Fatalf("catalog not merged: %s", raw)
	}
}
