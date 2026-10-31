package flow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ApplyPatch applies workflow patch ops to d and returns the new definition.
// Ops work on the JSON form so unknown or future fields survive.
//
//	{"op":"add_node","node":{…}}
//	{"op":"update_node","id":"x","set":{…}}        // shallow merge; agent/review/wait merge one level
//	{"op":"remove_node","id":"x"}                  // also clears references
//	{"op":"rename_node","id":"x","new_id":"y"}     // updates references and nodes.x in templates
//	{"op":"connect","from":"a","to":"b","handle":"next|default|body|case:N"}
//	{"op":"disconnect","from":"a","handle":"next|default|body|case:N"}
//	{"op":"set_trigger","trigger":{…}}
//	{"op":"set","field":"input_schema|default_input|ai|tool_config|description","value":…}
func ApplyPatch(d *Definition, ops []map[string]any) (*Definition, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	for i, op := range ops {
		if err := applyOp(doc, op); err != nil {
			return nil, fmt.Errorf("op %d (%v): %w", i, op["op"], err)
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var next Definition
	if err := json.Unmarshal(out, &next); err != nil {
		return nil, fmt.Errorf("patched workflow is malformed: %w", err)
	}
	return &next, nil
}

func nodesOf(doc map[string]any) []any {
	n, _ := doc["nodes"].([]any)
	return n
}

func findNode(doc map[string]any, id string) (map[string]any, int) {
	for i, raw := range nodesOf(doc) {
		if n, ok := raw.(map[string]any); ok && n["id"] == id {
			return n, i
		}
	}
	return nil, -1
}

func str(op map[string]any, key string) string {
	s, _ := op[key].(string)
	return strings.TrimSpace(s)
}

var nodeIDRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)

func applyOp(doc map[string]any, op map[string]any) error {
	switch str(op, "op") {
	case "add_node":
		node, ok := op["node"].(map[string]any)
		if !ok {
			return fmt.Errorf("node object required")
		}
		id, _ := node["id"].(string)
		if !nodeIDRe.MatchString(id) {
			return fmt.Errorf("invalid node id %q", id)
		}
		if n, _ := findNode(doc, id); n != nil {
			return fmt.Errorf("node %q already exists", id)
		}
		delete(node, "ui") // the canvas lays out new nodes
		doc["nodes"] = append(nodesOf(doc), node)

	case "update_node":
		n, _ := findNode(doc, str(op, "id"))
		if n == nil {
			return fmt.Errorf("unknown node %q", str(op, "id"))
		}
		set, ok := op["set"].(map[string]any)
		if !ok {
			return fmt.Errorf("set object required")
		}
		for k, v := range set {
			if k == "id" || k == "ui" {
				continue
			}
			if v == nil {
				delete(n, k)
				continue
			}
			sub, isMap := v.(map[string]any)
			cur, curMap := n[k].(map[string]any)
			if isMap && curMap && (k == "agent" || k == "review" || k == "wait") {
				for sk, sv := range sub {
					if sv == nil {
						delete(cur, sk)
					} else {
						cur[sk] = sv
					}
				}
				continue
			}
			n[k] = v
		}

	case "remove_node":
		id := str(op, "id")
		_, idx := findNode(doc, id)
		if idx < 0 {
			return fmt.Errorf("unknown node %q", id)
		}
		nodes := nodesOf(doc)
		if n := nodes[idx].(map[string]any); n["type"] == "start" {
			return fmt.Errorf("the start node can't be removed")
		}
		doc["nodes"] = append(nodes[:idx:idx], nodes[idx+1:]...)
		clearRefs(doc, id)

	case "rename_node":
		oldID, newID := str(op, "id"), str(op, "new_id")
		if !nodeIDRe.MatchString(newID) {
			return fmt.Errorf("invalid new_id %q", newID)
		}
		if n, _ := findNode(doc, oldID); n == nil {
			return fmt.Errorf("unknown node %q", oldID)
		}
		if n, _ := findNode(doc, newID); n != nil {
			return fmt.Errorf("node %q already exists", newID)
		}
		raw, _ := json.Marshal(doc)
		re := regexp.MustCompile(`nodes\.` + regexp.QuoteMeta(oldID) + `\b`)
		var renamed map[string]any
		if err := json.Unmarshal([]byte(re.ReplaceAllString(string(raw), "nodes."+newID)), &renamed); err != nil {
			return err
		}
		for k, v := range renamed {
			doc[k] = v
		}
		for _, raw := range nodesOf(doc) {
			n := raw.(map[string]any)
			if n["id"] == oldID {
				n["id"] = newID
			}
			for _, f := range []string{"next", "default", "body"} {
				if n[f] == oldID {
					n[f] = newID
				}
			}
			for _, c := range casesOf(n) {
				if c["to"] == oldID {
					c["to"] = newID
				}
			}
		}

	case "connect", "disconnect":
		from, _ := findNode(doc, str(op, "from"))
		if from == nil {
			return fmt.Errorf("unknown node %q", str(op, "from"))
		}
		to := ""
		if str(op, "op") == "connect" {
			to = str(op, "to")
			if n, _ := findNode(doc, to); n == nil {
				return fmt.Errorf("unknown node %q", to)
			}
		}
		handle := str(op, "handle")
		if handle == "" {
			handle = "next"
		}
		switch {
		case handle == "next" || handle == "default" || handle == "body":
			if to == "" {
				delete(from, handle)
			} else {
				from[handle] = to
			}
		case strings.HasPrefix(handle, "case:"):
			i, err := strconv.Atoi(strings.TrimPrefix(handle, "case:"))
			cases := casesOf(from)
			if err != nil || i < 0 || i >= len(cases) {
				return fmt.Errorf("node %q has no case %q", str(op, "from"), handle)
			}
			cases[i]["to"] = to
		default:
			return fmt.Errorf("unknown handle %q", handle)
		}

	case "set_trigger":
		t, ok := op["trigger"].(map[string]any)
		if !ok {
			return fmt.Errorf("trigger object required")
		}
		doc["trigger"] = t

	case "set":
		field := str(op, "field")
		switch field {
		case "input_schema", "default_input", "ai", "tool_config", "description":
		default:
			return fmt.Errorf("field %q can't be set (use node ops for nodes)", field)
		}
		if op["value"] == nil {
			delete(doc, field)
		} else {
			doc[field] = op["value"]
		}

	default:
		return fmt.Errorf("unknown op %q", str(op, "op"))
	}
	return nil
}

func casesOf(n map[string]any) []map[string]any {
	raw, _ := n["cases"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, c := range raw {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func clearRefs(doc map[string]any, id string) {
	for _, raw := range nodesOf(doc) {
		n := raw.(map[string]any)
		for _, f := range []string{"next", "default", "body"} {
			if n[f] == id {
				delete(n, f)
			}
		}
		for _, c := range casesOf(n) {
			if c["to"] == id {
				c["to"] = ""
			}
		}
	}
}
