package workflow

import (
	"bytes"
	"encoding/json"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"net/http/httptest"
	"testing"
)

func assistantTestApp(t *testing.T) core.App {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	for _, name := range []string{"workflows", "workflow_nodes", "workflow_edges"} {
		collection := core.NewBaseCollection(name)
		for _, field := range []string{"name", "description", "status", "trigger_type", "workflow", "node_key", "edge_key", "type", "label", "contact_mode", "source_node", "target_node", "source_handle", "target_handle"} {
			collection.Fields.Add(&core.TextField{Name: field})
		}
		for _, field := range []string{"version", "position_x", "position_y"} {
			collection.Fields.Add(&core.NumberField{Name: field})
		}
		for _, field := range []string{"is_published", "is_current"} {
			collection.Fields.Add(&core.BoolField{Name: field})
		}
		for _, field := range []string{"config", "schema", "condition", "compiled_snapshot"} {
			collection.Fields.Add(&core.JSONField{Name: field})
		}
		collection.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		if err := app.Save(collection); err != nil {
			t.Fatal(err)
		}
	}
	return app
}
func assistantSaveRequest(app core.App, body any) (*httptest.ResponseRecorder, error) {
	data, _ := json.Marshal(body)
	e := &core.RequestEvent{App: app}
	e.Request = httptest.NewRequest("POST", "/", bytes.NewReader(data))
	e.Request.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.Response = rec
	return rec, AssistantSave(e)
}
func TestAssistantWorkflowDraftsAreAtomicAndPreservePublishedGraph(t *testing.T) {
	app := assistantTestApp(t)
	graph := workflowMutationRequest{Name: "Welcome", Nodes: []workflowNodeDTO{{ID: "trigger", Type: "trigger", Label: "Start", Config: map[string]any{"mode": "webhook", "path": "/assistant-fixture"}}}}
	rec, err := assistantSaveRequest(app, map[string]any{"graph": graph})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.ID == "" {
		t.Fatal(rec.Body.String())
	}
	saved, err := app.FindRecordById("workflows", result.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := findWorkflowByWebhookPath(app, "/assistant-fixture"); err == nil {
		t.Fatal("unpublished draft accepted a webhook")
	}
	// Stale updates fail without modifying the original draft.
	graph.Name = "Stale edit"
	if _, err := assistantSaveRequest(app, map[string]any{"id": saved.Id, "revision": "old", "graph": graph}); err == nil {
		t.Fatal("stale revision accepted")
	}
	// Invalid graphs leave neither workflow nor graph rows behind.
	before, _ := app.CountRecords("workflows")
	graph.Nodes = append(graph.Nodes, workflowNodeDTO{ID: "orphan", Type: "transform", Label: "Orphan"})
	if _, err := assistantSaveRequest(app, map[string]any{"graph": graph}); err == nil {
		t.Fatal("invalid graph accepted")
	}
	after, _ := app.CountRecords("workflows")
	if before != after {
		t.Fatal("invalid graph partially persisted")
	}
	graph.Nodes = graph.Nodes[:1]
	saved.Set("is_published", true)
	saved.Set("status", "published")
	if err := app.Save(saved); err != nil {
		t.Fatal(err)
	}
	graph.Nodes[0].Label = "Edited draft"
	revision, err := assistantWorkflowRevision(app, saved)
	if err != nil {
		t.Fatal(err)
	}
	rec, err = assistantSaveRequest(app, map[string]any{"id": saved.Id, "revision": revision, "graph": graph})
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(rec.Body.Bytes(), &result)
	if result.ID == saved.Id {
		t.Fatal("published workflow overwritten")
	}
	originalNodes, _, err := loadWorkflowGraph(app, saved.Id)
	if err != nil || originalNodes[0].GetString("label") != "Start" {
		t.Fatal("published graph changed")
	}
	_, webhookWorkflow, err := findWorkflowByWebhookPath(app, "/assistant-fixture")
	if err != nil || webhookWorkflow.Id != saved.Id {
		t.Fatal("draft copy intercepted published webhook")
	}
	fork, _ := app.FindRecordById("workflows", result.ID)
	if fork.GetBool("is_published") || fork.GetString("status") != "draft" {
		t.Fatal("fork is not a draft")
	}
}
