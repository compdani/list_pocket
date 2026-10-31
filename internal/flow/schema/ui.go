package schema

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Field editors. A schema property picks its editor with "x-ui": either a
// widget name or {"widget": name, ...options}. Names without a dot are
// built-in widgets; "<plugin>.<field>" is an editor a plugin ships (see
// plugins.md). The UI falls back to a plain input for anything else.
var BuiltinWidgets = map[string]string{
	"textarea": "string", // multi-line text
	"code":     "string", // options: language = json | javascript | html
	"rows":     "array",  // spreadsheet-like rows (CSV paste or upload)
	"email":    "object", // inline email: {subject, preheader, html, text}
	"template": "string", // name of a template from the library; options: kind
}

var pluginFieldRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}\.[a-z][a-z0-9_]{0,63}$`)

// Widget returns a property's x-ui widget name and options ("" if none).
func Widget(prop map[string]any) (string, map[string]any) {
	switch x := prop["x-ui"].(type) {
	case string:
		return x, nil
	case map[string]any:
		name, _ := x["widget"].(string)
		return name, x
	}
	return "", nil
}

// IsPluginField reports whether a widget name refers to a plugin editor.
func IsPluginField(widget string) bool {
	return pluginFieldRe.MatchString(widget)
}

// CheckWidgets reports problems with x-ui in an object schema's properties
// (recursively). knownField, if set, says whether a plugin editor exists;
// unknown ones are reported too.
func CheckWidgets(s map[string]any, knownField func(string) bool) []string {
	var errs []string
	var walk func(s map[string]any, path string)
	walk = func(s map[string]any, path string) {
		props, _ := s["properties"].(map[string]any)
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p, ok := props[k].(map[string]any)
			if !ok {
				continue
			}
			field := path + "." + k
			if _, has := p["x-ui"]; has {
				w, _ := Widget(p)
				want, builtin := BuiltinWidgets[w]
				switch {
				case w == "":
					errs = append(errs, fmt.Sprintf("%s: x-ui must be a widget name or {\"widget\": name}", field))
				case builtin:
					if t, _ := p["type"].(string); t != "" && t != want {
						errs = append(errs, fmt.Sprintf("%s: x-ui %q edits a %s, not a %s", field, w, want, t))
					}
				case IsPluginField(w):
					if knownField != nil && !knownField(w) {
						errs = append(errs, fmt.Sprintf("%s: field editor %q is not provided by any active plugin", field, w))
					}
				default:
					errs = append(errs, fmt.Sprintf("%s: unknown x-ui widget %q (built-in: %s, or <plugin>.<field>)", field, w, strings.Join(builtinNames(), ", ")))
				}
			}
			walk(p, field)
			if items, ok := p["items"].(map[string]any); ok {
				walk(items, field+"[]")
			}
		}
	}
	walk(s, "$")
	return errs
}

func builtinNames() []string {
	names := make([]string, 0, len(BuiltinWidgets))
	for n := range BuiltinWidgets {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
