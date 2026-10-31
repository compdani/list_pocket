package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/compdani/list_pocket/internal/flow/agent"
	"github.com/compdani/list_pocket/internal/flow/queue"
	"github.com/compdani/list_pocket/internal/flow/schema"
	"github.com/compdani/list_pocket/internal/flow/tools"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	JobAdvance          = "workflow.advance"
	maxSubworkflowDepth = 3
	defaultConcurrency  = 4
	toolNodeTimeout     = 2 * time.Minute
)

// Run statuses.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusWaiting   = "waiting"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusSkipped   = "skipped"
)

func final(status string) bool {
	return status == StatusSucceeded || status == StatusFailed || status == StatusCancelled
}

// PresetSource looks up plugin agent presets.
type PresetSource interface {
	Preset(name string) (map[string]any, bool)
}

type Engine struct {
	App     core.App
	Runner  *agent.Runner
	Tools   *tools.Registry
	Presets PresetSource
	Queue   *queue.Queue
	// Notify delivers notify-node messages. Default: log.
	Notify func(run *core.Record, message string)

	mu       sync.Mutex
	runLocks map[string]*sync.Mutex
	cancels  map[string]context.CancelFunc
	triggers *triggerIndex
}

func NewEngine(app core.App, runner *agent.Runner, presets PresetSource, q *queue.Queue) *Engine {
	e := &Engine{
		App:      app,
		Runner:   runner,
		Tools:    runner.Tools,
		Presets:  presets,
		Queue:    q,
		runLocks: map[string]*sync.Mutex{},
		cancels:  map[string]context.CancelFunc{},
		triggers: &triggerIndex{},
	}
	q.Handle(JobAdvance, func(ctx context.Context, job queue.Job) error {
		return e.Advance(ctx, job.Target)
	})
	q.OnDead = func(job queue.Job, err error) {
		if job.Kind == JobAdvance {
			if run, ferr := app.FindRecordById("flow_runs", job.Target); ferr == nil && !final(run.GetString("status")) {
				e.finishRun(run, StatusFailed, nil, "engine error: "+err.Error())
			}
		}
	}
	return e
}

func (e *Engine) runLock(id string) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()
	l, ok := e.runLocks[id]
	if !ok {
		l = &sync.Mutex{}
		e.runLocks[id] = l
	}
	return l
}

// --- starting runs ---

type StartOptions struct {
	Source       string // manual_ui | cron | event | webhook | subworkflow | test
	StartedBy    string // users id
	ScheduledFor time.Time
	ParentRun    string
}

var ErrNotPublished = errors.New("workflow has no published version")

// PublishedDefinition loads the workflow's published definition.
func (e *Engine) PublishedDefinition(wf *core.Record) (*Definition, int, error) {
	v := wf.GetInt("published_version")
	if v == 0 {
		return nil, 0, ErrNotPublished
	}
	d, err := e.definitionAt(wf.Id, v)
	return d, v, err
}

func (e *Engine) definitionAt(workflowID string, version int) (*Definition, error) {
	vr, err := e.App.FindFirstRecordByFilter("flow_versions", "workflow = {:w} && version = {:v}", dbx.Params{"w": workflowID, "v": version})
	if err != nil {
		return nil, fmt.Errorf("workflow version %d not found", version)
	}
	return DecodeDefinition(vr.Get("definition"))
}

// runDefinition returns the definition a run executes: its snapshot for
// draft test runs (version 0), else the published version it started on.
func (e *Engine) runDefinition(run *core.Record) (*Definition, error) {
	if run.GetInt("version") == 0 {
		return DecodeDefinition(run.Get("test_definition"))
	}
	return e.definitionAt(run.GetString("workflow"), run.GetInt("version"))
}

// StartRun creates a run of the published version and queues it.
func (e *Engine) StartRun(wf *core.Record, input map[string]any, opts StartOptions) (*core.Record, error) {
	def, version, err := e.PublishedDefinition(wf)
	if err != nil {
		return nil, err
	}
	return e.startRun(wf, def, version, input, opts)
}

// StartTestRun runs the current draft without publishing it. Tools run for
// real; the run is marked source=test and version 0.
func (e *Engine) StartTestRun(wf *core.Record, input map[string]any, startedBy string) (*core.Record, Validation, error) {
	def, err := e.Draft(wf)
	if err != nil {
		return nil, Validation{}, err
	}
	v := e.ValidateDefinition(def)
	if !v.OK {
		return nil, v, ErrInvalid
	}
	run, err := e.startRun(wf, def, 0, input, StartOptions{Source: "test", StartedBy: startedBy})
	return run, v, err
}

