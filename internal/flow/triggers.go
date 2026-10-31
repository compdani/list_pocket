package flow

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/cron"
	"github.com/pocketbase/pocketbase/tools/types"
)

// triggerIndex caches which published, enabled workflows start on which
// events and record changes, so record hooks stay cheap.
type triggerIndex struct {
	mu          sync.RWMutex
	events      map[string][]indexed // event name
	collections map[string][]indexed
}

type indexed struct {
	workflowID string
	trigger    Trigger
}

// RefreshTriggers rebuilds the event/record trigger index.
func (e *Engine) RefreshTriggers() error {
	wfs, err := e.App.FindRecordsByFilter("flows", "enabled = true && trigger_type = 'event' && published_version > 0", "", 0, 0)
	if err != nil {
		return err
	}
	events := map[string][]indexed{}
	cols := map[string][]indexed{}
	for _, wf := range wfs {
		def, _, err := e.PublishedDefinition(wf)
		if err != nil {
			continue
		}
		t := def.Trigger
		if t.Event != "" {
			events[t.Event] = append(events[t.Event], indexed{wf.Id, t})
		}
		if t.Collection != "" {
			cols[t.Collection] = append(cols[t.Collection], indexed{wf.Id, t})
		}
	}
	e.triggers.mu.Lock()
	e.triggers.events, e.triggers.collections = events, cols
	e.triggers.mu.Unlock()
	return nil
}

// RegisterHooks binds record-change triggers.
func (e *Engine) RegisterHooks() {
	e.App.OnRecordAfterCreateSuccess().BindFunc(func(ev *core.RecordEvent) error {
		e.handleRecordChange(ev.Record, "create")
		return ev.Next()
	})
	e.App.OnRecordAfterUpdateSuccess().BindFunc(func(ev *core.RecordEvent) error {
		e.handleRecordChange(ev.Record, "update")
		return ev.Next()
	})
}

// Emit starts event-triggered workflows and resolves wait nodes for an event. It
// returns how many runs started and how many waits resolved.
func (e *Engine) Emit(name string, payload any) (started, resolved int) {
	e.triggers.mu.RLock()
	targets := append([]indexed(nil), e.triggers.events[name]...)
	e.triggers.mu.RUnlock()
	for _, t := range targets {
		if e.startFromTrigger(t, map[string]any{"event": payload}, payload, "event") {
			started++
		}
	}
	return started, e.resolveWaits(name, payload)
}

// StartFromWebhook starts a published webhook workflow with a request body. A false
// result with no error means its filter turned the request away.
func (e *Engine) StartFromWebhook(wf *core.Record, body any) (bool, error) {
	def, _, err := e.PublishedDefinition(wf)
	if err != nil {
		return false, err
	}
	if def.Trigger.Type != "webhook" || !wf.GetBool("enabled") {
		return false, ErrNotPublished
	}
	return e.startFromTrigger(indexed{wf.Id, def.Trigger}, map[string]any{"event": body}, body, "webhook"), nil
}

