package flow

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/flow/schema"

	"github.com/pocketbase/pocketbase/tools/cron"
)

// Catalog is what validation checks references against.
type Catalog interface {
	HasTool(name string) bool
	Preset(name string) (map[string]any, bool)
	HasEvent(name string) bool
	HasWorkflow(name string) bool
	HasCollection(name string) bool
	ToolHasSideEffects(name string) bool
}

type Validation struct {
	OK          bool     `json:"ok"`
	Errors      []string `json:"errors"`
	Warnings    []string `json:"warnings"`
	ValidatedAt string   `json:"validated_at"`
}

const maxLoopIterations = 100

var idRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,60}$`)

// Validate checks a definition: structure, graph shape, references, and
// that every template and expression compiles.
func Validate(d *Definition, cat Catalog) Validation {
	v := Validation{Errors: []string{}, Warnings: []string{}, ValidatedAt: time.Now().UTC().Format(time.RFC3339)}
	errf := func(format string, a ...any) { v.Errors = append(v.Errors, fmt.Sprintf(format, a...)) }
	warnf := func(format string, a ...any) { v.Warnings = append(v.Warnings, fmt.Sprintf(format, a...)) }

	if !nameRe.MatchString(d.Name) {
		errf("name %q must be 2-61 lowercase letters, digits, or _", d.Name)
	}
	validateTrigger(d, cat, errf, warnf)

	if d.InputSchema != nil && d.InputSchema["type"] != "object" {
		errf("input_schema must be an object schema")
	}
	for _, e := range schema.CheckWidgets(d.InputSchema, nil) {
		warnf("input_schema %s", strings.TrimPrefix(e, "$."))
	}
	if d.InputSchema != nil && d.DefaultInput != nil {
		if errs := schema.Validate(d.InputSchema, any(d.DefaultInput)); len(errs) > 0 {
			errf("default_input: %s", schema.Summary(errs))
		}
	}

	// --- nodes ---
	ids := map[string]*Node{}
	starts := 0
	for i := range d.Nodes {
		n := &d.Nodes[i]
		label := fmt.Sprintf("node %q", n.ID)
		if !idRe.MatchString(n.ID) {
			errf("nodes[%d]: invalid id %q", i, n.ID)
			continue
		}
		if ids[n.ID] != nil {
			errf("%s: duplicate id", label)
		}
		ids[n.ID] = n
		if !nodeTypes[n.Type] {
			errf("%s: unknown type %q", label, n.Type)
			continue
		}
		if n.Type == NodeStart {
			starts++
		}
		if n.OnError != "" && n.OnError != "fail" && n.OnError != "continue" {
			errf("%s: on_error must be fail or continue", label)
		}
		checkTemplates := func(field string, val any) {
			for _, e := range CheckTemplates(val) {
				errf("%s: %s: %s", label, field, e)
			}
		}
		checkTemplates("input", toAny(n.Input))

		switch n.Type {
		case NodeAgent:
			validateAgent(n, label, cat, errf, warnf)
		case NodeTool:
			if n.Tool == "" {
				errf("%s: tool is required", label)
			} else if !cat.HasTool(n.Tool) {
				errf("%s: unknown tool %q", label, n.Tool)
			}
		case NodeBranch:
			if len(n.Cases) == 0 {
				errf("%s: branch needs at least one case", label)
			}
			for j, c := range n.Cases {
				if err := CheckExpr(c.If); err != nil {
					errf("%s: cases[%d].if: %v", label, j, err)
				}
			}
			if n.Next != "" {
				errf("%s: branch routes with cases/default, not next", label)
			}
		case NodeForEach:
			if n.Items == "" || CheckExpr(n.Items) != nil {
				errf("%s: items must be a valid expression", label)
			}
			if n.Body == "" {
				errf("%s: body (first node of the per-item chain) is required", label)
			}
			if n.Concurrency < 0 || n.Concurrency > 20 {
				errf("%s: concurrency must be 0-20", label)
			}
		case NodeLoop:
			if n.Until == "" || CheckExpr(n.Until) != nil {
				errf("%s: until must be a valid expression", label)
			}
			if n.Body == "" {
				errf("%s: body (first node of the per-iteration chain) is required", label)
			}
			if n.MaxIterations < 1 || n.MaxIterations > maxLoopIterations {
				errf("%s: max_iterations must be 1-%d", label, maxLoopIterations)
			}
		case NodeReview:
			if n.Review == nil {
				errf("%s: review settings are required", label)
				break
			}
			checkTemplates("review.title", n.Review.Title)
			checkTemplates("review.instructions", n.Review.Instructions)
			checkTemplates("review.data", n.Review.Data)
			if n.Review.AutoApprove != "" && CheckExpr(n.Review.AutoApprove) != nil {
				errf("%s: review.auto_approve is not a valid expression", label)
			}
			if n.Review.OnDeny != "" && n.Review.OnDeny != "skip" && n.Review.OnDeny != "continue" {
				errf("%s: review.on_deny must be skip or continue", label)
			}
			if _, err := parseDuration(n.Review.Timeout); err != nil {
				errf("%s: review.timeout: %v", label, err)
			}
		case NodeWait:
			w := n.Wait
			if w == nil || (w.Delay == "" && w.Event == "") {
				errf("%s: wait needs delay or event", label)
				break
			}
			if _, err := parseDuration(w.Delay); err != nil {
				errf("%s: wait.delay: %v", label, err)
			}
			if _, err := parseDuration(w.Timeout); err != nil {
				errf("%s: wait.timeout: %v", label, err)
			}
			if w.Event != "" && !cat.HasEvent(w.Event) {
				errf("%s: unknown event %q", label, w.Event)
			}
			if w.Match != "" && CheckExpr(w.Match) != nil {
				errf("%s: wait.match is not a valid expression", label)
			}
		case NodeSubworkflow:
			if n.Workflow == "" {
				errf("%s: workflow is required", label)
			} else if n.Workflow == d.Name {
				errf("%s: a workflow can't call itself", label)
			} else if !cat.HasWorkflow(n.Workflow) {
				errf("%s: unknown workflow %q", label, n.Workflow)
			}
		case NodeNotify:
			checkTemplates("message", n.Message)
		case NodeTransform:
			checkTemplates("value", n.Value)
		case NodeEnd:
			checkTemplates("output", n.Output)
			if n.Next != "" {
				errf("%s: end can't have next", label)
			}
		}
	}
	if starts != 1 {
		errf("workflow needs exactly one start node (found %d)", starts)
	}

	// --- graph ---
	refErrors := 0
	exists := func(from, field, to string) {
		if to != "" && ids[to] == nil {
			errf("node %q: %s points to unknown node %q", from, field, to)
			refErrors++
		}
	}
	for _, n := range d.Nodes {
		exists(n.ID, "next", n.Next)
		exists(n.ID, "body", n.Body)
		if n.Default != "" {
			exists(n.ID, "default", n.Default)
		}
		for _, c := range n.Cases {
			exists(n.ID, "case", c.To) // "" ends the path
		}
	}
	// Graph shape needs sound references and a single start; other errors
	// don't stop it, so cycles and unreachable nodes are reported together.
	if refErrors == 0 && starts == 1 && len(ids) == len(d.Nodes) {
		validateGraph(d, ids, errf, warnf)
	}

	v.OK = len(v.Errors) == 0
	sort.Strings(v.Warnings)
	return v
}

func toAny(m map[string]any) any {
	if m == nil {
		return nil
	}
	return m
}

func validateTrigger(d *Definition, cat Catalog, errf, warnf func(string, ...any)) {
	t := d.Trigger
	switch t.Type {
	case "manual":
	case "cron":
		if _, err := cron.NewSchedule(t.Spec); err != nil {
			errf("trigger.spec: %v", err)
		}
		if t.Timezone != "" {
			if _, err := time.LoadLocation(t.Timezone); err != nil {
				errf("trigger.timezone: %v", err)
			}
		}
		if t.Overlap != "" && t.Overlap != "skip" && t.Overlap != "queue" {
			errf("trigger.overlap must be skip or queue")
		}
		for _, f := range []struct{ name, val string }{{"start_at", t.StartAt}, {"end_at", t.EndAt}} {
			if f.val != "" {
				if _, err := time.Parse(time.RFC3339, f.val); err != nil {
					errf("trigger.%s must be RFC 3339: %v", f.name, err)
				}
			}
		}
	case "event":
		switch {
		case t.Event != "" && t.Collection != "":
			errf("trigger: set event or collection, not both")
		case t.Event != "":
			if !cat.HasEvent(t.Event) {
				errf("trigger.event: unknown event %q", t.Event)
			}
		case t.Collection != "":
			if !cat.HasCollection(t.Collection) {
				errf("trigger.collection: %q can't start a workflow", t.Collection)
			}
			for _, on := range t.On {
				if on != "create" && on != "update" {
					errf("trigger.on: %q must be create or update", on)
				}
			}
		default:
			errf("trigger: event triggers need event or collection")
		}
		if t.Filter != "" && CheckExpr(t.Filter) != nil {
			errf("trigger.filter is not a valid expression")
		}
		for _, e := range CheckTemplates(toAny(t.InputMap)) {
			errf("trigger.input_map: %s", e)
		}
	case "webhook":
		if t.Filter != "" && CheckExpr(t.Filter) != nil {
			errf("trigger.filter is not a valid expression")
		}
		for _, e := range CheckTemplates(toAny(t.InputMap)) {
			errf("trigger.input_map: %s", e)
		}
	default:
		errf("trigger.type must be manual, cron, event, or webhook")
	}
}

func validateAgent(n *Node, label string, cat Catalog, errf, warnf func(string, ...any)) {
	a := n.Agent
	if a == nil {
		errf("%s: agent settings are required", label)
		return
	}
	agentTools := a.Tools
	if a.Preset != "" {
		preset, ok := cat.Preset(a.Preset)
		if !ok {
			errf("%s: unknown agent preset %q", label, a.Preset)
		} else if len(agentTools) == 0 {
			// The node's own tools replace the preset's, so these only apply without them.
			agentTools = presetTools(preset)
		}
	} else if a.SystemPrompt == "" {
		errf("%s: system_prompt or preset is required", label)
	}
	for _, t := range agentTools {
		if !cat.HasTool(t) {
			errf("%s: unknown tool %q", label, t)
		} else if cat.ToolHasSideEffects(t) {
			warnf("%s: the agent can use %s without a person checking first. Inbound messages can try to steer an agent, so prefer having it draft and a review step approve", label, t)
		}
	}
	if a.OutputSchema != nil && a.OutputSchema["type"] == nil {
		errf("%s: output_schema needs a type", label)
	}
}

func presetTools(preset map[string]any) []string {
	switch names := preset["tools"].(type) {
	case []string:
		return names
	case []any:
		out := make([]string, 0, len(names))
		for _, name := range names {
			if s, ok := name.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// validateGraph checks reachability, cycles, and ForEach/Loop bodies.
func validateGraph(d *Definition, ids map[string]*Node, errf, warnf func(string, ...any)) {
	succ := func(n *Node) []string {
		var out []string
		if n.Next != "" {
			out = append(out, n.Next)
		}
		for _, c := range n.Cases {
			if c.To != "" {
				out = append(out, c.To)
			}
		}
		if n.Default != "" {
			out = append(out, n.Default)
		}
		return out
	}

	// Main path: from start, not entering foreach/loop bodies.
	main := map[string]bool{}
	var walk func(id string, seen map[string]bool, path map[string]bool) bool
	walk = func(id string, seen, path map[string]bool) bool {
		if path[id] {
			errf("cycle through node %q (repeat with a foreach or loop node instead)", id)
			return false
		}
		if seen[id] {
			return true
		}
		seen[id] = true
		path[id] = true
		defer delete(path, id)
		for _, s := range succ(ids[id]) {
			if !walk(s, seen, path) {
				return false
			}
		}
		return true
	}
	start := d.StartNode()
	walk(start.ID, main, map[string]bool{})

	// Bodies.
	inBody := map[string]string{}
	for _, n := range d.Nodes {
		if !hasBody(n.Type) || n.Body == "" {
			continue
		}
		if !main[n.ID] {
			errf("%s %q must be on the main path (nested foreach/loop is not supported)", n.Type, n.ID)
			continue
		}
		body := map[string]bool{}
		walk(n.Body, body, map[string]bool{})
		for id := range body {
			switch {
			case main[id]:
				errf("%s %q: body node %q is also on the main path", n.Type, n.ID, id)
			case inBody[id] != "" && inBody[id] != n.ID:
				errf("node %q is in the bodies of both %q and %q", id, inBody[id], n.ID)
			case hasBody(ids[id].Type):
				errf("%s %q: nested %s %q is not supported", n.Type, n.ID, ids[id].Type, id)
			case ids[id].Type == NodeStart:
				errf("%s %q: body can't contain the start node", n.Type, n.ID)
			}
			inBody[id] = n.ID
		}
	}
	for _, n := range d.Nodes {
		if !main[n.ID] && inBody[n.ID] == "" {
			warnf("node %q is unreachable", n.ID)
		}
	}
}

func hasBody(typ string) bool { return typ == NodeForEach || typ == NodeLoop }
