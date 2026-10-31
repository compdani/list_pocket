// Package tools holds the tool registry and built-in tools.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/list_pocket/internal/flow/schema"
)

// Tool is a function an agent can call.
type Tool interface {
	Name() string // catalog name, e.g. "web.fetch" or "instantly.add_lead"
	Description() string
	InputSchema() map[string]any
	RequiresApproval() bool
	Invoke(ctx context.Context, call Call) (any, error)
}

// ReadOnlyTool is implemented by tools without side effects (lookups,
// fetches). The assistant calls these freely; anything else is proposed.
type ReadOnlyTool interface {
	ReadOnly() bool
}

// HumanWrittenTool is implemented by tools where, for some inputs, one field is
// only safe to act on unchecked if a person wrote it, not a model or run data.
type HumanWrittenTool interface {
	HumanWrittenField(args map[string]any) string
}

// HumanWrittenField names the field of args that t needs a person to have written,
// or returns "" if there is none.
func HumanWrittenField(t Tool, args map[string]any) string {
	if h, ok := t.(HumanWrittenTool); ok {
		return h.HumanWrittenField(args)
	}
	return ""
}

// IsReadOnly reports whether t declares itself side-effect free.
func IsReadOnly(t Tool) bool {
	ro, ok := t.(ReadOnlyTool)
	return ok && ro.ReadOnly()
}

// Call is one invocation.
type Call struct {
	Args   map[string]any
	Config map[string]any // per-workflow tool config (Phase 2+)
	RunID  string         // flow_agent_runs id
}

// ProviderSide marks catalog names that are fulfilled by the model provider
// rather than invoked by us (the agent loop wires them into the request).
const WebSearch = "web.search"

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for n := range r.tools {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Invoke validates args against the tool's input schema, then calls it.
func Invoke(ctx context.Context, t Tool, call Call) (any, error) {
	var asAny any = map[string]any(call.Args)
	if call.Args == nil {
		asAny = map[string]any{}
	}
	if errs := schema.Validate(t.InputSchema(), asAny); len(errs) > 0 {
		return nil, fmt.Errorf("invalid arguments: %s", schema.Summary(errs))
	}
	return t.Invoke(ctx, call)
}

// ParseArgs decodes a model's JSON arguments string.
func ParseArgs(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, fmt.Errorf("arguments are not a JSON object: %w", err)
	}
	return args, nil
}

var wireUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// WireName maps a catalog name to an OpenAI-safe function name
// (^[a-zA-Z0-9_-]{1,64}$): "web.fetch" → "web__fetch".
func WireName(name string) string {
	w := wireUnsafe.ReplaceAllString(strings.ReplaceAll(name, ".", "__"), "_")
	if len(w) > 64 {
		w = w[:64]
	}
	return w
}

// Definition is the function tool definition sent to the model.
func Definition(t Tool) map[string]any {
	return llm.FunctionTool(WireName(t.Name()), t.Description(), t.InputSchema())
}
