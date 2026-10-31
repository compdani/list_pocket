package flow_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/compdani/list_pocket/internal/migrations"

	"github.com/compdani/list_pocket/internal/flow"
	"github.com/compdani/list_pocket/internal/flow/agent"
	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/list_pocket/internal/flow/queue"
	"github.com/compdani/list_pocket/internal/flow/tools"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

var admin = flow.Actor{}
var approver = flow.Actor{}

// recordTool records calls; optionally requires approval.
type recordTool struct {
	name     string
	approval bool
	mu       sync.Mutex
	calls    []map[string]any
}

func (t *recordTool) Name() string        { return t.name }
func (t *recordTool) Description() string { return "Records its input." }
func (t *recordTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *recordTool) RequiresApproval() bool { return t.approval }
func (t *recordTool) Invoke(_ context.Context, c tools.Call) (any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls = append(t.calls, c.Args)
	return map[string]any{"ok": true, "n": len(t.calls), "config": c.Config}, nil
}
func (t *recordTool) count() int { t.mu.Lock(); defer t.mu.Unlock(); return len(t.calls) }

type fakePlugins struct{}

func (f fakePlugins) Preset(name string) (map[string]any, bool) {
	if name == "test.finder" {
		return map[string]any{"name": "finder", "system_prompt": "Find leads.", "output_schema": map[string]any{"type": "object"}}, true
	}
	return nil, false
}

type env struct {
	app    *tests.TestApp
	reg    *tools.Registry
	runner *agent.Runner
	engine *flow.Engine
	queue  *queue.Queue
	save   *recordTool
	gated  *recordTool
	llm    *httptest.Server
	llmN   int
}

// newEngine builds a fresh engine on the same app — what a restart does.
func (en *env) newEngine() {
	q := queue.New(en.app)
	q.Poll = 10 * time.Millisecond
	en.queue = q
	en.engine = flow.NewEngine(en.app, en.runner, fakePlugins{}, q)
}

func setup(t *testing.T, llmReplies ...string) *env {
	t.Helper()
	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{DataDir: t.TempDir(), EncryptionEnv: "pb_test_env"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if err := app.RunAllMigrations(); err != nil {
		t.Fatal(err)
	}
	en := &env{app: app, reg: tools.NewRegistry(), save: &recordTool{name: "test.save"}, gated: &recordTool{name: "test.gated", approval: true}}
	en.reg.Register(en.save)
	en.reg.Register(en.gated)

	var mu sync.Mutex
	en.llm = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		i := en.llmN
		en.llmN++
		mu.Unlock()
		reply := `{"choices":[{"delta":{"content":"ok"}}]}`
		if i < len(llmReplies) {
			reply = llmReplies[i]
		}
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", reply)
	}))
	t.Cleanup(en.llm.Close)
	en.runner = &agent.Runner{App: app, LLM: llm.NewClient(), Tools: en.reg, Resolver: &llm.Resolver{Getenv: func(k string) string {
		return map[string]string{"LISTPOCKET_AI_API_KEY": "k", "LISTPOCKET_AI_BASE_URL": en.llm.URL}[k]
	}}}
	en.newEngine()
	en.engine.RegisterHooks()
	return en
}

