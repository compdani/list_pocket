// Package agent runs agentic loops: model turn → tool calls → results →
// repeat, persisting every step to flow_agent_runs / flow_run_steps.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/list_pocket/internal/flow/schema"
	"github.com/compdani/list_pocket/internal/flow/tools"
	"github.com/compdani/list_pocket/internal/flow/web"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	defaultMaxSteps    = 20
	defaultRunTimeout  = 15 * time.Minute
	toolTimeout        = 60 * time.Second
	maxToolResultLen   = 50_000
	submitTool         = "submit_result"
	defaultSearchCap   = 20
	maxCitationExcerpt = 300
)

// Spec is an agent definition (an Agent node in a workflow, or ad hoc).
type Spec struct {
	Name         string         `json:"name"`
	SystemPrompt string         `json:"system_prompt"` // text/template over {"input": …}
	Tools        []string       `json:"tools"`         // catalog names, e.g. "web.search", "web.fetch"
	MaxSteps     int            `json:"max_steps"`
	OutputSchema map[string]any `json:"output_schema"`
	AI           map[string]any `json:"ai"` // node-level overrides
}

type Options struct {
	Input      map[string]any
	WorkflowAI map[string]any // workflow-level AI settings layer
	RunAI      map[string]any // one-off override layer
	NodeRunID  string
	ToolConfig map[string]map[string]any // per-tool config by catalog name (workflow tool_config)
	OnEvent    func(Event)
}

type Event struct {
	Kind string         `json:"kind"` // "delta" | "step" | "done"
	Text string         `json:"text,omitempty"`
	Step map[string]any `json:"step,omitempty"`
}

type Result struct {
	RunID          string         `json:"run_id"`
	Status         string         `json:"status"`
	Output         map[string]any `json:"output"`
	Error          string         `json:"error,omitempty"`
	CostUSD        float64        `json:"cost_usd"`
	Steps          int            `json:"steps"`
	SearchRequests int            `json:"search_requests"`
	CachedTokens   int            `json:"cached_tokens"`
	PromptTokens   int            `json:"prompt_tokens"`
}

type Runner struct {
	App      core.App
	LLM      *llm.Client
	Tools    *tools.Registry
	Resolver *llm.Resolver
}

// Start creates the flow_agent_runs record and runs the loop in the background.
func (r *Runner) Start(spec Spec, opts Options) (string, error) {
	rec, err := r.create(spec, opts)
	if err != nil {
		return "", err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultRunTimeout)
		defer cancel()
		r.execute(ctx, rec, spec, opts)
	}()
	return rec.Id, nil
}

// Run executes synchronously (CLI, tests, and later the workflow engine).
func (r *Runner) Run(ctx context.Context, spec Spec, opts Options) (*Result, error) {
	rec, err := r.create(spec, opts)
	if err != nil {
		return nil, err
	}
	return r.execute(ctx, rec, spec, opts), nil
}

