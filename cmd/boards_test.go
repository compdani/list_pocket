package main

import (
	"encoding/json"
	"io"
	"log"
	"testing"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/flow"
	"github.com/compdani/list_pocket/internal/flow/agent"
	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/list_pocket/internal/flow/queue"
	"github.com/compdani/list_pocket/internal/flow/tools"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/pocketbase/pocketbase"
)

// boardsTestApp is an app whose moves start the workflow watching for them. Its engine
// doesn't run steps, so the runs it creates can be counted.
func boardsTestApp(t *testing.T) (*App, string, func() int) {
	t.Helper()
	pbApp := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pbApp.ResetBootstrapState() })
	if err := pbApp.RunAppMigrations(); err != nil {
		t.Fatal(err)
	}
	lang, err := i18n.New([]byte(`{"_.code":"en","_.name":"English"}`))
	if err != nil {
		t.Fatal(err)
	}
	db, err := pbdb.NewFromPocketBase(pbApp)
	if err != nil {
		t.Fatal(err)
	}
	c := core.New(&core.Opt{DB: db, I18n: lang, Log: log.New(io.Discard, "", 0)}, &core.Hooks{})
	sub, _, err := c.InsertSubscriber(models.Subscriber{Email: "a@example.com", Name: "Ada"}, nil, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}

	runner := &agent.Runner{App: pbApp, LLM: llm.NewClient(), Tools: tools.NewRegistry()}
	engine := flow.NewEngine(pbApp, runner, nil, queue.New(pbApp))
	var def flow.Definition
	if err := json.Unmarshal([]byte(`{
  "name": "on_customer",
  "trigger": {"type": "event", "event": "contact.stage_changed",
    "filter": "event.board == 'pipeline' && event.stage == 'customer' && event.email == 'a@example.com'",
    "input_map": {"contact": "{{ event.contact_id }}"}},
  "nodes": [{"id": "start", "type": "start", "next": "done"}, {"id": "done", "type": "end"}]
}`), &def); err != nil {
		t.Fatal(err)
	}
	actor := flow.Actor{ID: "tester"}
	wf, err := engine.Import(&def, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, v, err := engine.Publish(wf, actor, ""); err != nil {
		t.Fatalf("publish: %v %+v", err, v)
	}

	a := &App{core: c, pb: pbApp, flows: &flowEngine{engine: engine}}
	runs := func() int {
		n, err := pbApp.CountRecords("flow_runs")
		if err != nil {
			t.Fatal(err)
		}
		return int(n)
	}
	return a, sub.RecordID, runs
}

func TestMoveContactOnBoard_raisesStageChangesOnlyWhenTheStageChanges(t *testing.T) {
	a, sub, runs := boardsTestApp(t)
	move := func(stage string, position *int) {
		t.Helper()
		if _, err := a.moveContactOnBoard(core.BoardCardMove{Board: "pipeline", Subscriber: sub, Stage: stage, Position: position, Actor: "Sam"}); err != nil {
			t.Fatal(err)
		}
	}

	move("lead", nil)
	if got := runs(); got != 0 {
		t.Fatalf("the filter should skip other stages, got %d runs", got)
	}
	move("customer", nil)
	if got := runs(); got != 1 {
		t.Fatalf("a move should start the workflow, got %d runs", got)
	}

	top := 0
	move("customer", &top)
	move("customer", nil)
	if got := runs(); got != 1 {
		t.Fatalf("staying in a stage shouldn't raise anything, got %d runs", got)
	}

	if _, err := a.removeContactFromBoard("pipeline", sub, "Sam"); err != nil {
		t.Fatal(err)
	}
	if got := runs(); got != 1 {
		t.Fatalf("a removal has an empty stage, so the filter should skip it, got %d runs", got)
	}
	move("customer", nil)
	if got := runs(); got != 2 {
		t.Fatalf("coming back should start it again, got %d runs", got)
	}
}