func (en *env) publish(t *testing.T, def string) *core.Record {
	t.Helper()
	var d flow.Definition
	if err := json.Unmarshal([]byte(def), &d); err != nil {
		t.Fatal(err)
	}
	wf, err := en.engine.Import(&d, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, v, err := en.engine.Publish(wf, admin, ""); err != nil {
		t.Fatalf("publish: %v %+v", err, v)
	}
	wf, _ = en.engine.FindWorkflow(d.Name)
	return wf
}

func (en *env) drain() { en.queue.Drain(context.Background()) }

func (en *env) run(t *testing.T, id string) *core.Record {
	t.Helper()
	r, err := en.app.FindRecordById("flow_runs", id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (en *env) nodes(t *testing.T, runID string) map[string]*core.Record {
	recs, _ := en.app.FindRecordsByFilter("flow_node_runs", "workflow_run = {:r}", "", 0, 0, dbx.Params{"r": runID})
	out := map[string]*core.Record{}
	for _, r := range recs {
		key := r.GetString("node_id")
		if i := r.GetInt("item_index"); i >= 0 {
			key = fmt.Sprintf("%s[%d]", key, i)
		}
		out[key] = r
	}
	return out
}

func (en *env) pendingApprovals(t *testing.T) []*core.Record {
	recs, _ := en.app.FindRecordsByFilter("flow_approvals", "status = 'pending'", "created", 0, 0)
	return recs
}

func submitReply(v any) string {
	args, _ := json.Marshal(v)
	q, _ := json.Marshal(string(args))
	return `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"s","type":"function","function":{"name":"submit_result","arguments":` + string(q) + `}}]}}]}`
}

// The Phase 3 acceptance workflow: agent → foreach → branch → review/tool,
// paused for review, resumed by an approval after a "restart".
const leadsWorkflow = `{
  "name": "lead_pipeline",
  "trigger": {"type": "manual"},
  "input_schema": {"type": "object", "properties": {"market": {"type": "string"}}, "required": ["market"]},
  "tool_config": {"test.save": {"campaign": "c-1"}},
  "nodes": [
    {"id": "start", "type": "start", "next": "find"},
    {"id": "find", "type": "agent", "next": "each",
     "agent": {"preset": "test.finder", "output_schema": {"type": "object", "properties": {"leads": {"type": "array"}}, "required": ["leads"]}},
     "input": {"market": "{{ input.market }}"}},
    {"id": "each", "type": "foreach", "items": "nodes.find.result.leads", "body": "route", "concurrency": 2, "next": "done"},
    {"id": "route", "type": "branch", "cases": [
       {"if": "item.tier == 'A'", "to": "review"},
       {"if": "item.tier == 'B'", "to": "save"}
    ]},
    {"id": "review", "type": "review", "next": "save",
     "review": {"title": "Approve {{ item.name }}?", "data": {"name": "{{ item.name }}", "note": "tier {{ item.tier }}"}}},
    {"id": "save", "type": "tool", "tool": "test.save", "input": {"name": "{{ item.name }}", "reviewed": "{{ nodes.review != nil }}"}},
    {"id": "done", "type": "end", "output": {"count": "{{ nodes.each.count }}", "market": "{{ input.market }}"}}
  ]
}`

func TestContactsInActiveRuns(t *testing.T) {
	en := setup(t)
	wf := en.publish(t, `{"name":"enrolled","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"pause"},
		{"id":"pause","type":"wait","wait":{"delay":"1h"}}]}`)
	other := en.publish(t, `{"name":"other","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"pause"},
		{"id":"pause","type":"wait","wait":{"delay":"1h"}}]}`)

	start := func(w *core.Record, contact string) *core.Record {
		t.Helper()
		run, err := en.engine.StartRun(w, map[string]any{"contact": contact}, flow.StartOptions{Source: "manual_ui"})
		if err != nil {
			t.Fatal(err)
		}
		return run
	}
	start(wf, "waiting")
	cancelled := start(wf, "cancelled")
	start(other, "elsewhere")
	if _, err := en.engine.StartRun(wf, map[string]any{}, flow.StartOptions{Source: "manual_ui"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := en.engine.StartTestRun(wf, map[string]any{"contact": "tester"}, ""); err != nil {
		t.Fatal(err)
	}
	en.drain()
	if err := en.engine.Cancel(cancelled.Id); err != nil {
		t.Fatal(err)
	}

	got, err := en.engine.ContactsInActiveRuns(wf)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got["waiting"] {
		t.Fatalf("active contacts: %v", got)
	}
}

func TestLeadPipelinePausesForReviewAndResumesAfterRestart(t *testing.T) {
	en := setup(t, submitReply(map[string]any{"leads": []any{
		map[string]any{"name": "Alpha", "tier": "A"},
		map[string]any{"name": "Bravo", "tier": "B"},
		map[string]any{"name": "Charlie", "tier": "C"},
		map[string]any{"name": "Delta", "tier": "A"},
	}}))
	wf := en.publish(t, leadsWorkflow)

	if _, err := en.engine.StartRun(wf, map[string]any{}, flow.StartOptions{Source: "manual_ui"}); err == nil {
		t.Fatal("missing required input should be rejected")
	}
	run, err := en.engine.StartRun(wf, map[string]any{"market": "Lake County"}, flow.StartOptions{Source: "manual_ui"})
	if err != nil {
		t.Fatal(err)
	}
	en.drain()

	r := en.run(t, run.Id)
	if r.GetString("status") != flow.StatusWaiting {
		t.Fatalf("status = %s err=%s", r.GetString("status"), r.GetString("error"))
	}
	n := en.nodes(t, run.Id)
	if n["save[1]"] == nil || n["save[1]"].GetString("status") != "succeeded" {
		t.Errorf("B lead should be saved without review: %v", n["save[1]"])
	}
	if n["route[2]"] == nil || n["save[2]"] != nil || n["review[2]"] != nil {
		t.Error("C lead should stop after routing")
	}
	if en.save.count() != 1 {
		t.Fatalf("saves before review = %d, want 1", en.save.count())
	}
	if !strings.Contains(fmt.Sprint(en.save.calls[0]), "Bravo") {
		t.Errorf("save input = %v", en.save.calls[0])
	}
	pending := en.pendingApprovals(t)
	if len(pending) != 2 || !strings.HasPrefix(pending[0].GetString("title"), "Approve ") {
		t.Fatalf("pending approvals = %d", len(pending))
	}

	// "Restart": brand-new engine and queue on the same database.
	en.newEngine()
	if err := en.engine.Recover(); err != nil {
		t.Fatal(err)
	}
	en.drain()
	if en.run(t, run.Id).GetString("status") != flow.StatusWaiting {
		t.Fatal("run should still be waiting after restart")
	}
	if en.llmN != 1 {
		t.Errorf("agent re-ran after restart: %d LLM calls", en.llmN)
	}

	// Approve one (with an edit), deny the other.
	var alpha, delta *core.Record
	for _, a := range pending {
		if strings.Contains(a.GetString("title"), "Alpha") {
			alpha = a
		} else {
			delta = a
		}
	}
	if err := en.engine.Decide(alpha.Id, true, map[string]any{"name": "Alpha (edited)"}, approver); err != nil {
		t.Fatal(err)
	}
	en.drain()
	if en.run(t, run.Id).GetString("status") != flow.StatusWaiting {
		t.Fatal("run should wait for the second review")
	}
	if err := en.engine.Decide(delta.Id, false, nil, approver); err != nil {
		t.Fatal(err)
	}
	if err := en.engine.Decide(delta.Id, true, nil, approver); err == nil {
		t.Error("deciding twice must fail")
	}
	en.drain()

	r = en.run(t, run.Id)
	if r.GetString("status") != flow.StatusSucceeded {
		t.Fatalf("final status = %s err=%s", r.GetString("status"), r.GetString("error"))
	}
	if out := r.GetString("output"); !strings.Contains(out, `"count":4`) || !strings.Contains(out, `"market":"Lake County"`) {
		t.Errorf("output = %s", out)
	}
	if en.save.count() != 2 {
		t.Fatalf("saves = %d, want 2 (B + approved A)", en.save.count())
	}
	last := fmt.Sprint(en.save.calls[1])
	if !strings.Contains(last, "Alpha") || !strings.Contains(last, "reviewed:true") {
		t.Errorf("approved save input = %v", last)
	}
	n = en.nodes(t, run.Id)
	if out := n["review[0]"].GetString("output"); !strings.Contains(out, "Alpha (edited)") {
		t.Errorf("review output should carry the edit: %s", out)
	}
	if n["save[3]"] != nil {
		t.Error("denied lead must not be saved")
	}
	if !strings.Contains(n["save[1]"].GetString("output"), "c-1") {
		t.Error("tool_config not passed to tool")
	}
	if c := r.GetFloat("cost_usd"); c < 0 {
		t.Errorf("cost = %v", c)
	}
}

func TestValidation(t *testing.T) {
	en := setup(t)
	check := func(def string, wants ...string) {
		t.Helper()
		var d flow.Definition
		if err := json.Unmarshal([]byte(def), &d); err != nil {
			t.Fatal(err)
		}
		v := en.engine.ValidateDefinition(&d)
		all := strings.Join(append(v.Errors, v.Warnings...), "\n")
		if len(wants) == 0 && !v.OK {
			t.Errorf("expected valid, got:\n%s", all)
		}
		for _, w := range wants {
			if !strings.Contains(all, w) {
				t.Errorf("missing %q in:\n%s", w, all)
			}
		}
	}
	check(leadsWorkflow)
	check(`{"name":"x1","trigger":{"type":"cron","spec":"bad"},"nodes":[]}`, "trigger.spec", "exactly one start")
	check(`{"name":"loop","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"a"},{"id":"a","type":"transform","next":"b"},{"id":"b","type":"transform","next":"a"}]}`, "cycle")
	check(`{"name":"refs","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"t"},
		{"id":"t","type":"tool","tool":"nope.tool","next":"g","input":{"x":"{{ input.( }}"}},
		{"id":"g","type":"agent","agent":{"preset":"nope.preset","tools":["web.search","missing.x"]}},
		{"id":"orphan","type":"transform"}]}`, "unknown tool \"nope.tool\"", "input:", "unknown agent preset", "unknown tool \"missing.x\"", "unreachable")
	check(`{"name":"nested","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"f1"},
		{"id":"f1","type":"foreach","items":"input.a","body":"f2"},
		{"id":"f2","type":"foreach","items":"item","body":"x"},
		{"id":"x","type":"transform"}]}`, "nested foreach")
	check(`{"name":"evt","trigger":{"type":"event","event":"other.thing"},"nodes":[{"id":"start","type":"start"}]}`, "unknown event")
	check(`{"name":"evt2","trigger":{"type":"event","collection":"users"},"nodes":[{"id":"start","type":"start"}]}`, "can't start a workflow")
	check(`{"name":"hook","trigger":{"type":"webhook","filter":"event.ok"},"nodes":[{"id":"start","type":"start"}]}`)
	check(`{"name":"loopy","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"l"},
		{"id":"l","type":"loop","body":"x","until":"loop.last.n >= 3","max_iterations":5,"next":"done"},
		{"id":"x","type":"tool","tool":"test.save"},{"id":"done","type":"end"}]}`)
	check(`{"name":"bad_loop","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"l"},
		{"id":"l","type":"loop","until":"loop.last.(","max_iterations":500}]}`, "until must be a valid expression", "body (first node", "max_iterations must be 1-100")
	check(`{"name":"nested_loop","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"f"},
		{"id":"f","type":"foreach","items":"input.a","body":"l"},
		{"id":"l","type":"loop","body":"x","until":"true","max_iterations":2},
		{"id":"x","type":"transform"}]}`, "nested loop")
}

func TestTemplates(t *testing.T) {
	env := map[string]any{"input": map[string]any{"n": 3.0, "name": "Ann"}, "nodes": map[string]any{"a": map[string]any{"list": []any{1.0, 2.0}}}}
	v, err := flow.Render(map[string]any{
		"typed":  "{{ input.n * 2 }}",
		"list":   "{{ nodes.a.list }}",
		"text":   "Hi {{ input.name }}, you have {{ input.n }} leads",
		"nested": []any{"{{ input.name }}", 5.0},
	}, env)
	if err != nil {
		t.Fatal(err)
	}
	m := v.(map[string]any)
	if m["typed"] != 6.0 || fmt.Sprint(m["list"]) != "[1 2]" || m["text"] != "Hi Ann, you have 3 leads" || fmt.Sprint(m["nested"]) != "[Ann 5]" {
		t.Errorf("render = %#v", m)
	}
	if ok, _ := flow.EvalBool("{{ input.missing == nil }}", env); !ok {
		t.Error("undefined variables should be nil")
	}
	if _, err := flow.Eval("input.name.(", env); err == nil {
		t.Error("syntax error expected")
	}
}

func TestCronTriggerAndOverlap(t *testing.T) {
	en := setup(t)
	wf := en.publish(t, `{"name":"nightly","trigger":{"type":"cron","spec":"*/5 * * * *","timezone":"America/Chicago","input":{"at":"{{ now }}"}},
		"nodes":[{"id":"start","type":"start","next":"hold"},
		{"id":"hold","type":"review","review":{"title":"hold"}}]}`)
	next := wf.GetDateTime("next_run_at").Time()
	if next.IsZero() || next.Minute()%5 != 0 || !next.After(time.Now()) {
		t.Fatalf("next_run_at = %v", next)
	}

	en.engine.Tick(time.Now()) // not due yet
	if n, _ := en.app.CountRecords("flow_runs"); n != 0 {
		t.Fatal("tick before next_run_at started a run")
	}
	en.engine.Tick(next.Add(time.Second))
	runs, _ := en.app.FindRecordsByFilter("flow_runs", "source = 'cron'", "", 0, 0)
	if len(runs) != 1 || runs[0].GetDateTime("scheduled_for").Time().Unix() != next.Unix() {
		t.Fatalf("cron runs = %d", len(runs))
	}
	if !strings.Contains(runs[0].GetString("input"), `"at":"20`) {
		t.Errorf("cron input template: %s", runs[0].GetString("input"))
	}
	en.drain() // run waits on its review

	wf, _ = en.engine.FindWorkflow("nightly")
	next2 := wf.GetDateTime("next_run_at").Time()
	if !next2.After(next) {
		t.Fatalf("next_run_at not advanced: %v", next2)
	}
	en.engine.Tick(next2.Add(time.Second))
	if n, _ := en.app.CountRecords("flow_runs"); n != 1 {
		t.Errorf("overlap=skip should skip while a run is active; runs = %d", n)
	}

	// NextRun respects the timezone.
	at := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC) // 06:00 Chicago
	got, err := flow.NextRun("30 7 * * *", "America/Chicago", at)
	if err != nil || got != time.Date(2026, 1, 15, 13, 30, 0, 0, time.UTC) {
		t.Errorf("NextRun = %v %v", got, err)
	}

	// Pausing clears the schedule.
	if err := en.engine.SetEnabled(wf, false); err != nil {
		t.Fatal(err)
	}
	if wf.GetDateTime("next_run_at").IsZero() == false {
		t.Error("disabled workflow should have no next_run_at")
	}
}

func pluginEvent(t *testing.T, en *env, name string, payload any) {
	t.Helper()
	en.engine.Emit(name, payload)
}

func TestEventTriggerAndWaitForEvent(t *testing.T) {
	en := setup(t)
	en.publish(t, `{"name":"on_reply","trigger":{"type":"event","event":"inbound.email","filter":"event.kind == 'reply'","input_map":{"from":"{{ event.from }}"}},
		"nodes":[{"id":"start","type":"start","next":"save"},{"id":"save","type":"tool","tool":"test.save","input":{"from":"{{ input.from }}"}}]}`)
	en.publish(t, `{"name":"waiter","trigger":{"type":"manual"},
		"nodes":[{"id":"start","type":"start","next":"w"},
		{"id":"w","type":"wait","next":"after","wait":{"event":"inbound.email","match":"event.from == input.who","timeout":"1h"}},
		{"id":"after","type":"transform","value":"got {{ nodes.w.event.from }}"}]}`)

	waiter, _ := en.engine.FindWorkflow("waiter")
	run, _ := en.engine.StartRun(waiter, map[string]any{"who": "ann@x.test"}, flow.StartOptions{Source: "manual_ui"})
	en.drain()
	if en.run(t, run.Id).GetString("status") != flow.StatusWaiting {
		t.Fatal("waiter should wait")
	}

	pluginEvent(t, en, "inbound.email", map[string]any{"kind": "bounce", "from": "bob@x.test"})
	en.drain()
	if en.save.count() != 0 {
		t.Error("filter should reject non-reply events")
	}
	if en.run(t, run.Id).GetString("status") != flow.StatusWaiting {
		t.Error("non-matching event should not resolve the wait")
	}

	pluginEvent(t, en, "inbound.email", map[string]any{"kind": "reply", "from": "ann@x.test"})
	en.drain()
	if en.save.count() != 1 || !strings.Contains(fmt.Sprint(en.save.calls[0]), "ann@x.test") {
		t.Errorf("event trigger: calls = %v", en.save.calls)
	}
	r := en.run(t, run.Id)
	if r.GetString("status") != flow.StatusSucceeded || !strings.Contains(r.GetString("output"), "got ann@x.test") {
		t.Errorf("waiter: %s %s %s", r.GetString("status"), r.GetString("output"), r.GetString("error"))
	}
}

func TestToolApprovalWaitDelaySubworkflowCancel(t *testing.T) {
	en := setup(t)
	en.publish(t, `{"name":"child","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"end"},{"id":"end","type":"end","output":{"doubled":"{{ input.n * 2 }}"}}]}`)
	wf := en.publish(t, `{"name":"parent","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"pause"},
		{"id":"pause","type":"wait","wait":{"delay":"20ms"},"next":"sub"},
		{"id":"sub","type":"subworkflow","workflow":"child","input":{"n":21},"next":"gate"},
		{"id":"gate","type":"tool","tool":"test.gated","input":{"value":"{{ nodes.sub.doubled }}"},"next":"end"},
		{"id":"end","type":"end","output":"{{ nodes.gate }}"}]}`)

	run, _ := en.engine.StartRun(wf, nil, flow.StartOptions{Source: "manual_ui"})
	en.drain()
	n := en.nodes(t, run.Id)
	if n["pause"].GetString("status") != "waiting" || n["pause"].GetDateTime("wake_at").IsZero() {
		t.Fatal("wait node should be waiting with wake_at")
	}
	time.Sleep(40 * time.Millisecond)
	en.drain() // wake job due → delay done → subworkflow → child → parent → gate waits

	n = en.nodes(t, run.Id)
	if !strings.Contains(n["sub"].GetString("output"), `"doubled":42`) {
		t.Fatalf("subworkflow output = %s (status %s)", n["sub"].GetString("output"), n["sub"].GetString("status"))
	}

	// A subworkflow input that renders to nil falls back to the child's default_input.
	en.publish(t, `{"name":"typed_child","trigger":{"type":"manual"},
		"input_schema":{"type":"object","properties":{"label":{"type":"string"}}},
		"default_input":{"label":"fallback"},
		"nodes":[{"id":"start","type":"start","next":"end"},{"id":"end","type":"end","output":{"label":"{{ input.label }}"}}]}`)
	nilParent := en.publish(t, `{"name":"nil_parent","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"sub"},
		{"id":"sub","type":"subworkflow","workflow":"typed_child","input":{"label":"{{ input.label ?? nil }}"},"next":"end"},
		{"id":"end","type":"end","output":"{{ nodes.sub }}"}]}`)
	nilRun, _ := en.engine.StartRun(nilParent, nil, flow.StartOptions{Source: "manual_ui"})
	en.drain()
	if r := en.run(t, nilRun.Id); r.GetString("status") != flow.StatusSucceeded || !strings.Contains(r.GetString("output"), `"label":"fallback"`) {
		t.Fatalf("nil subworkflow input: %s %s %s", r.GetString("status"), r.GetString("output"), r.GetString("error"))
	}
	pending := en.pendingApprovals(t)
	if len(pending) != 1 || pending[0].GetString("kind") != "tool" || en.gated.count() != 0 {
		t.Fatalf("tool approval: pending=%d calls=%d", len(pending), en.gated.count())
	}
	if err := en.engine.Decide(pending[0].Id, true, map[string]any{"tool": "test.gated", "input": map[string]any{"value": 99}}, approver); err != nil {
		t.Fatal(err)
	}
	en.drain()
	r := en.run(t, run.Id)
	if r.GetString("status") != flow.StatusSucceeded || en.gated.count() != 1 || fmt.Sprint(en.gated.calls[0]["value"]) != "99" {
		t.Fatalf("after tool approval: %s calls=%v err=%s", r.GetString("status"), en.gated.calls, r.GetString("error"))
	}

	// Cancel a waiting run: approvals expire, run is final.
	run2, _ := en.engine.StartRun(wf, nil, flow.StartOptions{Source: "manual_ui"})
	en.drain()
	if err := en.engine.Cancel(run2.Id); err != nil {
		t.Fatal(err)
	}
	en.drain()
	if s := en.run(t, run2.Id).GetString("status"); s != flow.StatusCancelled {
		t.Errorf("cancelled run status = %s", s)
	}
	if err := en.engine.Cancel(run2.Id); err == nil {
		t.Error("cancelling twice should fail")
	}
}

func TestCrashedNodeIsRetriedAndFailuresReported(t *testing.T) {
	en := setup(t)
	wf := en.publish(t, `{"name":"crashy","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"save"},
		{"id":"save","type":"tool","tool":"test.save","next":"bad"},
		{"id":"bad","type":"transform","value":"{{ 1 / input.zero.x }}","on_error":"continue","next":"worse"},
		{"id":"worse","type":"transform","value":"{{ input.zero.x.y }}"}]}`)

	run, _ := en.engine.StartRun(wf, map[string]any{}, flow.StartOptions{Source: "manual_ui"})
	// Simulate a crash mid-node: a node_run stuck in "running".
	col, _ := en.app.FindCollectionByNameOrId("flow_node_runs")
	stuck := core.NewRecord(col)
	stuck.Set("workflow_run", run.Id)
	stuck.Set("node_id", "save")
	stuck.Set("node_type", "tool")
	stuck.Set("item_index", -1)
	stuck.Set("status", "running")
	_ = en.app.Save(stuck)

	en.drain()
	r := en.run(t, run.Id)
	if en.save.count() != 1 {
		t.Errorf("crashed node should be re-executed once, calls = %d", en.save.count())
	}
	n := en.nodes(t, run.Id)
	if n["bad"].GetString("status") != "succeeded" || !strings.Contains(n["bad"].GetString("output"), "error") {
		t.Errorf("on_error=continue: %s %s", n["bad"].GetString("status"), n["bad"].GetString("output"))
	}
	if r.GetString("status") != flow.StatusFailed || !strings.Contains(r.GetString("error"), "node worse") {
		t.Errorf("run = %s %q", r.GetString("status"), r.GetString("error"))
	}
}

func TestDetachedSubworkflowDoesNotHoldParent(t *testing.T) {
	en := setup(t)
	en.publish(t, `{"name":"slow_child","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"pause"},
		{"id":"pause","type":"wait","wait":{"delay":"1h"},"next":"end"},{"id":"end","type":"end"}]}`)
	wf := en.publish(t, `{"name":"launcher","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"sub"},
		{"id":"sub","type":"subworkflow","workflow":"slow_child","detach":true,"next":"end"},
		{"id":"end","type":"end","output":"{{ nodes.sub.run_id }}"}]}`)
	run, _ := en.engine.StartRun(wf, nil, flow.StartOptions{Source: "manual_ui"})
	en.drain()
	r := en.run(t, run.Id)
	if r.GetString("status") != flow.StatusSucceeded {
		t.Fatalf("parent should finish without waiting: %s %s", r.GetString("status"), r.GetString("error"))
	}
	childID := strings.Trim(r.GetString("output"), `"`)
	if c := en.run(t, childID); c.GetString("status") != flow.StatusWaiting || c.GetString("parent_run") != run.Id {
		t.Errorf("child: %s parent=%s", c.GetString("status"), c.GetString("parent_run"))
	}
}

func TestLoopRunsUntilConditionOrLimit(t *testing.T) {
	en := setup(t)
	wf := en.publish(t, `{"name":"poller","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"poll"},
		{"id":"poll","type":"loop","body":"check","until":"nodes.check.n >= input.target","max_iterations":5,"next":"end"},
		{"id":"check","type":"tool","tool":"test.save","input":{"iteration":"{{ loop.iteration }}","prev":"{{ loop.last?.n }}"},"next":"tag"},
		{"id":"tag","type":"transform","value":{"n":"{{ nodes.check.n }}","at":"{{ index }}"}},
		{"id":"end","type":"end","output":"{{ nodes.poll }}"}]}`)

	run, _ := en.engine.StartRun(wf, map[string]any{"target": 3}, flow.StartOptions{Source: "manual_ui"})
	en.drain()
	r := en.run(t, run.Id)
	if r.GetString("status") != flow.StatusSucceeded {
		t.Fatalf("run = %s %s", r.GetString("status"), r.GetString("error"))
	}
	var out map[string]any
	_ = r.UnmarshalJSONField("output", &out)
	if out["iterations"] != 3.0 || out["done"] != true || fmt.Sprint(out["last"]) != "map[at:2 n:3]" || len(out["results"].([]any)) != 3 {
		t.Errorf("output = %v", out)
	}
	if en.save.count() != 3 || fmt.Sprint(en.save.calls[0]["iteration"]) != "0" || en.save.calls[0]["prev"] != nil || fmt.Sprint(en.save.calls[2]["prev"]) != "2" {
		t.Errorf("calls = %v", en.save.calls)
	}
	if n := en.nodes(t, run.Id); n["check[2]"] == nil || n["check[3]"] != nil {
		t.Errorf("node runs per iteration: %v", n)
	}

	// Out of iterations: fails, or continues with done=false.
	run2, _ := en.engine.StartRun(wf, map[string]any{"target": 100}, flow.StartOptions{Source: "manual_ui"})
	en.drain()
	if r := en.run(t, run2.Id); r.GetString("status") != flow.StatusFailed || !strings.Contains(r.GetString("error"), "after 5 iterations") {
		t.Errorf("limit: %s %q", r.GetString("status"), r.GetString("error"))
	}
	wf3 := en.publish(t, `{"name":"poller_soft","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"poll"},
		{"id":"poll","type":"loop","body":"check","until":"false","max_iterations":2,"on_error":"continue","next":"end"},
		{"id":"check","type":"transform","value":"{{ loop.iteration }}"},
		{"id":"end","type":"end","output":"{{ nodes.poll }}"}]}`)
	run3, _ := en.engine.StartRun(wf3, nil, flow.StartOptions{Source: "manual_ui"})
	en.drain()
	r3 := en.run(t, run3.Id)
	out = nil
	_ = r3.UnmarshalJSONField("output", &out)
	if r3.GetString("status") != flow.StatusSucceeded || out["done"] != false || out["iterations"] != 2.0 || out["last"] != 1.0 {
		t.Errorf("on_error=continue: %s %v", r3.GetString("status"), out)
	}
}

func TestLoopPausesForApprovalAndResumesAfterRestart(t *testing.T) {
	en := setup(t)
	wf := en.publish(t, `{"name":"retry_until_ok","trigger":{"type":"manual"},"nodes":[
		{"id":"start","type":"start","next":"try"},
		{"id":"try","type":"loop","body":"ask","until":"loop.last.approved","max_iterations":4,"next":"end"},
		{"id":"ask","type":"review","review":{"title":"Attempt {{ loop.iteration }}"},"on_error":"continue"},
		{"id":"end","type":"end","output":"{{ nodes.try.iterations }}"}]}`)
	run, _ := en.engine.StartRun(wf, nil, flow.StartOptions{Source: "manual_ui"})

	for i, approve := range []bool{false, false, true} {
		en.drain()
		pending := en.pendingApprovals(t)
		if len(pending) != 1 || pending[0].GetString("title") != fmt.Sprintf("Attempt %d", i) {
			t.Fatalf("iteration %d: pending = %v", i, pending)
		}
		if s := en.run(t, run.Id).GetString("status"); s != flow.StatusWaiting {
			t.Fatalf("iteration %d: run = %s", i, s)
		}
		en.newEngine() // restart between decisions
		if err := en.engine.Decide(pending[0].Id, approve, nil, approver); err != nil {
			t.Fatal(err)
		}
	}
	en.drain()
	r := en.run(t, run.Id)
	if r.GetString("status") != flow.StatusSucceeded || r.GetString("output") != "3" {
		t.Fatalf("run = %s output=%s err=%s", r.GetString("status"), r.GetString("output"), r.GetString("error"))
	}
}

// humanBodyTool needs a person to have written its body when format is html.
type humanBodyTool struct{ *recordTool }

func (t humanBodyTool) HumanWrittenField(args map[string]any) string {
	if args["format"] == "html" {
		return "body"
	}
	return ""
}

func TestHumanWrittenFieldPausesOnlyWhenFilledAtRuntime(t *testing.T) {
	en := setup(t)
	tool := humanBodyTool{&recordTool{name: "test.human"}}
	en.reg.Register(tool)
	workflow := func(name, input string) *core.Record {
		return en.publish(t, `{"name":"`+name+`","trigger":{"type":"manual"},"nodes":[
			{"id":"start","type":"start","next":"send"},
			{"id":"send","type":"tool","tool":"test.human","input":`+input+`,"next":"end"},
			{"id":"end","type":"end"}]}`)
	}
	start := func(wf *core.Record) *core.Record {
		run, err := en.engine.StartRun(wf, map[string]any{"text": "<b>from a model</b>"}, flow.StartOptions{Source: "manual_ui"})
		if err != nil {
			t.Fatal(err)
		}
		en.drain()
		return en.run(t, run.Id)
	}

	if r := start(workflow("fixed", `{"format":"html","body":"<p>written in the workflow</p>"}`)); r.GetString("status") != flow.StatusSucceeded || tool.count() != 1 {
		t.Fatalf("a body written in the workflow should send: %s, %d calls", r.GetString("status"), tool.count())
	}
	if r := start(workflow("plain", `{"format":"plain","body":"{{ input.text }}"}`)); r.GetString("status") != flow.StatusSucceeded || tool.count() != 2 {
		t.Fatalf("run data in a plain body should send: %s, %d calls", r.GetString("status"), tool.count())
	}

	run := start(workflow("filled", `{"format":"html","body":"<p>{{ input.text }}</p>"}`))
	if run.GetString("status") == flow.StatusSucceeded || tool.count() != 2 {
		t.Fatalf("run data in an html body should wait: %s, %d calls", run.GetString("status"), tool.count())
	}
	pending, err := en.app.FindRecordsByFilter("flow_approvals", "status = 'pending'", "", 0, 0)
	if err != nil || len(pending) != 1 {
		t.Fatalf("want one pending approval, got %d (%v)", len(pending), err)
	}
	if err := en.engine.Decide(pending[0].Id, true, nil, flow.Actor{ID: "tester"}); err != nil {
		t.Fatal(err)
	}
	en.drain()
	if r := en.run(t, run.Id); r.GetString("status") != flow.StatusSucceeded || tool.count() != 3 {
		t.Fatalf("approval should send it: %s, %d calls", r.GetString("status"), tool.count())
	}
}

// A preset's tools can reach a person's data as much as the node's own, so they get
// the same checks, unless the node replaces them.
func TestPresetToolsAreValidated(t *testing.T) {
	cat := fakePresetCatalog{presets: map[string]map[string]any{
		"p.ok":      {"system_prompt": "x", "tools": []any{"read"}},
		"p.writes":  {"system_prompt": "x", "tools": []string{"write"}},
		"p.missing": {"system_prompt": "x", "tools": []string{"gone"}},
	}}
	check := func(agent string) flow.Validation {
		var d flow.Definition
		if err := json.Unmarshal([]byte(`{"name":"wf","trigger":{"type":"manual"},"nodes":[{"id":"start","type":"start","next":"a"},{"id":"a","type":"agent","agent":`+agent+`}]}`), &d); err != nil {
			t.Fatal(err)
		}
		return flow.Validate(&d, cat)
	}
	if v := check(`{"preset":"p.ok"}`); !v.OK || len(v.Warnings) != 0 {
		t.Errorf("read-only preset: %+v", v)
	}
	if v := check(`{"preset":"p.writes"}`); !v.OK || len(v.Warnings) != 1 {
		t.Errorf("a preset that can write should warn: %+v", v)
	}
	if v := check(`{"preset":"p.missing"}`); v.OK {
		t.Errorf("a preset with an unknown tool should fail: %+v", v)
	}
	if v := check(`{"preset":"p.writes","tools":["read"]}`); !v.OK || len(v.Warnings) != 0 {
		t.Errorf("the node's tools replace the preset's: %+v", v)
	}
	if v := check(`{"preset":"p.gone"}`); v.OK {
		t.Errorf("an unknown preset should fail: %+v", v)
	}
}

type fakePresetCatalog struct{ presets map[string]map[string]any }

func (c fakePresetCatalog) HasTool(name string) bool { return name == "read" || name == "write" }
func (c fakePresetCatalog) Preset(name string) (map[string]any, bool) {
	p, ok := c.presets[name]
	return p, ok
}
func (c fakePresetCatalog) HasEvent(string) bool                { return false }
func (c fakePresetCatalog) HasWorkflow(string) bool             { return false }
func (c fakePresetCatalog) HasCollection(string) bool           { return false }
func (c fakePresetCatalog) ToolHasSideEffects(name string) bool { return name == "write" }

func TestRegisterEvent(t *testing.T) {
	if flow.RegisterEvent(flow.EventInboundEmail, "spoofed") {
		t.Fatal("a plugin must not replace a built-in event")
	}
	if !flow.RegisterEvent("regtest.happened", "It happened.") {
		t.Fatal("a new event should register")
	}
	if !flow.IsKnownEvent("regtest.happened") || flow.RegisterEvent("regtest.happened", "again") {
		t.Fatal("the event should be known, and only once")
	}
	en := setup(t)
	en.publish(t, `{"name":"on_plugin","trigger":{"type":"event","event":"regtest.happened"},"nodes":[{"id":"start","type":"start","next":"save"},{"id":"save","type":"tool","tool":"test.save","input":{"n":"{{ input.n }}"}}]}`)
	if started, _ := en.engine.Emit("regtest.happened", map[string]any{"n": 1}); started != 1 {
		t.Fatalf("started %d runs", started)
	}
}