// ContactsInActiveRuns returns the contacts with an unfinished run of this workflow,
// going by the input.contact convention. Test runs don't count.
func (e *Engine) ContactsInActiveRuns(wf *core.Record) (map[string]bool, error) {
	var contacts []string
	err := e.App.DB().NewQuery(`SELECT DISTINCT json_extract(input, '$.contact') FROM flow_runs
		WHERE workflow = {:w} AND source != 'test' AND status IN ({:queued}, {:running}, {:waiting})
		AND json_extract(input, '$.contact') IS NOT NULL`).
		Bind(dbx.Params{"w": wf.Id, "queued": StatusQueued, "running": StatusRunning, "waiting": StatusWaiting}).
		Column(&contacts)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(contacts))
	for _, c := range contacts {
		out[c] = true
	}
	return out, nil
}

func (e *Engine) startRun(wf *core.Record, def *Definition, version int, input map[string]any, opts StartOptions) (*core.Record, error) {
	merged := map[string]any{}
	for k, v := range def.DefaultInput {
		merged[k] = v
	}
	for k, v := range input {
		merged[k] = v
	}
	if def.InputSchema != nil {
		if errs := schema.Validate(def.InputSchema, any(merged)); len(errs) > 0 {
			return nil, fmt.Errorf("invalid input: %s", schema.Summary(errs))
		}
	}
	if opts.ParentRun != "" && e.depth(opts.ParentRun) >= maxSubworkflowDepth {
		return nil, fmt.Errorf("subworkflow depth limit (%d) reached", maxSubworkflowDepth)
	}

	col, err := e.App.FindCachedCollectionByNameOrId("flow_runs")
	if err != nil {
		return nil, err
	}
	run := core.NewRecord(col)
	run.Set("workflow", wf.Id)
	run.Set("version", version)
	run.Set("status", StatusQueued)
	run.Set("source", opts.Source)
	run.Set("started_by", opts.StartedBy)
	run.Set("input", merged)
	run.Set("parent_run", opts.ParentRun)
	if version == 0 {
		run.Set("test_definition", def)
	}
	if !opts.ScheduledFor.IsZero() {
		run.Set("scheduled_for", opts.ScheduledFor.UTC())
	}
	if err := e.App.Save(run); err != nil {
		return nil, err
	}
	return run, e.Queue.Enqueue(JobAdvance, run.Id, nil, time.Time{})
}

func (e *Engine) depth(runID string) int {
	d := 0
	for runID != "" && d <= maxSubworkflowDepth {
		r, err := e.App.FindRecordById("flow_runs", runID)
		if err != nil {
			break
		}
		runID = r.GetString("parent_run")
		d++
	}
	return d
}

// --- advancing ---

