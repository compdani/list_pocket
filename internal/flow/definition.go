// Package flow implements versioned workflow graphs and the durable engine that runs
// them, with agent, tool, branch, loop, review and wait steps.
package flow

import (
	"encoding/json"
	"time"
)

// Definition is a workflow version: what workflows.draft and
// flow_versions.definition store.
type Definition struct {
	Name         string                    `json:"name"`
	Description  string                    `json:"description,omitempty"`
	Trigger      Trigger                   `json:"trigger"`
	InputSchema  map[string]any            `json:"input_schema,omitempty"`
	DefaultInput map[string]any            `json:"default_input,omitempty"`
	AI           map[string]any            `json:"ai,omitempty"` // workflow AI settings layer
	ToolConfig   map[string]map[string]any `json:"tool_config,omitempty"`
	Nodes        []Node                    `json:"nodes"`
}

type Trigger struct {
	Type string `json:"type"` // manual | cron | event | webhook

	// cron
	Spec     string `json:"spec,omitempty"`
	Timezone string `json:"timezone,omitempty"`
	Overlap  string `json:"overlap,omitempty"` // skip (default) | queue
	StartAt  string `json:"start_at,omitempty"`
	EndAt    string `json:"end_at,omitempty"`
	MaxRuns  int    `json:"max_runs,omitempty"`
	// Input for cron runs (templated against {now}); defaults to default_input.
	Input map[string]any `json:"input,omitempty"`

	// event: a List Pocket event ("inbound.email") or a record change.
	// webhook: a POST to the workflow's secret URL. Filter and InputMap see the body
	// as {event}.
	Event      string   `json:"event,omitempty"`
	Collection string   `json:"collection,omitempty"`
	On         []string `json:"on,omitempty"`     // create | update
	Filter     string   `json:"filter,omitempty"` // expr over {event} / {record}
	// InputMap builds the run input from the event (templated); defaults to the payload.
	InputMap map[string]any `json:"input_map,omitempty"`
}

// Node types.
const (
	NodeStart       = "start"
	NodeAgent       = "agent"
	NodeTool        = "tool"
	NodeBranch      = "branch"
	NodeForEach     = "foreach"
	NodeLoop        = "loop"
	NodeReview      = "review"
	NodeWait        = "wait"
	NodeSubworkflow = "subworkflow"
	NodeNotify      = "notify"
	NodeTransform   = "transform"
	NodeEnd         = "end"
)

var nodeTypes = map[string]bool{
	NodeStart: true, NodeAgent: true, NodeTool: true, NodeBranch: true, NodeForEach: true, NodeLoop: true, NodeReview: true,
	NodeWait: true, NodeSubworkflow: true, NodeNotify: true, NodeTransform: true, NodeEnd: true,
}

type Node struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	// Next is the successor; empty ends the path (or the ForEach item).
	Next string `json:"next,omitempty"`

	// Input is templated with {{ }} and passed to agent/tool/subworkflow nodes.
	Input map[string]any `json:"input,omitempty"`

	Agent *AgentNode `json:"agent,omitempty"`
	Tool  string     `json:"tool,omitempty"`

	// branch: first matching case wins; Default when none match ("" ends the path).
	Cases   []Case `json:"cases,omitempty"`
	Default string `json:"default,omitempty"`

	// foreach: Items is an expression producing a list; Body is the first
	// node of the per-item chain; Next runs after every item finished.
	Items       string `json:"items,omitempty"`
	Body        string `json:"body,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`

	// loop: runs Body (one iteration at a time) until Until is true or
	// MaxIterations is reached; Next runs after the loop.
	Until         string `json:"until,omitempty"`
	MaxIterations int    `json:"max_iterations,omitempty"`

	Review *ReviewNode `json:"review,omitempty"`
	Wait   *WaitNode   `json:"wait,omitempty"`

	Workflow string `json:"workflow,omitempty"` // subworkflow name
	// Detach starts the subworkflow and continues at once (output {run_id})
	// instead of waiting for it to finish.
	Detach bool `json:"detach,omitempty"`

	Message string `json:"message,omitempty"` // notify (templated)
	Value   any    `json:"value,omitempty"`   // transform (templated)
	Output  any    `json:"output,omitempty"`  // end (templated)

	OnError string `json:"on_error,omitempty"` // fail (default) | continue
	Retries int    `json:"retries,omitempty"`

	// UI holds canvas layout; the engine ignores it.
	UI *NodeUI `json:"ui,omitempty"`
}

type NodeUI struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Case struct {
	If string `json:"if"`
	To string `json:"to"` // node id; "" ends the path (or the ForEach item / Loop iteration)
}

type AgentNode struct {
	Preset       string         `json:"preset,omitempty"` // "<plugin>.<preset>"
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Tools        []string       `json:"tools,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	MaxSteps     int            `json:"max_steps,omitempty"`
	AI           map[string]any `json:"ai,omitempty"`
}

type ReviewNode struct {
	Title        string `json:"title,omitempty"`        // templated
	Instructions string `json:"instructions,omitempty"` // templated
	Data         any    `json:"data,omitempty"`         // templated; what the reviewer sees/edits
	AutoApprove  string `json:"auto_approve,omitempty"` // expr; true → approved without a human
	OnDeny       string `json:"on_deny,omitempty"`      // skip (default: end this path/item) | continue
	Timeout      string `json:"timeout,omitempty"`      // e.g. "72h"; expires → treated as deny
}

type WaitNode struct {
	Delay   string `json:"delay,omitempty"`   // e.g. "10m"
	Event   string `json:"event,omitempty"`   // event to wait for
	Match   string `json:"match,omitempty"`   // expr over {event, input, nodes, item}
	Timeout string `json:"timeout,omitempty"` // e.g. "72h"; output {timed_out: true}
}

// Node returns the node with id, or nil.
func (d *Definition) Node(id string) *Node {
	for i := range d.Nodes {
		if d.Nodes[i].ID == id {
			return &d.Nodes[i]
		}
	}
	return nil
}

// StartNode returns the start node, or nil.
func (d *Definition) StartNode() *Node {
	for i := range d.Nodes {
		if d.Nodes[i].Type == NodeStart {
			return &d.Nodes[i]
		}
	}
	return nil
}

// ToolNames lists every catalog tool the workflow references.
func (d *Definition) ToolNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, n := range d.Nodes {
		add(n.Tool)
		if n.Agent != nil {
			for _, t := range n.Agent.Tools {
				add(t)
			}
		}
	}
	return out
}

// DecodeDefinition converts a stored JSON value into a Definition.
func DecodeDefinition(raw any) (*Definition, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var d Definition
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}
