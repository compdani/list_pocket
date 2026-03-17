package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		subscribers, err := app.FindCollectionByNameOrId("subscribers")
		if err != nil {
			return err
		}

		workflows, err := ensureWorkflowCollection(app, "workflows")
		if err != nil {
			return err
		}
		replaceField(workflows, &core.TextField{Name: "name", Required: true})
		replaceField(workflows, &core.TextField{Name: "description"})
		replaceField(workflows, &core.NumberField{Name: "version", Required: true, Min: ptrFloat(1)})
		replaceField(workflows, &core.SelectField{Name: "status", Values: []string{"draft", "published", "archived"}})
		replaceField(workflows, &core.SelectField{Name: "trigger_type", Values: []string{"manual", "webhook", "tag_added", "tag_removed"}})
		replaceField(workflows, &core.BoolField{Name: "is_published"})
		replaceField(workflows, &core.JSONField{Name: "compiled_snapshot"})
		workflows.AddIndex("idx_workflows_name", false, "name", "")
		if err := app.Save(workflows); err != nil {
			return err
		}

		nodes, err := ensureWorkflowCollection(app, "workflow_nodes")
		if err != nil {
			return err
		}
		replaceField(nodes, &core.RelationField{Name: "workflow", CollectionId: workflows.Id, Required: true})
		replaceField(nodes, &core.TextField{Name: "node_key", Required: true})
		replaceField(nodes, &core.NumberField{Name: "version", Required: true, Min: ptrFloat(1)})
		replaceField(nodes, &core.BoolField{Name: "is_current"})
		replaceField(nodes, &core.TextField{Name: "type", Required: true})
		replaceField(nodes, &core.TextField{Name: "label", Required: true})
		replaceField(nodes, &core.JSONField{Name: "config"})
		replaceField(nodes, &core.JSONField{Name: "schema"})
		replaceField(nodes, &core.NumberField{Name: "position_x"})
		replaceField(nodes, &core.NumberField{Name: "position_y"})
		replaceField(nodes, &core.TextField{Name: "contact_mode"})
		if err := app.Save(nodes); err != nil {
			return err
		}

		edges, err := ensureWorkflowCollection(app, "workflow_edges")
		if err != nil {
			return err
		}
		replaceField(edges, &core.RelationField{Name: "workflow", CollectionId: workflows.Id, Required: true})
		replaceField(edges, &core.TextField{Name: "edge_key", Required: true})
		replaceField(edges, &core.NumberField{Name: "version", Required: true, Min: ptrFloat(1)})
		replaceField(edges, &core.BoolField{Name: "is_current"})
		replaceField(edges, &core.RelationField{Name: "source_node", CollectionId: nodes.Id, Required: true})
		replaceField(edges, &core.RelationField{Name: "target_node", CollectionId: nodes.Id, Required: true})
		replaceField(edges, &core.TextField{Name: "source_handle"})
		replaceField(edges, &core.TextField{Name: "target_handle"})
		replaceField(edges, &core.JSONField{Name: "condition"})
		if err := app.Save(edges); err != nil {
			return err
		}

		runs, err := ensureWorkflowCollection(app, "workflow_runs")
		if err != nil {
			return err
		}
		replaceField(runs, &core.RelationField{Name: "workflow", CollectionId: workflows.Id, Required: true})
		replaceField(runs, &core.NumberField{Name: "workflow_version", Required: true, Min: ptrFloat(1)})
		replaceField(runs, &core.SelectField{Name: "status", Values: []string{"queued", "running", "waiting", "success", "failed", "cancelled"}})
		replaceField(runs, &core.JSONField{Name: "trigger_payload"})
		replaceField(runs, &core.JSONField{Name: "runtime_context"})
		replaceField(runs, &core.JSONField{Name: "current_payload"})
		replaceField(runs, &core.TextField{Name: "resume_from_node"})
		replaceField(runs, &core.DateField{Name: "wake_at"})
		replaceField(runs, &core.RelationField{Name: "contact", CollectionId: subscribers.Id})
		runs.Fields.RemoveByName("company")
		replaceField(runs, &core.JSONField{Name: "compiled_snapshot"})
		replaceField(runs, &core.DateField{Name: "started_at"})
		replaceField(runs, &core.DateField{Name: "ended_at"})
		replaceField(runs, &core.TextField{Name: "summary"})
		if err := app.Save(runs); err != nil {
			return err
		}

		nodeRuns, err := ensureWorkflowCollection(app, "node_runs")
		if err != nil {
			return err
		}
		replaceField(nodeRuns, &core.RelationField{Name: "run", CollectionId: runs.Id, Required: true})
		replaceField(nodeRuns, &core.RelationField{Name: "node", CollectionId: nodes.Id, Required: true})
		replaceField(nodeRuns, &core.SelectField{Name: "status", Values: []string{"queued", "running", "waiting", "success", "failed", "cancelled"}})
		replaceField(nodeRuns, &core.JSONField{Name: "input"})
		replaceField(nodeRuns, &core.JSONField{Name: "output"})
		replaceField(nodeRuns, &core.JSONField{Name: "logs"})
		replaceField(nodeRuns, &core.TextField{Name: "error"})
		replaceField(nodeRuns, &core.DateField{Name: "started_at"})
		replaceField(nodeRuns, &core.DateField{Name: "ended_at"})
		if err := app.Save(nodeRuns); err != nil {
			return err
		}

		activities, err := ensureWorkflowCollection(app, "activities")
		if err != nil {
			return err
		}
		replaceField(activities, &core.RelationField{Name: "contact", CollectionId: subscribers.Id, Required: true})
		replaceField(activities, &core.RelationField{Name: "workflow_run", CollectionId: runs.Id})
		replaceField(activities, &core.TextField{Name: "kind", Required: true})
		replaceField(activities, &core.JSONField{Name: "payload"})
		replaceField(activities, &core.DateField{Name: "happened_at"})
		if err := app.Save(activities); err != nil {
			return err
		}

		return backfillWorkflowGraphVersioning(app)
	}, func(app core.App) error {
		for _, name := range []string{"activities", "node_runs", "workflow_runs", "workflow_edges", "workflow_nodes", "workflows"} {
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

func ensureWorkflowCollection(app core.App, name string) (*core.Collection, error) {
	col, err := app.FindCollectionByNameOrId(name)
	if err == nil {
		return col, nil
	}
	col = core.NewBaseCollection(name)
	return col, nil
}

func replaceField(col *core.Collection, field core.Field) {
	col.Fields.RemoveByName(field.GetName())
	col.Fields.Add(field)
}

func backfillWorkflowGraphVersioning(app core.App) error {
	nodes, err := app.FindRecordsByFilter("workflow_nodes", "", "", 1000, 0)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		changed := false
		if node.GetString("node_key") == "" {
			node.Set("node_key", node.Id)
			changed = true
		}
		if node.GetInt("version") == 0 {
			node.Set("version", 1)
			changed = true
		}
		if _, ok := node.Get("is_current").(bool); !ok {
			node.Set("is_current", true)
			changed = true
		}
		if changed {
			if err := app.Save(node); err != nil {
				return err
			}
		}
	}

	edges, err := app.FindRecordsByFilter("workflow_edges", "", "", 1000, 0)
	if err != nil {
		return err
	}
	for _, edge := range edges {
		changed := false
		if edge.GetString("edge_key") == "" {
			edge.Set("edge_key", edge.Id)
			changed = true
		}
		if edge.GetInt("version") == 0 {
			edge.Set("version", 1)
			changed = true
		}
		if _, ok := edge.Get("is_current").(bool); !ok {
			edge.Set("is_current", true)
			changed = true
		}
		if changed {
			if err := app.Save(edge); err != nil {
				return err
			}
		}
	}
	return nil
}

func ptrFloat(v float64) *float64 {
	return &v
}