// Advance moves a run forward as far as it can: executes ready nodes,
// stops at waiting nodes, and finishes the run when every path is done.
// It is idempotent: completed node runs are replayed from storage.
func (e *Engine) Advance(ctx context.Context, runID string) error {
	l := e.runLock(runID)
	l.Lock()
	defer l.Unlock()

	run, err := e.App.FindRecordById("flow_runs", runID)
	if err != nil {
		return nil // deleted
	}
	if final(run.GetString("status")) {
		return nil
	}
	def, err := e.runDefinition(run)
	if err != nil {
		e.finishRun(run, StatusFailed, nil, err.Error())
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.cancels[runID] = cancel
	e.mu.Unlock()
	defer func() {
		cancel()
		e.mu.Lock()
		delete(e.cancels, runID)
		e.mu.Unlock()
	}()

	run.Set("status", StatusRunning)
	if run.GetDateTime("started").IsZero() {
		run.Set("started", types.NowDateTime())
	}
	if err := e.App.Save(run); err != nil {
		return err
	}

	st, err := e.loadState(run, def)
	if err != nil {
		return err
	}
	res := st.walk(ctx, def.StartNode().ID, -1, nil)

	// Re-read: a cancel may have landed while we were running.
	if fresh, err := e.App.FindRecordById("flow_runs", runID); err == nil && fresh.GetString("status") == StatusCancelled {
		return nil
	}
	switch res.state {
	case pathDone:
		out := st.output
		if out == nil {
			out = res.out
		}
		e.finishRun(run, StatusSucceeded, out, "")
	case pathWaiting:
		e.updateTotals(run)
		run.Set("status", StatusWaiting)
		if err := e.App.Save(run); err != nil {
			return err
		}
		if wake := st.earliestWake(); !wake.IsZero() {
			return e.Queue.Enqueue(JobAdvance, runID, map[string]any{"wake": true}, wake)
		}
	case pathFailed:
		if ctx.Err() != nil && res.err != nil && errors.Is(res.err, context.Canceled) {
			return nil
		}
		e.finishRun(run, StatusFailed, nil, res.err.Error())
	}
	return nil
}

func (e *Engine) finishRun(run *core.Record, status string, output any, errMsg string) {
	e.updateTotals(run)
	run.Set("status", status)
	run.Set("output", output)
	run.Set("error", errMsg)
	run.Set("finished", types.NowDateTime())
	if err := e.App.Save(run); err != nil {
		e.App.Logger().Error("saving workflow run failed", "run", run.Id, "error", err)
		return
	}
	if parent := run.GetString("parent_run"); parent != "" {
		e.resolveSubworkflow(parent, run)
	}
}

func (e *Engine) updateTotals(run *core.Record) {
	var totals struct {
		Cost   float64 `db:"cost"`
		Tokens int     `db:"tokens"`
	}
	_ = e.App.DB().NewQuery(`SELECT COALESCE(SUM(a.cost_usd),0) AS cost, COALESCE(SUM(a.prompt_tokens + a.completion_tokens),0) AS tokens
		FROM flow_agent_runs a JOIN flow_node_runs n ON a.node_run = n.id WHERE n.workflow_run = {:r}`).
		Bind(dbx.Params{"r": run.Id}).One(&totals)
	run.Set("cost_usd", totals.Cost)
	run.Set("tokens", totals.Tokens)
}

// Cancel stops a run: in-flight work is interrupted and pending approvals expire.
func (e *Engine) Cancel(runID string) error {
	run, err := e.App.FindRecordById("flow_runs", runID)
	if err != nil {
		return err
	}
	if final(run.GetString("status")) {
		return fmt.Errorf("run already %s", run.GetString("status"))
	}
	e.mu.Lock()
	if c := e.cancels[runID]; c != nil {
		c()
	}
	e.mu.Unlock()

	run.Set("status", StatusCancelled)
	run.Set("finished", types.NowDateTime())
	if err := e.App.Save(run); err != nil {
		return err
	}
	nodeRuns, _ := e.App.FindRecordsByFilter("flow_node_runs", "workflow_run = {:r} && (status = 'waiting' || status = 'running')", "", 0, 0, dbx.Params{"r": runID})
	for _, nr := range nodeRuns {
		nr.Set("status", StatusCancelled)
		_ = e.App.Save(nr)
		if a, err := e.App.FindFirstRecordByFilter("flow_approvals", "node_run = {:n} && status = 'pending'", dbx.Params{"n": nr.Id}); err == nil {
			a.Set("status", "expired")
			_ = e.App.Save(a)
		}
		if child := nr.GetString("child_run"); child != "" {
			_ = e.Cancel(child)
		}
	}
	return nil
}

// Recover queues every unfinished run (on serve), so runs interrupted by a
// restart continue and waiting runs get their wake-ups rescheduled.
func (e *Engine) Recover() error {
	runs, err := e.App.FindRecordsByFilter("flow_runs", "status = 'queued' || status = 'running' || status = 'waiting'", "", 0, 0)
	if err != nil {
		return err
	}
	for _, r := range runs {
		if err := e.Queue.Enqueue(JobAdvance, r.Id, nil, time.Time{}); err != nil {
			return err
		}
	}
	return nil
}

// --- run state ---

type pathState int

const (
	pathDone pathState = iota
	pathWaiting
	pathFailed
)

type pathResult struct {
	state pathState
	out   any
	err   error
}

type runState struct {
	e      *Engine
	run    *core.Record
	def    *Definition
	mu     sync.Mutex
	nrs    map[string]*core.Record // "<node>|<idx>"
	main   map[string]any          // main-path node outputs
	items  map[int]map[string]any  // per-item body outputs
	output any                     // set by a main-path end node
}

func nrKey(node string, idx int) string { return fmt.Sprintf("%s|%d", node, idx) }

func (e *Engine) loadState(run *core.Record, def *Definition) (*runState, error) {
	recs, err := e.App.FindRecordsByFilter("flow_node_runs", "workflow_run = {:r}", "created", 0, 0, dbx.Params{"r": run.Id})
	if err != nil {
		return nil, err
	}
	st := &runState{e: e, run: run, def: def, nrs: map[string]*core.Record{}, main: map[string]any{}, items: map[int]map[string]any{}}
	for _, r := range recs {
		st.nrs[nrKey(r.GetString("node_id"), r.GetInt("item_index"))] = r
	}
	return st, nil
}

func (st *runState) get(node string, idx int) *core.Record {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.nrs[nrKey(node, idx)]
}

func (st *runState) put(nr *core.Record) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.nrs[nrKey(nr.GetString("node_id"), nr.GetInt("item_index"))] = nr
}

func (st *runState) setOutput(node string, idx int, out any) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if idx < 0 {
		st.main[node] = out
		return
	}
	if st.items[idx] == nil {
		st.items[idx] = map[string]any{}
	}
	st.items[idx][node] = out
}

