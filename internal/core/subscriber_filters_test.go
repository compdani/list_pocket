package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSubscriberFiltersEmpty(t *testing.T) {
	for _, raw := range []string{"", "null", "{}", `{"logic":"and","rules":[]}`} {
		f, err := ParseSubscriberFilters(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("raw %q: unexpected err %v", raw, err)
		}
		if f != nil {
			t.Fatalf("raw %q: expected nil filters", raw)
		}
	}
}

func TestParseSubscriberFiltersRejectsBadKey(t *testing.T) {
	raw := json.RawMessage(`{
		"logic":"and",
		"rules":[{"field":"attrib","op":"eq","key":"city';drop","value":"x"}]
	}`)
	if _, err := ParseSubscriberFilters(raw); err == nil {
		t.Fatal("expected error for unsafe attrib key")
	}
}

func TestParseSubscriberFiltersRejectsDeepNesting(t *testing.T) {
	raw := json.RawMessage(`{
		"logic":"and",
		"rules":[{
			"logic":"or",
			"rules":[{
				"logic":"and",
				"rules":[{"field":"status","op":"eq","value":"enabled"}]
			}]
		}]
	}`)
	if _, err := ParseSubscriberFilters(raw); err == nil {
		t.Fatal("expected error for deep nesting")
	}
}

func TestCompileTagHasAny(t *testing.T) {
	c := &Core{}
	sql, args, err := c.CompileSubscriberFilters(json.RawMessage(`{
		"logic":"and",
		"rules":[{"field":"tag","op":"has_any","value":["VIP","demo"]}]
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(sql, "EXISTS") || !strings.Contains(sql, "json_each") {
		t.Fatalf("unexpected sql: %s", sql)
	}
	if len(args) != 2 || args[0] != "vip" || args[1] != "demo" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestCompileTagHasAllAndHasNone(t *testing.T) {
	c := &Core{}
	sql, args, err := c.CompileSubscriberFilters(json.RawMessage(`{
		"logic":"and",
		"rules":[
			{"field":"tag","op":"has_all","value":["a","b"]},
			{"field":"tag","op":"has_none","value":["c"]}
		]
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(sql, " AND ") {
		t.Fatalf("expected AND join: %s", sql)
	}
	if !strings.Contains(sql, "NOT EXISTS") {
		t.Fatalf("expected NOT EXISTS for has_none: %s", sql)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %#v", args)
	}
}

func TestCompileAttribOps(t *testing.T) {
	c := &Core{}
	sql, args, err := c.CompileSubscriberFilters(json.RawMessage(`{
		"logic":"and",
		"rules":[
			{"field":"attrib","key":"city","op":"eq","value":"Berlin"},
			{"field":"attrib","key":"projects","op":"gt","value":3},
			{"field":"attrib","key":"stack.preferred_language","op":"exists"}
		]
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(sql, "json_extract") {
		t.Fatalf("expected json_extract: %s", sql)
	}
	if !strings.Contains(sql, "CAST") {
		t.Fatalf("expected CAST for comparisons: %s", sql)
	}
	foundPath := false
	for _, a := range args {
		if a == "$.stack.preferred_language" {
			foundPath = true
		}
	}
	if !foundPath {
		t.Fatalf("missing nested path in args: %#v", args)
	}
}

func TestCompileNestedOrGroup(t *testing.T) {
	c := &Core{}
	sql, args, err := c.CompileSubscriberFilters(json.RawMessage(`{
		"logic":"and",
		"rules":[
			{"field":"status","op":"eq","value":"enabled"},
			{
				"logic":"or",
				"rules":[
					{"field":"email","op":"contains","value":"@acme.com"},
					{"field":"name","op":"starts_with","value":"Ann"}
				]
			}
		]
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("expected nested OR: %s", sql)
	}
	if len(args) != 3 {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestCompileRejectsTagsAttribKey(t *testing.T) {
	c := &Core{}
	_, _, err := c.CompileSubscriberFilters(json.RawMessage(`{
		"logic":"and",
		"rules":[{"field":"attrib","key":"tags","op":"eq","value":"x"}]
	}`))
	if err == nil {
		t.Fatal("expected error when filtering attribs.tags via attrib field")
	}
}