func (r *Runner) create(spec Spec, opts Options) (*core.Record, error) {
	col, err := r.App.FindCachedCollectionByNameOrId("flow_agent_runs")
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(col)
	rec.Set("node_run", opts.NodeRunID)
	rec.Set("status", "queued")
	rec.Set("input", map[string]any{"spec": spec, "input": opts.Input})
	if err := r.App.Save(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// run holds per-execution state.
type run struct {
	r         *Runner
	rec       *core.Record
	spec      Spec
	opts      Options
	settings  llm.Settings
	endpoint  llm.Endpoint
	block     *web.Blocklist
	seq       int
	cost      float64
	promptTok int
	complTok  int
	searches  int
	cachedTok int
	citations []llm.Citation
}

func (r *Runner) execute(ctx context.Context, rec *core.Record, spec Spec, opts Options) *Result {
	st := &run{r: r, rec: rec, spec: spec, opts: opts}
	result, err := st.loop(ctx)
	res := &Result{RunID: rec.Id, CostUSD: st.cost, Steps: st.seq, SearchRequests: st.searches, CachedTokens: st.cachedTok, PromptTokens: st.promptTok}
	if err != nil {
		res.Status = "failed"
		res.Error = err.Error()
		if errors.Is(err, context.Canceled) {
			res.Status = "cancelled"
		}
	} else {
		res.Status = "succeeded"
		res.Output = result
	}
	st.finish(res)
	st.emit(Event{Kind: "done"})
	return res
}

func (st *run) loop(ctx context.Context) (map[string]any, error) {
	r := st.r
	var err error
	st.settings, st.endpoint, err = r.Resolver.Resolve(st.opts.WorkflowAI, st.spec.AI, st.opts.RunAI)
	if err != nil {
		return nil, err
	}
	st.block = LoadBlocklist(r.Resolver)

	st.rec.Set("status", "running")
	st.rec.Set("resolved_ai", st.settings.ToMap())
	st.rec.Set("started", types.NowDateTime())
	if err := r.App.Save(st.rec); err != nil {
		return nil, err
	}
	if err := st.checkGlobalBudget(); err != nil {
		return nil, err
	}

	toolDefs, byWire, useSearch, err := st.buildTools()
	if err != nil {
		return nil, err
	}

	system, err := renderPrompt(st.spec.SystemPrompt, st.opts.Input)
	if err != nil {
		return nil, err
	}
	if st.spec.OutputSchema != nil {
		system += "\n\nWhen you have the final answer, call the `" + submitTool + "` tool exactly once with it. " +
			"Use only facts you found or were given; do not invent data."
	}
	inputJSON, _ := json.MarshalIndent(st.opts.Input, "", "  ")
	messages := []llm.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: "Input:\n```json\n" + string(inputJSON) + "\n```"},
	}

	maxSteps := st.spec.MaxSteps
	if maxSteps <= 0 {
		maxSteps = defaultMaxSteps
	}
	nudges, repairs := 0, 0
	warnedSteps, warnedSearch := false, false

	for turn := 0; turn < maxSteps; turn++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if b := st.settings.Budget.MaxUSDPerRun; b > 0 && st.cost >= b {
			return nil, fmt.Errorf("run budget exceeded: $%.4f of $%.2f", st.cost, b)
		}

		// Wrap-up: some models keep researching until they run out of steps.
		// Warn two steps before the limit, and force the result on the last.
		remaining := maxSteps - turn
		finalTurn := remaining == 1 && st.spec.OutputSchema != nil
		if st.spec.OutputSchema != nil && remaining == 2 && !warnedSteps {
			warnedSteps = true
			messages = append(messages, llm.Message{Role: "user", Content: "You have 2 steps left. Wrap up now: call `" + submitTool + "` with the best result you have (an incomplete result is fine)."})
		}
		searchOK := useSearch && st.searches < st.searchCap() && !finalTurn
		if useSearch && !searchOK && !warnedSearch && !finalTurn {
			warnedSearch = true
			messages = append(messages, llm.Message{Role: "user", Content: fmt.Sprintf("Web search limit reached (%d searches). Finish with what you have; call `%s` when ready.", st.searches, submitTool)})
		}

		req := llm.Request{Model: st.settings.Model, Messages: messages, Tools: toolDefs, Settings: st.settings, Stream: true}
		if searchOK && st.settings.Search.Strategy != "subrequest" {
			web.ApplySearch(&req, st.endpoint, st.settings.Search, st.block)
		}
		if finalTurn {
			req.Extra = map[string]any{"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": submitTool}}}
		}
		if st.settings.ResponseFormat == "json_schema" && st.spec.OutputSchema != nil && !finalTurn {
			req.Extra = map[string]any{"response_format": map[string]any{
				"type":        "json_schema",
				"json_schema": map[string]any{"name": "result", "schema": st.spec.OutputSchema},
			}}
		}

		started := time.Now()
		resp, err := r.LLM.Complete(ctx, st.endpoint, req, func(d string) { st.emit(Event{Kind: "delta", Text: d}) })
		if err != nil {
			st.saveStep("model", map[string]any{"request_tools": len(req.Tools)}, nil, time.Since(started), err.Error())
			return nil, err
		}
		stepCost := st.account(resp)
		st.saveStep("model", map[string]any{
			"content":       resp.Message.Content,
			"tool_calls":    resp.Message.ToolCalls,
			"citations":     resp.Citations,
			"finish_reason": resp.FinishReason,
			"model":         resp.Model,
		}, map[string]any{
			"prompt_tokens":       resp.Usage.PromptTokens,
			"completion_tokens":   resp.Usage.CompletionTokens,
			"web_search_requests": resp.Usage.WebSearchRequests,
			"cached_tokens":       resp.Usage.CachedTokens,
			"cache_write_tokens":  resp.Usage.CacheWriteTokens,
			"cost_usd":            stepCost,
		}, time.Since(started), "")
		messages = append(messages, resp.Message)

		if len(resp.Message.ToolCalls) == 0 {
			if st.spec.OutputSchema == nil {
				return map[string]any{"text": resp.Message.Content}, nil
			}
			if st.settings.ResponseFormat == "json_schema" {
				if out, problems := parseResult(resp.Message.Content, st.spec.OutputSchema); problems == "" {
					return out, nil
				}
			}
			nudges++
			if nudges > 2 {
				return nil, errors.New("model did not submit a result")
			}
			messages = append(messages, llm.Message{Role: "user", Content: "Call the `" + submitTool + "` tool with your final answer now."})
			continue
		}

		var final map[string]any
		for _, call := range resp.Message.ToolCalls {
			content, out := st.handleCall(ctx, call, byWire, &repairs)
			if out != nil {
				final = out
			}
			messages = append(messages, llm.Message{Role: "tool", ToolCallID: call.ID, Content: content})
		}
		if final != nil {
			return final, nil
		}
		if repairs > 1 {
			return nil, errors.New("result failed validation after a repair attempt")
		}
	}
	return nil, fmt.Errorf("stopped after %d steps without a result", maxSteps)
}