// env is the expression environment for a node at item idx.
func (st *runState) env(idx int, item any) map[string]any {
	st.mu.Lock()
	defer st.mu.Unlock()
	nodes := make(map[string]any, len(st.main))
	for k, v := range st.main {
		nodes[k] = v
	}
	for k, v := range st.items[idx] {
		nodes[k] = v
	}
	var input any
	_ = st.run.UnmarshalJSONField("input", &input)
	env := map[string]any{
		"input": input,
		"nodes": nodes,
		"run":   map[string]any{"id": st.run.Id, "source": st.run.GetString("source")},
		"now":   time.Now().UTC().Format(time.RFC3339),
	}
	if idx >= 0 {
		if it, ok := item.(loopItem); ok {
			// "last" is an expr builtin, so loop state lives under loop.
			env["loop"] = map[string]any{"iteration": it.iteration, "last": it.last}
		} else {
			env["item"] = item
		}
		env["index"] = idx
	}
	return env
}

func (st *runState) earliestWake() time.Time {
	st.mu.Lock()
	defer st.mu.Unlock()
	var min time.Time
	for _, nr := range st.nrs {
		if nr.GetString("status") != StatusWaiting {
			continue
		}
		w := nr.GetDateTime("wake_at").Time()
		if !w.IsZero() && (min.IsZero() || w.Before(min)) {
			min = w
		}
	}
	return min
}

// walk runs the chain starting at id for item idx (-1 = main path).
func (st *runState) walk(ctx context.Context, id string, idx int, item any) pathResult {
	var last any
	for id != "" {
		if err := ctx.Err(); err != nil {
			return pathResult{state: pathFailed, err: err}
		}
		n := st.def.Node(id)
		if n == nil {
			return pathResult{state: pathFailed, err: fmt.Errorf("unknown node %q", id)}
		}
		out, state, err := st.step(ctx, n, idx, item)
		switch state {
		case pathWaiting:
			return pathResult{state: pathWaiting}
		case pathFailed:
			return pathResult{state: pathFailed, err: fmt.Errorf("node %s: %w", n.ID, err)}
		}
		st.setOutput(n.ID, idx, out)
		last = out
		if n.Type == NodeEnd && idx < 0 {
			st.output = out
		}
		id = st.successor(n, out)
	}
	return pathResult{state: pathDone, out: last}
}

func (st *runState) successor(n *Node, out any) string {
	m, _ := out.(map[string]any)
	switch n.Type {
	case NodeBranch:
		next, _ := m["_next"].(string)
		return next
	case NodeReview:
		if approved, _ := m["approved"].(bool); !approved && n.Review != nil && n.Review.OnDeny != "continue" {
			return ""
		}
	case NodeTool:
		if denied, _ := m["_denied"].(bool); denied {
			return ""
		}
	case NodeEnd:
		return ""
	}
	return n.Next
}

// step returns the node's output, executing or resuming it as needed.
func (st *runState) step(ctx context.Context, n *Node, idx int, item any) (any, pathState, error) {
	nr := st.get(n.ID, idx)
	if nr != nil {
		switch nr.GetString("status") {
		case StatusSucceeded, StatusSkipped:
			return decodeOutput(nr), pathDone, nil
		case StatusFailed:
			return nil, pathFailed, errors.New(nr.GetString("error"))
		case StatusCancelled:
			return nil, pathFailed, context.Canceled
		case StatusWaiting:
			if !hasBody(n.Type) {
				return st.resume(ctx, n, nr, idx, item)
			}
		}
		// running (interrupted by a crash) or a foreach/loop in progress: execute again.
	}
	if nr == nil {
		var err error
		nr, err = st.newNodeRun(n, idx)
		if err != nil {
			return nil, pathFailed, err
		}
	}

	attempts := n.Retries + 1
	var out any
	var state pathState
	var err error
	for a := 0; a < attempts; a++ {
		if a > 0 {
			select {
			case <-ctx.Done():
				return nil, pathFailed, ctx.Err()
			case <-time.After(time.Duration(a) * 2 * time.Second):
			}
		}
		nr.Set("attempts", nr.GetInt("attempts")+1)
		out, state, err = st.execute(ctx, n, nr, idx, item)
		if state != pathFailed || ctx.Err() != nil {
			break
		}
	}
	return st.settle(n, nr, out, state, err)
}

// settle persists a node's outcome; on_error=continue turns a failure into
// an {error} output.
func (st *runState) settle(n *Node, nr *core.Record, out any, state pathState, err error) (any, pathState, error) {
	switch state {
	case pathDone:
		nr.Set("status", StatusSucceeded)
		nr.Set("output", out)
		nr.Set("error", "")
		nr.Set("finished", types.NowDateTime())
	case pathWaiting:
		nr.Set("status", StatusWaiting)
	case pathFailed:
		if errors.Is(err, context.Canceled) {
			nr.Set("status", StatusCancelled)
			break
		}
		nr.Set("error", err.Error())
		nr.Set("finished", types.NowDateTime())
		if n.OnError == "continue" {
			out = map[string]any{"error": err.Error()}
			nr.Set("status", StatusSucceeded)
			nr.Set("output", out)
			state, err = pathDone, nil
		} else {
			nr.Set("status", StatusFailed)
		}
	}
	if saveErr := st.e.App.Save(nr); saveErr != nil {
		return nil, pathFailed, saveErr
	}
	st.put(nr)
	return out, state, err
}

