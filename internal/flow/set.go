package flow

import (
	"context"
	"fmt"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/compdani/list_pocket/internal/flow/tmpl"
)

// Variable types for set nodes.
const (
	VarString   = "string"
	VarNumber   = "number"
	VarBoolean  = "boolean"
	VarTemplate = "template"
	VarCode     = "code"
	VarHTML     = "html"
)

var varNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,60}$`)

// validateVariables checks a set node's variables without running them.
func validateVariables(vars []Variable, label string, errf func(string, ...any)) {
	if len(vars) == 0 {
		errf("%s: set needs at least one variable", label)
	}
	seen := map[string]bool{}
	for i, v := range vars {
		where := fmt.Sprintf("%s: variables[%d]", label, i)
		if !varNameRe.MatchString(v.Name) {
			errf("%s: name %q must be letters, digits, or _, and can't start with a digit", where, v.Name)
		} else {
			if seen[v.Name] {
				errf("%s: duplicate name %q", where, v.Name)
			}
			seen[v.Name] = true
			where = fmt.Sprintf("%s: variable %q", label, v.Name)
		}
		text, isText := v.Value.(string)
		if v.Value != nil && !isText && v.Type != VarNumber && v.Type != VarBoolean {
			errf("%s: value must be text", where)
			continue
		}
		switch v.Type {
		case VarString:
		case VarTemplate, VarHTML:
			for _, e := range CheckTemplates(text) {
				errf("%s: %s", where, e)
			}
		case VarCode:
			if strings.TrimSpace(text) == "" {
				errf("%s: code is required", where)
			} else if _, err := compileCode(text); err != nil {
				errf("%s: code: %v", where, err)
			}
		case VarNumber, VarBoolean:
			if isText && strings.Contains(text, "{{") {
				for _, e := range CheckTemplates(text) {
					errf("%s: %s", where, e)
				}
			} else if _, err := coerceVariable(v.Type, v.Value); err != nil {
				errf("%s: %v", where, err)
			}
		default:
			errf("%s: type must be string, number, boolean, template, code, or html", where)
		}
	}
}

// runSet works out a set node's variables in order, so each can read the ones before it.
func runSet(ctx context.Context, def *Definition, n *Node, env map[string]any, logf func(string)) (map[string]any, error) {
	vars, _ := env["vars"].(map[string]any)
	if vars == nil {
		vars = map[string]any{}
		env["vars"] = vars
	}
	out := make(map[string]any, len(n.Variables))
	for _, v := range n.Variables {
		val, err := variableValue(ctx, def, n, v, env, out, logf)
		if err != nil {
			return nil, fmt.Errorf("variable %s: %w", v.Name, err)
		}
		out[v.Name] = val
		vars[v.Name] = val
	}
	return out, nil
}

func variableValue(ctx context.Context, def *Definition, n *Node, v Variable, env, current map[string]any, logf func(string)) (any, error) {
	text, _ := v.Value.(string)
	switch v.Type {
	case VarString:
		return text, nil
	case VarTemplate:
		return Render(text, env)
	case VarCode:
		return runCode(ctx, text, env, logf)
	case VarHTML:
		trusted := trustedHTML(def, n, env, current)
		return tmpl.RenderString(text, env, func(s string) string {
			if trusted[s] {
				return s
			}
			return html.EscapeString(s)
		})
	}
	rendered, err := Render(v.Value, env)
	if err != nil {
		return nil, err
	}
	return coerceVariable(v.Type, rendered)
}

// trustedHTML is every value an HTML variable has produced so far on this path. Those are
// markup someone wrote, so one HTML variable can include another. Anything else that goes
// into HTML, such as a contact's name, is escaped. Comparing values is safe because a value
// that matches is the same text, wherever it came from.
func trustedHTML(def *Definition, n *Node, env, current map[string]any) map[string]bool {
	trusted := map[string]bool{}
	nodes, _ := env["nodes"].(map[string]any)
	for i := range def.Nodes {
		other := &def.Nodes[i]
		if other.Type != NodeSet {
			continue
		}
		out, _ := nodes[other.ID].(map[string]any)
		if other.ID == n.ID {
			out = current
		}
		for _, v := range other.Variables {
			if s, ok := out[v.Name].(string); ok && v.Type == VarHTML {
				trusted[s] = true
			}
		}
	}
	return trusted
}

func coerceVariable(typ string, val any) (any, error) {
	text, isText := val.(string)
	text = strings.TrimSpace(text)
	if typ == VarNumber {
		if f, ok := val.(float64); ok {
			return f, nil
		}
		if isText {
			f, err := strconv.ParseFloat(text, 64)
			if err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
				return f, nil
			}
		}
		return nil, fmt.Errorf("%v isn't a number", describeValue(val))
	}
	if b, ok := val.(bool); ok {
		return b, nil
	}
	if val == nil {
		return false, nil
	}
	if strings.EqualFold(text, "true") {
		return true, nil
	}
	if isText && (text == "" || strings.EqualFold(text, "false")) {
		return false, nil
	}
	return nil, fmt.Errorf("%v isn't true or false", describeValue(val))
}

func describeValue(val any) string {
	if val == nil {
		return "an empty value"
	}
	return strconv.Quote(stringify(val))
}
