package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(v)
	}
}

func toStrings(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, toString(item))
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return []string{toString(v)}
	}
}

func toBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "1", "t", "true", "yes", "on":
			return true
		}
		return false
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	default:
		return false
	}
}

func toInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func toDuration(v any) time.Duration {
	switch t := v.(type) {
	case time.Duration:
		return t
	case string:
		d, err := time.ParseDuration(t)
		if err != nil {
			return 0
		}
		return d
	case int:
		return time.Duration(t)
	case int64:
		return time.Duration(t)
	case float64:
		return time.Duration(t)
	default:
		return 0
	}
}

func normalizeMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = normalizeValue(v)
	}
	return out
}

func normalizeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return normalizeMap(t)
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[fmt.Sprint(k)] = normalizeValue(val)
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return v
	}
}

func unflatten(m map[string]any, delim string) map[string]any {
	out := map[string]any{}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Shorter keys first so parent maps exist before nested writes.
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if len(keys[j]) < len(keys[i]) {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for _, k := range keys {
		setPath(out, strings.Split(k, delim), m[k])
	}
	return out
}

func setPath(m map[string]any, parts []string, val any) {
	if len(parts) == 1 {
		m[parts[0]] = val
		return
	}
	next, ok := m[parts[0]].(map[string]any)
	if !ok {
		next = map[string]any{}
		m[parts[0]] = next
	}
	setPath(next, parts[1:], val)
}

func flatten(m map[string]any, prefix, delim string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + delim + k
		}
		if nested, ok := v.(map[string]any); ok && len(nested) > 0 {
			for fk, fv := range flatten(nested, key, delim) {
				out[fk] = fv
			}
			continue
		}
		out[key] = v
	}
	return out
}