func (st *runState) newNodeRun(n *Node, idx int) (*core.Record, error) {
	col, err := st.e.App.FindCachedCollectionByNameOrId("flow_node_runs")
	if err != nil {
		return nil, err
	}
	nr := core.NewRecord(col)
	nr.Set("workflow_run", st.run.Id)
	nr.Set("node_id", n.ID)
	nr.Set("node_type", n.Type)
	nr.Set("item_index", idx)
	nr.Set("status", StatusRunning)
	nr.Set("started", types.NowDateTime())
	if err := st.e.App.Save(nr); err != nil {
		return nil, err
	}
	st.put(nr)
	return nr, nil
}

func decodeOutput(nr *core.Record) any {
	var out any
	_ = nr.UnmarshalJSONField("output", &out)
	return out
}

func decodeInput(nr *core.Record) map[string]any {
	var in map[string]any
	_ = nr.UnmarshalJSONField("input", &in)
	return in
}

// --- node execution ---

func (st *runState) execute(ctx context.Context, n *Node, nr *core.Record, idx int, item any) (any, pathState, error) {
	env := st.env(idx, item)
	switch n.Type {
	case NodeStart:
		return env["input"], pathDone, nil

	case NodeTransform:
		out, err := Render(n.Value, env)
		return out, stateOf(err), err

	case NodeEnd:
		if n.Output == nil {
			return nil, pathDone, nil
		}
		out, err := Render(n.Output, env)
		return out, stateOf(err), err

	case NodeNotify:
		msg, err := Render(n.Message, env)
		if err != nil {
			return nil, pathFailed, err
		}
		text := stringify(msg)
		if st.e.Notify != nil {
			st.e.Notify(st.run, text)
		} else {
			st.e.App.Logger().Info("workflow notify", "run", st.run.Id, "node", n.ID, "message", text)
		}
		return map[string]any{"message": text}, pathDone, nil

	case NodeBranch:
		for i, c := range n.Cases {
			ok, err := EvalBool(c.If, env)
			if err != nil {
				return nil, pathFailed, fmt.Errorf("case %d: %w", i, err)
			}
			if ok {
				return map[string]any{"_next": c.To, "case": i}, pathDone, nil
			}
		}
		return map[string]any{"_next": n.Default, "case": -1}, pathDone, nil

	case NodeTool:
		return st.execTool(ctx, n, nr, env)
	case NodeAgent:
		return st.execAgent(ctx, n, nr, env)
	case NodeForEach:
		return st.execForEach(ctx, n, nr, env)
	case NodeLoop:
		return st.execLoop(ctx, n, nr)
	case NodeReview:
		return st.execReview(n, nr, env)
	case NodeWait:
		return st.execWait(n, nr, env)
	case NodeSubworkflow:
		return st.execSubworkflow(n, nr, env)
	}
	return nil, pathFailed, fmt.Errorf("unsupported node type %q", n.Type)
}

func stateOf(err error) pathState {
	if err != nil {
		return pathFailed
	}
	return pathDone
}

func (st *runState) execTool(ctx context.Context, n *Node, nr *core.Record, env map[string]any) (any, pathState, error) {
	t, ok := st.e.Tools.Get(n.Tool)
	if !ok {
		return nil, pathFailed, fmt.Errorf("tool %q is not available (plugin disabled?)", n.Tool)
	}
	input, err := RenderMap(n.Input, env)
	if err != nil {
		return nil, pathFailed, err
	}
	nr.Set("input", input)
	if t.RequiresApproval() || filledAtRuntime(t, n.Input, input) {
		if err := st.createApproval(nr, "tool", "Run "+n.Tool+"?", "", map[string]any{"tool": n.Tool, "input": input}); err != nil {
			return nil, pathFailed, err
		}
		return nil, pathWaiting, nil
	}
	return st.invokeTool(ctx, t, n, input)
}

// filledAtRuntime reports whether a field that needs a person's writing comes from
// run data. Only text spelled out in the workflow itself counts as written by a person.
func filledAtRuntime(t tools.Tool, raw, rendered map[string]any) bool {
	field := tools.HumanWrittenField(t, rendered)
	if field == "" {
		return false
	}
	text, ok := raw[field].(string)
	return !ok || strings.Contains(text, "{{")
}

func (st *runState) invokeTool(ctx context.Context, t tools.Tool, n *Node, input map[string]any) (any, pathState, error) {
	tctx, cancel := context.WithTimeout(ctx, toolNodeTimeout)
	defer cancel()
	out, err := tools.Invoke(tctx, t, tools.Call{Args: input, Config: st.def.ToolConfig[n.Tool], RunID: st.run.Id})
	return out, stateOf(err), err
}