// buildTools returns function definitions, a wire-name → catalog-name map,
// and whether provider web search is requested.
func (st *run) buildTools() ([]map[string]any, map[string]tools.Tool, bool, error) {
	var defs []map[string]any
	byWire := map[string]tools.Tool{}
	useSearch := false
	for _, name := range st.spec.Tools {
		if name == tools.WebSearch {
			if st.settings.Search.Mode == "off" {
				continue
			}
			useSearch = true
			if st.settings.Search.Strategy == "subrequest" {
				t := &searchSubrequest{st: st}
				defs = append(defs, tools.Definition(t))
				byWire[tools.WireName(t.Name())] = t
			}
			continue
		}
		t, ok := st.r.Tools.Get(name)
		if !ok {
			return nil, nil, false, fmt.Errorf("unknown tool %q", name)
		}
		defs = append(defs, tools.Definition(t))
		byWire[tools.WireName(name)] = t
	}
	if st.spec.OutputSchema != nil {
		defs = append(defs, llm.FunctionTool(submitTool, "Submit the final result. Call exactly once when done.", submitParams(st.spec.OutputSchema)))
	}
	return defs, byWire, useSearch, nil
}

// handleCall runs one tool call and returns the tool message content, plus
// the final result when the call was a valid submit_result.
func (st *run) handleCall(ctx context.Context, call llm.ToolCall, byWire map[string]tools.Tool, repairs *int) (string, map[string]any) {
	started := time.Now()
	name := call.Function.Name

	if name == submitTool {
		out, problems := parseResult(unwrapSubmit(call.Function.Arguments, st.spec.OutputSchema), st.spec.OutputSchema)
		if problems != "" {
			*repairs++
			msg := "Result rejected: " + problems + ". Fix it and call " + submitTool + " again."
			st.saveStep("tool", map[string]any{"name": submitTool, "arguments": call.Function.Arguments, "result": msg}, nil, time.Since(started), problems)
			return msg, nil
		}
		st.saveStep("tool", map[string]any{"name": submitTool, "arguments": call.Function.Arguments, "result": "accepted"}, nil, time.Since(started), "")
		return "accepted", out
	}

	t, ok := byWire[name]
	if !ok {
		msg := fmt.Sprintf("error: unknown tool %q", name)
		st.saveStep("tool", map[string]any{"name": name, "arguments": call.Function.Arguments, "result": msg}, nil, time.Since(started), msg)
		return msg, nil
	}
	if t.RequiresApproval() {
		// Approval pauses arrive with the workflow engine (Phase 3).
		msg := "error: this tool requires human approval, which is not available in this context"
		st.saveStep("tool", map[string]any{"name": t.Name(), "arguments": call.Function.Arguments, "result": msg}, nil, time.Since(started), msg)
		return msg, nil
	}

	args, err := tools.ParseArgs(call.Function.Arguments)
	if err == nil {
		// A model wrote everything here, and no person is checking this call.
		if field := tools.HumanWrittenField(t, args); field != "" {
			msg := fmt.Sprintf("error: with these arguments a person has to check %s first, which is not available here. Use plain text, or draft it for a review step", field)
			st.saveStep("tool", map[string]any{"name": t.Name(), "arguments": call.Function.Arguments, "result": msg}, nil, time.Since(started), msg)
			return msg, nil
		}
	}
	var result any
	if err == nil {
		tctx, cancel := context.WithTimeout(ctx, toolTimeout)
		result, err = tools.Invoke(tctx, t, tools.Call{Args: args, Config: st.opts.ToolConfig[t.Name()], RunID: st.rec.Id})
		cancel()
	}
	var content string
	if err != nil {
		content = "error: " + err.Error()
	} else {
		raw, _ := json.Marshal(result)
		content = string(raw)
	}
	if len(content) > maxToolResultLen {
		content = content[:maxToolResultLen] + "…(truncated)"
	}
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	st.saveStep("tool", map[string]any{"name": t.Name(), "arguments": args, "result": json.RawMessage(mustJSON(content))}, nil, time.Since(started), errStr)
	return content, nil
}

