// Package schema validates JSON values against the subset of JSON Schema
// the harness uses for tool inputs and agent outputs: type, properties,
// required, items, enum, additionalProperties=false, minItems/maxItems.
package schema

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Validate returns human-readable problems; empty means valid. v must be the
// result of json.Unmarshal into `any`.
func Validate(schema map[string]any, v any) []string {
	var errs []string
	validate(schema, v, "$", &errs)
	return errs
}

func validate(s map[string]any, v any, path string, errs *[]string) {
	if s == nil {
		return
	}
	if t, ok := s["type"]; ok && !matchesType(t, v) {
		*errs = append(*errs, fmt.Sprintf("%s: expected %v, got %s", path, t, typeName(v)))
		return
	}
	if enum, ok := s["enum"].([]any); ok && !inEnum(enum, v) {
		*errs = append(*errs, fmt.Sprintf("%s: must be one of %v", path, enum))
	}

	switch val := v.(type) {
	case map[string]any:
		props, _ := s["properties"].(map[string]any)
		for _, r := range toStrings(s["required"]) {
			if _, ok := val[r]; !ok {
				*errs = append(*errs, fmt.Sprintf("%s: missing required field %q", path, r))
			}
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if ps, ok := props[k].(map[string]any); ok {
				validate(ps, val[k], path+"."+k, errs)
			} else if ap, ok := s["additionalProperties"].(bool); ok && !ap {
				*errs = append(*errs, fmt.Sprintf("%s: unexpected field %q", path, k))
			}
		}
	case []any:
		if min, ok := num(s["minItems"]); ok && float64(len(val)) < min {
			*errs = append(*errs, fmt.Sprintf("%s: needs at least %v items", path, min))
		}
		if max, ok := num(s["maxItems"]); ok && float64(len(val)) > max {
			*errs = append(*errs, fmt.Sprintf("%s: allows at most %v items", path, max))
		}
		if items, ok := s["items"].(map[string]any); ok {
			for i, item := range val {
				validate(items, item, fmt.Sprintf("%s[%d]", path, i), errs)
			}
		}
	}
}

func matchesType(t any, v any) bool {
	switch tt := t.(type) {
	case string:
		return isType(tt, v)
	case []any:
		for _, x := range tt {
			if s, ok := x.(string); ok && isType(s, v) {
				return true
			}
		}
		return false
	}
	return true
}

func isType(t string, v any) bool {
	switch t {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "null":
		return v == nil
	}
	return true
}

func typeName(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	}
	return fmt.Sprintf("%T", v)
}

func inEnum(enum []any, v any) bool {
	for _, e := range enum {
		if fmt.Sprint(e) == fmt.Sprint(v) && typeName(e) == typeName(v) {
			return true
		}
	}
	return false
}

func toStrings(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func num(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

// Summary joins problems for returning to a model.
func Summary(errs []string) string {
	if len(errs) > 8 {
		errs = append(errs[:8], fmt.Sprintf("…and %d more", len(errs)-8))
	}
	return strings.Join(errs, "; ")
}