func (st *runState) execAgent(ctx context.Context, n *Node, nr *core.Record, env map[string]any) (any, pathState, error) {
	spec, err := st.agentSpec(n)
	if err != nil {
		return nil, pathFailed, err
	}
	input, err := RenderMap(n.Input, env)
	if err != nil {
		return nil, pathFailed, err
	}
	nr.Set("input", input)
	if err := st.checkDailyBudget(); err != nil {
		return nil, pathFailed, err
	}
	res, err := st.e.Runner.Run(ctx, spec, agent.Options{
		Input:      input,
		WorkflowAI: st.def.AI,
		NodeRunID:  nr.Id,
		ToolConfig: st.def.ToolConfig,
	})
	if err != nil {
		return nil, pathFailed, err
	}
	nr.Set("cost_usd", res.CostUSD)
	if res.Status != "succeeded" {
		return nil, pathFailed, fmt.Errorf("agent %s: %s", res.Status, res.Error)
	}
	return res.Output, pathDone, nil
}

// agentSpec builds the agent from a preset (if any) overlaid by node fields.
func (st *runState) agentSpec(n *Node) (agent.Spec, error) {
	a := n.Agent
	if a == nil {
		return agent.Spec{}, errors.New("agent settings missing")
	}
	var spec agent.Spec
	if a.Preset != "" {
		p, ok := st.e.catalog().Preset(a.Preset)
		if !ok {
			return spec, fmt.Errorf("agent preset %q is not available", a.Preset)
		}
		raw, _ := json.Marshal(p)
		if err := json.Unmarshal(raw, &spec); err != nil {
			return spec, fmt.Errorf("preset %s: %w", a.Preset, err)
		}
	}
	spec.Name = n.ID
	if a.SystemPrompt != "" {
		spec.SystemPrompt = a.SystemPrompt
	}
	if len(a.Tools) > 0 {
		spec.Tools = a.Tools
	}
	if a.OutputSchema != nil {
		spec.OutputSchema = a.OutputSchema
	}
	if a.MaxSteps > 0 {
		spec.MaxSteps = a.MaxSteps
	}
	if a.AI != nil {
		merged := map[string]any{}
		for k, v := range spec.AI {
			merged[k] = v
		}
		for k, v := range a.AI {
			merged[k] = v
		}
		spec.AI = merged
	}
	return spec, nil
}

// checkDailyBudget enforces ai.budget.max_usd_per_day for this workflow.
func (st *runState) checkDailyBudget() error {
	budget, _ := st.def.AI["budget"].(map[string]any)
	limit, _ := budget["max_usd_per_day"].(float64)
	if limit <= 0 {
		return nil
	}
	today := time.Now().UTC().Format("2006-01-02") + " 00:00:00.000Z"
	var spent float64
	err := st.e.App.DB().NewQuery(`SELECT COALESCE(SUM(a.cost_usd),0) FROM flow_agent_runs a
		JOIN flow_node_runs n ON a.node_run = n.id JOIN flow_runs r ON n.workflow_run = r.id
		WHERE r.workflow = {:w} AND a.created >= {:today}`).
		Bind(dbx.Params{"w": st.run.GetString("workflow"), "today": today}).Row(&spent)
	if err != nil {
		return err
	}
	if spent >= limit {
		return fmt.Errorf("workflow daily budget reached: $%.2f of $%.2f", spent, limit)
	}
	return nil
}

func (st *runState) execForEach(ctx context.Context, n *Node, nr *core.Record, env map[string]any) (any, pathState, error) {
	// Items are evaluated once and stored, so resumes see the same list.
	in := decodeInput(nr)
	items, ok := in["items"].([]any)
	if !ok {
		v, err := Eval(n.Items, env)
		if err != nil {
			return nil, pathFailed, err
		}
		list, isList := v.([]any)
		if v != nil && !isList {
			return nil, pathFailed, fmt.Errorf("items must be a list, got %T", v)
		}
		items = list
		nr.Set("input", map[string]any{"items": items})
		if err := st.e.App.Save(nr); err != nil {
			return nil, pathFailed, err
		}
	}

	conc := n.Concurrency
	if conc <= 0 {
		conc = defaultConcurrency
	}
	results := make([]pathResult, len(items))
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = st.walk(ctx, n.Body, i, items[i])
		}(i)
	}
	wg.Wait()

	outs := make([]any, len(items))
	waiting, failed := 0, 0
	var firstErr error
	for i, r := range results {
		switch r.state {
		case pathDone:
			outs[i] = r.out
		case pathWaiting:
			waiting++
		case pathFailed:
			failed++
			if firstErr == nil {
				firstErr = fmt.Errorf("item %d: %w", i, r.err)
			}
			outs[i] = map[string]any{"error": r.err.Error()}
		}
	}
	if failed > 0 && n.OnError != "continue" {
		return nil, pathFailed, firstErr
	}
	if waiting > 0 {
		return nil, pathWaiting, nil
	}
	return map[string]any{"count": len(items), "failed": failed, "results": outs}, pathDone, nil
}