// searchCap is the most provider searches one agent run may make.
func (st *run) searchCap() int {
	if n := st.settings.Search.MaxSearchesPerRun; n > 0 {
		return n
	}
	return defaultSearchCap
}

func (st *run) account(resp *llm.Response) float64 {
	model := resp.Model
	if model == "" {
		model = st.settings.Model
	}
	c := st.endpoint.Cost(model, resp.Usage)
	st.cost += c
	st.promptTok += resp.Usage.PromptTokens
	st.complTok += resp.Usage.CompletionTokens
	st.searches += resp.Usage.WebSearchRequests
	st.cachedTok += resp.Usage.CachedTokens
	st.citations = append(st.citations, resp.Citations...)
	return c
}

func (st *run) saveStep(typ string, payload, usage map[string]any, dur time.Duration, errStr string) {
	st.seq++
	col, err := st.r.App.FindCachedCollectionByNameOrId("flow_run_steps")
	if err != nil {
		st.r.App.Logger().Error("flow_run_steps missing", "error", err)
		return
	}
	step := core.NewRecord(col)
	step.Set("agent_run", st.rec.Id)
	step.Set("seq", st.seq)
	step.Set("type", typ)
	step.Set("payload", payload)
	step.Set("usage", usage)
	step.Set("duration_ms", dur.Milliseconds())
	step.Set("error", errStr)
	if err := st.r.App.Save(step); err != nil {
		st.r.App.Logger().Error("saving run step failed", "run", st.rec.Id, "error", err)
	}
	st.emit(Event{Kind: "step", Step: map[string]any{"seq": st.seq, "type": typ, "payload": payload, "usage": usage, "error": errStr}})
}

