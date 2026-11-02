package main

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/flow/tools"
	"github.com/compdani/list_pocket/models"
)

func TestEveryFlowToolDescribesItsOutput(t *testing.T) {
	outputs := flowToolOutputs()
	seen := map[string]bool{}
	for _, tool := range (&App{}).flowTools() {
		seen[tool.Name()] = true
		// It runs at the model provider, so a step never gets its output.
		if tool.Name() == tools.WebSearch {
			continue
		}
		if tools.OutputSchema(tool) == nil {
			t.Errorf("%s has no output schema", tool.Name())
		}
	}
	for name := range outputs {
		if !seen[name] {
			t.Errorf("output for %s, which isn't a tool", name)
		}
	}
}

func propertyNames(schema map[string]any) []string {
	props, _ := schema["properties"].(map[string]any)
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func jsonKeys(t *testing.T, value any) []string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestContactOutputMatchesContactSummary(t *testing.T) {
	got, want := propertyNames(contactOutput(nil)), jsonKeys(t, contactSummary(models.Subscriber{}))
	if len(got) != len(want) {
		t.Fatalf("schema %v, summary %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("schema %v, summary %v", got, want)
		}
	}
}

func TestTypedOutputsMatchTheirJSON(t *testing.T) {
	for name, tc := range map[string]struct {
		schema map[string]any
		value  any
	}{
		"BoardMove":          {tools.SchemaFor[core.BoardMove](), core.BoardMove{}},
		"ContactProfile":     {tools.SchemaFor[core.ContactProfile](), core.ContactProfile{}},
		"ContactNote":        {tools.SchemaFor[core.ContactNote](), core.ContactNote{}},
		"ContactTask":        {tools.SchemaFor[core.ContactTask](), core.ContactTask{}},
		"ConversationList":   {tools.SchemaFor[core.ConversationList](), core.ConversationList{}},
		"ConversationThread": {tools.SchemaFor[core.ConversationThread](), core.ConversationThread{}},
	} {
		props := map[string]bool{}
		for _, prop := range propertyNames(tc.schema) {
			props[prop] = true
		}
		// Fields with omitempty are missing from the JSON of an empty value, but every field
		// that is there has to be in the schema.
		for _, key := range jsonKeys(t, tc.value) {
			if !props[key] {
				t.Errorf("%s: %s is in the JSON but not the schema %v", name, key, propertyNames(tc.schema))
			}
		}
	}
}

func TestCatalogSizeStaysReasonable(t *testing.T) {
	raw, err := json.Marshal(flowToolOutputs())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("output schemas: %d bytes", len(raw))
	// The assistant reads the catalog, so this has to stay small next to a model's context.
	if len(raw) > 64<<10 {
		t.Errorf("output schemas are %d bytes", len(raw))
	}
}