// loopItem is what a Loop body sees: the iteration number and the previous
// iteration's output (nil on the first), as loop.iteration and loop.last.
type loopItem struct {
	iteration int
	last      any
}

// execLoop runs the body one iteration at a time; iteration i's nodes use
// item index i. After each iteration until is evaluated with loop.last =
// that iteration's output and nodes = its body outputs.
func (st *runState) execLoop(ctx context.Context, n *Node, nr *core.Record) (any, pathState, error) {
	// Iterations whose until was false are stored, so resumes don't
	// re-evaluate them.
	results, _ := decodeInput(nr)["results"].([]any)
	if results == nil {
		results = []any{}
	}
	var last any
	if len(results) > 0 {
		last = results[len(results)-1]
	}
	output := func(done bool) map[string]any {
		return map[string]any{"iterations": len(results), "done": done, "last": last, "results": results}
	}
	for i := len(results); i < n.MaxIterations; i++ {
		r := st.walk(ctx, n.Body, i, loopItem{iteration: i, last: last})
		switch r.state {
		case pathWaiting:
			return nil, pathWaiting, nil
		case pathFailed:
			return nil, pathFailed, fmt.Errorf("iteration %d: %w", i, r.err)
		}
		last = r.out
		results = append(results, last)
		stop, err := EvalBool(n.Until, st.env(i, loopItem{iteration: i, last: last}))
		if err != nil {
			return nil, pathFailed, fmt.Errorf("until: %w", err)
		}
		if stop {
			return output(true), pathDone, nil
		}
		nr.Set("input", map[string]any{"results": results})
		if err := st.e.App.Save(nr); err != nil {
			return nil, pathFailed, err
		}
	}
	// Out of iterations: on_error=continue keeps the results.
	if n.OnError == "continue" {
		return output(false), pathDone, nil
	}
	return nil, pathFailed, fmt.Errorf("until was still false after %d iterations", n.MaxIterations)
}

func (st *runState) execReview(n *Node, nr *core.Record, env map[string]any) (any, pathState, error) {
	r := n.Review
	data, err := Render(r.Data, env)
	if err != nil {
		return nil, pathFailed, fmt.Errorf("review.data: %w", err)
	}
	if r.AutoApprove != "" {
		ok, err := EvalBool(r.AutoApprove, env)
		if err != nil {
			return nil, pathFailed, fmt.Errorf("review.auto_approve: %w", err)
		}
		if ok {
			return map[string]any{"approved": true, "data": data, "auto": true}, pathDone, nil
		}
	}
	title, err := Render(r.Title, env)
	if err != nil {
		return nil, pathFailed, err
	}
	instructions, err := Render(r.Instructions, env)
	if err != nil {
		return nil, pathFailed, err
	}
	nr.Set("input", map[string]any{"data": data})
	if d, _ := parseDuration(r.Timeout); d > 0 {
		nr.Set("wake_at", time.Now().Add(d).UTC())
	}
	if err := st.createApproval(nr, "review", stringify(title), stringify(instructions), data); err != nil {
		return nil, pathFailed, err
	}
	return nil, pathWaiting, nil
}

func (st *runState) execWait(n *Node, nr *core.Record, env map[string]any) (any, pathState, error) {
	w := n.Wait
	var wake time.Time
	if d, _ := parseDuration(w.Delay); d > 0 {
		wake = time.Now().Add(d)
	}
	if d, _ := parseDuration(w.Timeout); d > 0 && w.Event != "" {
		t := time.Now().Add(d)
		if wake.IsZero() || t.Before(wake) {
			wake = t
		}
	}
	if !wake.IsZero() {
		nr.Set("wake_at", wake.UTC())
	}
	if w.Event != "" {
		nr.Set("waiting_for", w.Event)
		// Snapshot what match expressions may reference.
		nr.Set("input", map[string]any{"match_env": map[string]any{"input": env["input"], "nodes": env["nodes"], "item": env["item"]}})
	}
	return nil, pathWaiting, nil
}

