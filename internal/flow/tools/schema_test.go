package tools

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

type schemaBase struct {
	ID string `json:"id"`
}

type schemaChild struct {
	Parent *schemaChild `json:"parent"`
	Label  string       `json:"label"`
}

type schemaSample struct {
	schemaBase
	Name     string            `json:"name,omitempty"`
	Count    int               `json:"count"`
	Score    float64           `json:"score"`
	On       bool              `json:"on"`
	When     time.Time         `json:"when"`
	Raw      json.RawMessage   `json:"raw"`
	Blob     []byte            `json:"blob"`
	Attrs    map[string]string `json:"attrs"`
	Children []schemaChild     `json:"children"`
	Hidden   string            `json:"-"`
	Untagged string
	private  string
}

func TestSchemaForFollowsJSONEncoding(t *testing.T) {
	_ = schemaSample{}.private
	got := SchemaFor[schemaSample]()
	if got["type"] != "object" {
		t.Fatalf("type = %v", got["type"])
	}
	props := got["properties"].(map[string]any)
	want := map[string]any{
		"id":       map[string]any{"type": "string"},
		"name":     map[string]any{"type": "string"},
		"count":    map[string]any{"type": "integer"},
		"score":    map[string]any{"type": "number"},
		"on":       map[string]any{"type": "boolean"},
		"when":     map[string]any{},
		"raw":      map[string]any{},
		"blob":     map[string]any{"type": "string"},
		"attrs":    map[string]any{"type": "object"},
		"Untagged": map[string]any{"type": "string"},
		"children": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
			"parent": map[string]any{"type": "object"},
			"label":  map[string]any{"type": "string"},
		}}},
	}
	for name, schema := range want {
		if !reflect.DeepEqual(props[name], schema) {
			t.Errorf("%s = %#v, want %#v", name, props[name], schema)
		}
	}
	if _, ok := props["Hidden"]; ok {
		t.Error("json:\"-\" field was included")
	}
	if _, ok := props["private"]; ok {
		t.Error("unexported field was included")
	}
	if len(props) != len(want) {
		t.Errorf("got %d properties, want %d: %v", len(props), len(want), props)
	}
}

func TestSchemaForStopsAtRecursiveTypes(t *testing.T) {
	got := SchemaFor[schemaChild]()
	parent := got["properties"].(map[string]any)["parent"].(map[string]any)
	if parent["type"] != "object" || parent["properties"] != nil {
		t.Fatalf("parent = %#v", parent)
	}
}

func TestSchemaForSlices(t *testing.T) {
	got := SchemaFor[[]schemaBase]()
	if got["type"] != "array" {
		t.Fatalf("type = %v", got["type"])
	}
	items := got["items"].(map[string]any)
	if _, ok := items["properties"].(map[string]any)["id"]; !ok {
		t.Fatalf("items = %#v", items)
	}
}

type outputTool struct{ WebSearchInfo }

func (outputTool) OutputSchema() map[string]any { return map[string]any{"type": "object"} }

func TestOutputSchemaIsOptional(t *testing.T) {
	if got := OutputSchema(WebSearchInfo{}); got != nil {
		t.Fatalf("tool without a schema = %v", got)
	}
	if got := OutputSchema(outputTool{}); got == nil {
		t.Fatal("tool with a schema returned nil")
	}
}
