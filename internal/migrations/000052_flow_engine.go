package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const flowJSONMax = 5 << 20

func init() {
	text := func(name string, required bool) core.Field {
		return &core.TextField{Name: name, Required: required, Max: 100_000}
	}
	longText := func(name string) core.Field { return &core.TextField{Name: name, Max: 1_000_000} }
	jsonField := func(name string) core.Field { return &core.JSONField{Name: name, MaxSize: flowJSONMax} }
	intField := func(name string) core.Field {
		return &core.NumberField{Name: name, OnlyInt: true}
	}
	number := func(name string) core.Field { return &core.NumberField{Name: name} }
	date := func(name string) core.Field { return &core.DateField{Name: name} }
	sel := func(name string, required bool, values ...string) core.Field {
		return &core.SelectField{Name: name, Required: required, MaxSelect: 1, Values: values}
	}
	rel := func(name string, to *core.Collection, required, cascade bool) core.Field {
		return &core.RelationField{Name: name, CollectionId: to.Id, Required: required, MaxSelect: 1, CascadeDelete: cascade}
	}
	dates := func(c *core.Collection) {
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true}, &core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	}
	runStatus := []string{"queued", "running", "waiting", "succeeded", "failed", "cancelled", "skipped"}

	// Rules stay nil (superuser only), so everything goes through the permission-checked
	// API. That matters here: these rows hold run input and control what the engine does.
	m.Register(func(app core.App) error {
		flows := core.NewBaseCollection("flows")
		flows.Fields.Add(
			text("name", true), text("description", false), &core.BoolField{Name: "enabled"},
			sel("trigger_type", false, "manual", "cron", "event", "webhook"),
			jsonField("draft"), intField("published_version"), date("next_run_at"),
			text("created_by", false), &core.TextField{Name: "webhook_token", Hidden: true, Max: 100},
		)
		dates(flows)
		flows.AddIndex("idx_flows_name", true, "name", "")
		flows.AddIndex("idx_flows_next_run", false, "next_run_at", "enabled = true AND trigger_type = 'cron'")
		if err := app.Save(flows); err != nil {
			return err
		}

		versions := core.NewBaseCollection("flow_versions")
		versions.Fields.Add(
			rel("workflow", flows, true, true), intField("version"), jsonField("definition"),
			jsonField("validation"), text("note", false), text("published_by", false),
		)
		dates(versions)
		versions.AddIndex("idx_flow_versions_workflow_version", true, "workflow, version", "")
		if err := app.Save(versions); err != nil {
			return err
		}

		runs := core.NewBaseCollection("flow_runs")
		runs.Fields.Add(
			rel("workflow", flows, true, true), intField("version"), sel("status", true, runStatus...),
			sel("source", true, "manual_ui", "cron", "event", "webhook", "subworkflow", "test"),
			text("started_by", false), jsonField("input"), jsonField("output"), longText("error"),
			number("cost_usd"), intField("tokens"), jsonField("test_definition"),
			date("scheduled_for"), date("started"), date("finished"),
		)
		dates(runs)
		runs.AddIndex("idx_flow_runs_workflow_created", false, "workflow, created", "")
		runs.AddIndex("idx_flow_runs_status", false, "status", "")
		if err := app.Save(runs); err != nil {
			return err
		}
		runs.Fields.Add(rel("parent_run", runs, false, false))
		if err := app.Save(runs); err != nil {
			return err
		}

		nodeRuns := core.NewBaseCollection("flow_node_runs")
		nodeRuns.Fields.Add(
			rel("workflow_run", runs, true, true), text("node_id", true), text("node_type", false),
			intField("item_index"), sel("status", true, runStatus...), jsonField("input"), jsonField("output"),
			intField("attempts"), longText("error"), number("cost_usd"), date("started"), date("finished"),
			date("wake_at"), text("waiting_for", false), text("child_run", false),
		)
		dates(nodeRuns)
		nodeRuns.AddIndex("idx_flow_node_runs_run", false, "workflow_run, node_id, item_index", "")
		nodeRuns.AddIndex("idx_flow_node_runs_waiting_for", false, "waiting_for", "status = 'waiting' AND waiting_for != ''")
		if err := app.Save(nodeRuns); err != nil {
			return err
		}

		agentRuns := core.NewBaseCollection("flow_agent_runs")
		agentRuns.Fields.Add(
			rel("node_run", nodeRuns, false, true), sel("status", true, runStatus...), jsonField("resolved_ai"),
			jsonField("input"), jsonField("output"), longText("error"), intField("steps"), intField("prompt_tokens"),
			intField("completion_tokens"), intField("search_requests"), intField("cached_tokens"), number("cost_usd"),
			date("started"), date("finished"),
		)
		dates(agentRuns)
		agentRuns.AddIndex("idx_flow_agent_runs_node_run", false, "node_run", "")
		if err := app.Save(agentRuns); err != nil {
			return err
		}

		steps := core.NewBaseCollection("flow_run_steps")
		steps.Fields.Add(
			rel("agent_run", agentRuns, true, true), intField("seq"),
			sel("type", true, "model", "tool", "approval", "message"), jsonField("payload"), jsonField("usage"),
			intField("duration_ms"), longText("error"),
		)
		dates(steps)
		steps.AddIndex("idx_flow_run_steps_agent_run_seq", true, "agent_run, seq", "")
		if err := app.Save(steps); err != nil {
			return err
		}

		approvals := core.NewBaseCollection("flow_approvals")
		approvals.Fields.Add(
			sel("kind", true, "tool", "review"), rel("node_run", nodeRuns, false, true), text("title", false),
			longText("instructions"), jsonField("payload"), jsonField("edited_payload"),
			sel("status", true, "pending", "approved", "denied", "expired"), text("decided_by", false),
			text("channel", false), date("decided_at"),
		)
		dates(approvals)
		approvals.AddIndex("idx_flow_approvals_status", false, "status, created", "")
		if err := app.Save(approvals); err != nil {
			return err
		}

		jobs := core.NewBaseCollection("flow_jobs")
		jobs.Fields.Add(
			text("kind", true), text("target_id", true), jsonField("payload"), date("available_at"),
			intField("attempts"), intField("max_attempts"), text("locked_by", false), date("locked_until"),
			longText("last_error"),
		)
		dates(jobs)
		jobs.AddIndex("idx_flow_jobs_available", false, "available_at", "")
		return app.Save(jobs)
	}, func(app core.App) error {
		for _, name := range []string{"flow_jobs", "flow_approvals", "flow_run_steps", "flow_agent_runs", "flow_node_runs", "flow_runs", "flow_versions", "flows"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			if err := app.Delete(col); err != nil {
				return err
			}
		}
		return nil
	})
}