func (st *runState) execSubworkflow(n *Node, nr *core.Record, env map[string]any) (any, pathState, error) {
	wf, err := st.e.App.FindFirstRecordByData("flows", "name", n.Workflow)
	if err != nil {
		return nil, pathFailed, fmt.Errorf("workflow %q not found", n.Workflow)
	}
	input, err := RenderMap(n.Input, env)
	if err != nil {
		return nil, pathFailed, err
	}
	// Keys that render to nil are left out so the child's default_input applies.
	for k, v := range input {
		if v == nil {
			delete(input, k)
		}
	}
	child, err := st.e.StartRun(wf, input, StartOptions{Source: "subworkflow", ParentRun: st.run.Id})
	if err != nil {
		return nil, pathFailed, err
	}
	nr.Set("input", input)
	if n.Detach {
		return map[string]any{"run_id": child.Id}, pathDone, nil
	}
	nr.Set("child_run", child.Id)
	return nil, pathWaiting, nil
}

// resume re-checks a waiting node: decided approvals, elapsed delays,
// timeouts. Events and finished subworkflows resolve nodes directly.
func (st *runState) resume(ctx context.Context, n *Node, nr *core.Record, idx int, item any) (any, pathState, error) {
	now := time.Now()
	wake := nr.GetDateTime("wake_at").Time()
	due := !wake.IsZero() && !now.Before(wake)

	switch n.Type {
	case NodeReview, NodeTool:
		appr, err := st.e.App.FindFirstRecordByFilter("flow_approvals", "node_run = {:n}", dbx.Params{"n": nr.Id})
		if err != nil {
			return nil, pathFailed, errors.New("approval record missing")
		}
		status := appr.GetString("status")
		if status == "pending" && due {
			appr.Set("status", "expired")
			appr.Set("decided_at", types.NowDateTime())
			if err := st.e.App.Save(appr); err != nil {
				return nil, pathFailed, err
			}
			status = "expired"
		}
		if status == "pending" {
			return nil, pathWaiting, nil
		}
		approved := status == "approved"
		var payload any
		_ = appr.UnmarshalJSONField("payload", &payload)
		var edited any
		_ = appr.UnmarshalJSONField("edited_payload", &edited)
		if edited != nil {
			payload = edited
		}
		meta := map[string]any{"approved": approved, "status": status, "decided_by": appr.GetString("decided_by"), "channel": appr.GetString("channel")}

		if n.Type == NodeReview {
			meta["data"] = payload
			return st.settle(n, nr, meta, pathDone, nil)
		}
		if !approved {
			meta["_denied"] = true
			return st.settle(n, nr, meta, pathDone, nil)
		}
		p, _ := payload.(map[string]any)
		input, _ := p["input"].(map[string]any)
		t, ok := st.e.Tools.Get(n.Tool)
		if !ok {
			return st.settle(n, nr, nil, pathFailed, fmt.Errorf("tool %q is not available", n.Tool))
		}
		out, state, err := st.invokeTool(ctx, t, n, input)
		return st.settle(n, nr, out, state, err)

	case NodeWait:
		if n.Wait.Event == "" && due {
			return st.settle(n, nr, map[string]any{"waited": n.Wait.Delay}, pathDone, nil)
		}
		if n.Wait.Event != "" && due {
			nr.Set("waiting_for", "")
			return st.settle(n, nr, map[string]any{"timed_out": true}, pathDone, nil)
		}
		return nil, pathWaiting, nil

	case NodeSubworkflow:
		child, err := st.e.App.FindRecordById("flow_runs", nr.GetString("child_run"))
		if err != nil {
			return st.settle(n, nr, nil, pathFailed, errors.New("subworkflow run missing"))
		}
		switch child.GetString("status") {
		case StatusSucceeded:
			var out any
			_ = child.UnmarshalJSONField("output", &out)
			return st.settle(n, nr, out, pathDone, nil)
		case StatusFailed, StatusCancelled:
			return st.settle(n, nr, nil, pathFailed, fmt.Errorf("subworkflow %s: %s", child.GetString("status"), child.GetString("error")))
		}
		return nil, pathWaiting, nil
	}
	return nil, pathWaiting, nil
}

func (st *runState) createApproval(nr *core.Record, kind, title, instructions string, payload any) error {
	col, err := st.e.App.FindCachedCollectionByNameOrId("flow_approvals")
	if err != nil {
		return err
	}
	a := core.NewRecord(col)
	a.Set("kind", kind)
	a.Set("node_run", nr.Id)
	a.Set("title", strings.TrimSpace(title))
	a.Set("instructions", instructions)
	a.Set("payload", payload)
	a.Set("status", "pending")
	return st.e.App.Save(a)
}

// resolveSubworkflow wakes the parent run when a child finishes.
func (e *Engine) resolveSubworkflow(parentID string, child *core.Record) {
	if _, err := e.App.FindFirstRecordByFilter("flow_node_runs", "workflow_run = {:p} && child_run = {:c}", dbx.Params{"p": parentID, "c": child.Id}); err != nil {
		return
	}
	if err := e.Queue.Enqueue(JobAdvance, parentID, nil, time.Time{}); err != nil {
		e.App.Logger().Error("waking parent run failed", "parent", parentID, "error", err)
	}
}
