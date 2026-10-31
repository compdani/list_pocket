// Package tmpl is the expression and template language shared by
// workflows, the template library, and plugin scripts.
package tmpl

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
)

// Expressions use expr-lang (safe: no I/O, no code execution). Workflows
// pass {input, nodes: {<id>: output}, item, index, event, record, run, now};
// the template library passes the caller's variables.
//
// Templates: a string that is exactly "{{ expr }}" evaluates to the
// expression's value (keeping its type); a string with embedded {{ }}
// interpolates to a string; maps and arrays are rendered recursively.

var placeholder = regexp.MustCompile(`\{\{\s*(.+?)\s*\}\}`)

var (
	cacheMu sync.RWMutex
	cache   = map[string]*vm.Program{}
)

func compile(code string) (*vm.Program, error) {
	cacheMu.RLock()
	p, ok := cache[code]
	cacheMu.RUnlock()
	if ok {
		return p, nil
	}
	p, err := expr.Compile(code, expr.AllowUndefinedVariables())
	if err != nil {
		return nil, err
	}
	cacheMu.Lock()
	cache[code] = p
	cacheMu.Unlock()
	return p, nil
}

// Eval evaluates an expression (with or without surrounding {{ }}).
func Eval(code string, env map[string]any) (any, error) {
	code = stripBraces(code)
	p, err := compile(code)
	if err != nil {
		return nil, fmt.Errorf("expression %q: %w", code, err)
	}
	out, err := expr.Run(p, env)
	if err != nil {
		return nil, fmt.Errorf("expression %q: %w", code, err)
	}
	return normalize(out), nil
}

// EvalBool evaluates a condition; nil/false/0/"" are false.
func EvalBool(code string, env map[string]any) (bool, error) {
	v, err := Eval(code, env)
	if err != nil {
		return false, err
	}
	return truthy(v), nil
}

func stripBraces(code string) string {
	code = strings.TrimSpace(code)
	if m := placeholder.FindStringSubmatch(code); m != nil && m[0] == code {
		return m[1]
	}
	return code
}

// Render evaluates templates inside v.
func Render(v any, env map[string]any) (any, error) {
	switch t := v.(type) {
	case string:
		return renderString(t, env)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			r, err := Render(val, env)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			r, err := Render(val, env)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out[i] = r
		}
		return out, nil
	}
	return v, nil
}

// RenderMap renders a map template (nil stays nil).
func RenderMap(m map[string]any, env map[string]any) (map[string]any, error) {
	if m == nil {
		return nil, nil
	}
	out, err := Render(m, env)
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

func renderString(s string, env map[string]any) (any, error) {
	return RenderString(s, env, nil)
}

// RenderString renders one template string. escape, if set, is applied to
// interpolated values (not to the literal text), e.g. html.EscapeString.
func RenderString(s string, env map[string]any, escape func(string) string) (any, error) {
	matches := placeholder.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s, nil
	}
	if escape == nil && len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(s) {
		return Eval(s[matches[0][2]:matches[0][3]], env)
	}
	var sb strings.Builder
	last := 0
	for _, m := range matches {
		sb.WriteString(s[last:m[0]])
		v, err := Eval(s[m[2]:m[3]], env)
		if err != nil {
			return nil, err
		}
		text := Stringify(v)
		if escape != nil {
			text = escape(text)
		}
		sb.WriteString(text)
		last = m[1]
	}
	sb.WriteString(s[last:])
	return sb.String(), nil
}

// Stringify formats a value for interpolation into text.
func Stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case map[string]any, []any:
		raw, _ := json.Marshal(t)
		return string(raw)
	}
	return fmt.Sprint(v)
}

// CheckTemplates compiles every expression in v without running it.
func CheckTemplates(v any) []string {
	var errs []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			for _, m := range placeholder.FindAllStringSubmatch(t, -1) {
				if _, err := compile(m[1]); err != nil {
					errs = append(errs, fmt.Sprintf("%q: %v", m[1], err))
				}
			}
		case map[string]any:
			for _, val := range t {
				walk(val)
			}
		case []any:
			for _, val := range t {
				walk(val)
			}
		}
	}
	walk(v)
	return errs
}

// CheckExpr compiles a bare expression.
func CheckExpr(code string) error {
	_, err := compile(stripBraces(code))
	return err
}

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case float64:
		return t != 0
	case int:
		return t != 0
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	}
	return true
}

// normalize converts expr results to JSON-shaped values (float64 numbers,
// []any, map[string]any) so outputs store and compare consistently.
func normalize(v any) any {
	switch v.(type) {
	case nil, string, bool, float64, map[string]any, []any:
		return v
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	var out any
	if json.Unmarshal(raw, &out) != nil {
		return fmt.Sprint(v)
	}
	return out
}

// Identifiers lists the root variable names the placeholders in v read
// (e.g. "lead" for {{ lead.name }}), sorted; expressions that don't parse
// are skipped.
func Identifiers(v any) []string {
	seen := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			for _, m := range placeholder.FindAllStringSubmatch(t, -1) {
				tree, err := parser.Parse(m[1])
				if err != nil {
					continue
				}
				ast.Find(tree.Node, func(n ast.Node) bool {
					if id, ok := n.(*ast.IdentifierNode); ok && id.Value != "" && id.Value != "#" {
						seen[id.Value] = true
					}
					return false
				})
			}
		case map[string]any:
			for _, val := range t {
				walk(val)
			}
		case []any:
			for _, val := range t {
				walk(val)
			}
		}
	}
	walk(v)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