func (st *run) finish(res *Result) {
	kept, dropped := web.FilterCitations(st.citations, st.block)
	for i := range kept {
		// Full excerpts stay in flow_run_steps; the output keeps a short preview.
		if r := []rune(kept[i].Content); len(r) > maxCitationExcerpt {
			kept[i].Content = string(r[:maxCitationExcerpt]) + "…"
		}
	}
	if res.Output != nil {
		cleaned, removed := web.DropBlockedSources(any(res.Output), st.block)
		res.Output, _ = cleaned.(map[string]any)
		if len(removed) > 0 {
			res.Output["_dropped_sources"] = removed
		}
		res.Output["_citations"] = kept
	}
	if len(dropped) > 0 {
		st.r.App.Logger().Warn("dropped blocklisted citations", "run", st.rec.Id, "count", len(dropped))
	}

	st.rec.Set("status", res.Status)
	st.rec.Set("output", res.Output)
	st.rec.Set("error", res.Error)
	st.rec.Set("steps", st.seq)
	st.rec.Set("prompt_tokens", st.promptTok)
	st.rec.Set("completion_tokens", st.complTok)
	st.rec.Set("search_requests", st.searches)
	st.rec.Set("cached_tokens", st.cachedTok)
	st.rec.Set("cost_usd", st.cost)
	st.rec.Set("finished", types.NowDateTime())
	if err := st.r.App.Save(st.rec); err != nil {
		st.r.App.Logger().Error("saving agent run failed", "run", st.rec.Id, "error", err)
	}
}

func (st *run) emit(e Event) {
	if st.opts.OnEvent != nil {
		st.opts.OnEvent(e)
	}
}

// checkGlobalBudget refuses to start when today's spend across all agent
// runs has reached budgets.max_usd_per_day.
func (st *run) checkGlobalBudget() error {
	v, err := st.r.Resolver.Setting("budgets")
	if err != nil {
		return nil
	}
	m, _ := v.(map[string]any)
	limit, _ := m["max_usd_per_day"].(float64)
	if limit <= 0 {
		return nil
	}
	today := time.Now().UTC().Format("2006-01-02") + " 00:00:00.000Z"
	var spent float64
	err = st.r.App.DB().NewQuery("SELECT COALESCE(SUM(cost_usd), 0) FROM flow_agent_runs WHERE created >= {:today}").
		Bind(map[string]any{"today": today}).Row(&spent)
	if err != nil {
		return err
	}
	if spent >= limit {
		return fmt.Errorf("global daily budget reached: $%.2f of $%.2f", spent, limit)
	}
	return nil
}

// LoadBlocklist reads the domain_blocklist setting.
func LoadBlocklist(res *llm.Resolver) *web.Blocklist {
	v, err := res.Setting("domain_blocklist")
	if err != nil {
		return web.NewBlocklist(nil)
	}
	var patterns []string
	if list, ok := v.([]any); ok {
		for _, p := range list {
			if s, ok := p.(string); ok {
				patterns = append(patterns, s)
			}
		}
	}
	return web.NewBlocklist(patterns)
}

func renderPrompt(tmpl string, input map[string]any) (string, error) {
	t, err := template.New("system").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("system prompt template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, map[string]any{"input": input}); err != nil {
		return "", fmt.Errorf("system prompt template: %w", err)
	}
	return strings.ReplaceAll(buf.String(), "<no value>", ""), nil
}

// submitParams makes the submit_result parameters: the output schema itself
// when it's an object schema, else wrapped as {"result": schema}.
func submitParams(s map[string]any) map[string]any {
	if s["type"] == "object" {
		return s
	}
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"result": s},
		"required":   []string{"result"},
	}
}

func unwrapSubmit(args string, s map[string]any) string {
	if s["type"] == "object" {
		return args
	}
	var wrapped struct {
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(args), &wrapped) == nil && len(wrapped.Result) > 0 {
		return string(wrapped.Result)
	}
	return args
}

// parseResult decodes raw JSON and validates it; non-object results are
// returned as {"result": value}.
func parseResult(raw string, s map[string]any) (map[string]any, string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(strings.TrimSuffix(raw, "```"), "```json")
	var v any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &v); err != nil {
		return nil, "not valid JSON: " + err.Error()
	}
	if errs := schema.Validate(s, v); len(errs) > 0 {
		return nil, schema.Summary(errs)
	}
	if m, ok := v.(map[string]any); ok {
		return map[string]any{"result": m}, ""
	}
	return map[string]any{"result": v}, ""
}

func mustJSON(content string) []byte {
	if json.Valid([]byte(content)) {
		return []byte(content)
	}
	raw, _ := json.Marshal(content)
	return raw
}