func (e *Engine) handleRecordChange(rec *core.Record, op string) {
	e.triggers.mu.RLock()
	targets := append([]indexed(nil), e.triggers.collections[rec.Collection().Name]...)
	e.triggers.mu.RUnlock()
	if len(targets) == 0 {
		return
	}
	raw, _ := json.Marshal(rec.PublicExport())
	var exported map[string]any
	_ = json.Unmarshal(raw, &exported)
	for _, t := range targets {
		if len(t.trigger.On) > 0 && !contains(t.trigger.On, op) {
			continue
		}
		e.startFromTrigger(t, map[string]any{"record": exported, "op": op}, map[string]any{"record": exported, "op": op}, "event")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// startFromTrigger applies the trigger's filter and input_map, then starts
// a run. defaultInput is used when input_map is empty.
func (e *Engine) startFromTrigger(t indexed, env map[string]any, defaultInput any, source string) bool {
	if t.trigger.Filter != "" {
		ok, err := EvalBool(t.trigger.Filter, env)
		if err != nil {
			e.App.Logger().Warn("trigger filter failed", "workflow", t.workflowID, "error", err)
			return false
		}
		if !ok {
			return false
		}
	}
	input, _ := defaultInput.(map[string]any)
	if t.trigger.InputMap != nil {
		var err error
		if input, err = RenderMap(t.trigger.InputMap, env); err != nil {
			e.App.Logger().Warn("trigger input_map failed", "workflow", t.workflowID, "error", err)
			return false
		}
	}
	wf, err := e.App.FindRecordById("flows", t.workflowID)
	if err != nil || !wf.GetBool("enabled") {
		return false
	}
	if _, err := e.StartRun(wf, input, StartOptions{Source: source}); err != nil {
		e.App.Logger().Warn("starting event-triggered run failed", "workflow", wf.GetString("name"), "error", err)
		return false
	}
	return true
}

// resolveWaits completes wait nodes waiting for this event whose match holds.
func (e *Engine) resolveWaits(name string, payload any) int {
	waiting, err := e.App.FindRecordsByFilter("flow_node_runs", "status = 'waiting' && waiting_for = {:e}", "created", 0, 0, dbx.Params{"e": name})
	if err != nil {
		return 0
	}
	n := 0
	for _, nr := range waiting {
		run, err := e.App.FindRecordById("flow_runs", nr.GetString("workflow_run"))
		if err != nil || final(run.GetString("status")) {
			continue
		}
		def, err := e.runDefinition(run)
		if err != nil {
			continue
		}
		node := def.Node(nr.GetString("node_id"))
		if node == nil || node.Wait == nil {
			continue
		}
		if node.Wait.Match != "" {
			env, _ := decodeInput(nr)["match_env"].(map[string]any)
			if env == nil {
				env = map[string]any{}
			}
			env["event"] = payload
			if ok, err := EvalBool(node.Wait.Match, env); err != nil || !ok {
				continue
			}
		}
		nr.Set("status", StatusSucceeded)
		nr.Set("output", map[string]any{"event": payload})
		nr.Set("waiting_for", "")
		nr.Set("finished", types.NowDateTime())
		if err := e.App.Save(nr); err != nil {
			continue
		}
		_ = e.Queue.Enqueue(JobAdvance, run.Id, nil, time.Time{})
		n++
	}
	return n
}

// --- cron ---

// NextRun returns the first minute strictly after `after` matching spec in tz.
func NextRun(spec, tz string, after time.Time) (time.Time, error) {
	sched, err := cron.NewSchedule(spec)
	if err != nil {
		return time.Time{}, err
	}
	loc := time.UTC
	if tz != "" {
		if loc, err = time.LoadLocation(tz); err != nil {
			return time.Time{}, err
		}
	}
	t := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if sched.IsDue(cron.NewMoment(t)) {
			return t.UTC(), nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron %q never fires", spec)
}

// scheduleNext sets next_run_at for a cron workflow (or clears it).
func (e *Engine) scheduleNext(wf *core.Record, def *Definition, after time.Time) error {
	t := def.Trigger
	if t.Type != "cron" || !wf.GetBool("enabled") {
		wf.Set("next_run_at", "")
		return nil
	}
	if t.StartAt != "" {
		if s, err := time.Parse(time.RFC3339, t.StartAt); err == nil && after.Before(s) {
			after = s.Add(-time.Minute)
		}
	}
	next, err := NextRun(t.Spec, t.Timezone, after)
	if err != nil {
		return err
	}
	if t.EndAt != "" {
		if end, err := time.Parse(time.RFC3339, t.EndAt); err == nil && next.After(end) {
			wf.Set("next_run_at", "")
			return nil
		}
	}
	wf.Set("next_run_at", next)
	return nil
}

// Tick starts due cron workflows. Missed ticks (app was down) are skipped:
// a due workflow runs once and its next run is computed from now.
func (e *Engine) Tick(now time.Time) {
	due, err := e.App.FindRecordsByFilter("flows",
		"enabled = true && trigger_type = 'cron' && published_version > 0 && next_run_at != '' && next_run_at <= {:now}",
		"next_run_at", 0, 0, dbx.Params{"now": dbTime(now)})
	if err != nil {
		e.App.Logger().Error("cron tick query failed", "error", err)
		return
	}
	for _, wf := range due {
		e.fireCron(wf, now)
	}
}

func (e *Engine) fireCron(wf *core.Record, now time.Time) {
	def, _, err := e.PublishedDefinition(wf)
	if err != nil {
		return
	}
	scheduled := wf.GetDateTime("next_run_at").Time()
	defer func() {
		if err := e.scheduleNext(wf, def, now); err != nil {
			e.App.Logger().Error("scheduling next cron run failed", "workflow", wf.GetString("name"), "error", err)
		}
		if err := e.App.Save(wf); err != nil {
			e.App.Logger().Error("saving workflow schedule failed", "error", err)
		}
	}()

	if def.Trigger.MaxRuns > 0 {
		n, _ := e.App.CountRecords("flow_runs", dbx.HashExp{"workflow": wf.Id, "source": "cron"})
		if int(n) >= def.Trigger.MaxRuns {
			wf.Set("enabled", false)
			return
		}
	}
	if def.Trigger.Overlap != "queue" {
		active, _ := e.App.CountRecords("flow_runs", dbx.HashExp{"workflow": wf.Id},
			dbx.In("status", StatusQueued, StatusRunning, StatusWaiting))
		if active > 0 {
			e.App.Logger().Info("cron run skipped: previous run still active", "workflow", wf.GetString("name"))
			return
		}
	}
	input := def.Trigger.Input
	if input != nil {
		if input, err = RenderMap(input, map[string]any{"now": now.UTC().Format(time.RFC3339)}); err != nil {
			e.App.Logger().Error("cron input template failed", "workflow", wf.GetString("name"), "error", err)
			return
		}
	}
	if _, err := e.StartRun(wf, input, StartOptions{Source: "cron", ScheduledFor: scheduled}); err != nil {
		e.App.Logger().Error("starting cron run failed", "workflow", wf.GetString("name"), "error", err)
	}
}

func dbTime(t time.Time) string {
	dt, _ := types.ParseDateTime(t.UTC())
	return strings.TrimSpace(dt.String())
}
