package workflow

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/pocketbase/pocketbase/core"
	"net/http"
	"sort"
	"strings"
)

// AssistantRead uses the same access boundary as the authenticated workflow UI.
func AssistantRead(re *core.RequestEvent) error {
	if id := re.Request.PathValue("id"); id != "" {
		if _, err := re.App.FindRecordById("workflows", id); err != nil {
			return re.NotFoundError("workflow not found", err)
		}
	}
	payload, err := buildDashboardPayload(re.App, re.Request.PathValue("id"))
	if err != nil {
		return err
	}
	revisions := map[string]string{}
	for _, item := range payload.Workflows {
		if record, err := re.App.FindRecordById("workflows", item.ID); err == nil {
			revision, err := assistantWorkflowRevision(re.App, record)
			if err != nil {
				return err
			}
			revisions[item.ID] = revision
		}
	}
	return re.JSON(http.StatusOK, map[string]any{"workflows": payload.Workflows, "activeWorkflow": payload.ActiveWorkflow, "runLogs": payload.RunLogs, "revisions": revisions})
}

// AssistantSave commits a validated graph atomically. Published workflows are
// copied to a draft because their runtime currently resolves current node rows.
func AssistantSave(re *core.RequestEvent) error {
	var in struct {
		ID       string                  `json:"id"`
		Revision string                  `json:"revision"`
		Graph    workflowMutationRequest `json:"graph"`
	}
	if err := re.BindBody(&in); err != nil {
		return re.BadRequestError("invalid workflow proposal", err)
	}
	if len(in.Graph.Nodes) > 200 || len(in.Graph.Edges) > 400 {
		return re.BadRequestError("workflow too large", nil)
	}
	allowed := map[string]bool{"trigger": true, "transform": true, "condition": true, "event_start": true, "wait_until": true, "http_request": true, "pb_update": true, "send_transactional_email": true, "campaign_launch": true}
	seen := map[string]bool{}
	for _, node := range in.Graph.Nodes {
		if node.ID == "" || seen[node.ID] || !allowed[node.Type] {
			return re.BadRequestError("invalid or duplicate node", nil)
		}
		seen[node.ID] = true
	}
	seen = map[string]bool{}
	for _, edge := range in.Graph.Edges {
		if edge.ID == "" || seen[edge.ID] {
			return re.BadRequestError("invalid or duplicate edge", nil)
		}
		seen[edge.ID] = true
	}
	var id string
	err := re.App.RunInTransaction(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("workflows")
		if err != nil {
			return err
		}
		record := core.NewRecord(collection)
		if in.ID != "" {
			existing, err := app.FindRecordById("workflows", in.ID)
			if err != nil {
				return err
			}
			currentRevision, err := assistantWorkflowRevision(app, existing)
			if err != nil {
				return err
			}
			if in.Revision == "" || currentRevision != in.Revision {
				return fmt.Errorf("workflow changed; refresh before applying")
			}
			if !existing.GetBool("is_published") {
				record = existing
			} else {
				in.Graph.Name = strings.TrimSpace(in.Graph.Name) + " (draft)"
			}
		}
		if record.IsNew() {
			record.Set("name", defaultWorkflowName(in.Graph.Name))
			record.Set("status", "draft")
			record.Set("is_published", false)
			record.Set("version", 1)
			record.Set("trigger_type", "manual")
			if err := app.Save(record); err != nil {
				return err
			}
		}
		if err := applyWorkflowMutation(app, record, in.Graph); err != nil {
			return err
		}
		nodes, edges, err := loadWorkflowGraph(app, record.Id)
		if err != nil {
			return err
		}
		if findings := validateWorkflowGraph(nodes, edges); len(findings) > 0 {
			return fmt.Errorf("invalid workflow: %s", strings.Join(findingMessages(findings), "; "))
		}
		id = record.Id
		return nil
	})
	if err != nil {
		return re.BadRequestError("workflow proposal rejected", err)
	}
	return re.JSON(http.StatusOK, map[string]any{"id": id, "status": "draft", "copiedFrom": in.ID})
}

// AssistantValidate checks the same topology rules as publication, without saving.
func AssistantValidate(re *core.RequestEvent) error {
	var in struct {
		Graph workflowMutationRequest `json:"graph"`
	}
	if err := re.BindBody(&in); err != nil {
		return re.BadRequestError("invalid graph", err)
	}
	nodesCollection, err := re.App.FindCollectionByNameOrId("workflow_nodes")
	if err != nil {
		return err
	}
	edgesCollection, err := re.App.FindCollectionByNameOrId("workflow_edges")
	if err != nil {
		return err
	}
	nodes := []*core.Record{}
	edges := []*core.Record{}
	allowed := map[string]bool{"trigger": true, "transform": true, "condition": true, "event_start": true, "wait_until": true, "http_request": true, "pb_update": true, "send_transactional_email": true, "campaign_launch": true}
	seen := map[string]bool{}
	if len(in.Graph.Nodes) > 200 || len(in.Graph.Edges) > 400 {
		return re.BadRequestError("graph too large", nil)
	}
	for _, n := range in.Graph.Nodes {
		if n.ID == "" || seen[n.ID] || !allowed[n.Type] {
			return re.BadRequestError("invalid or duplicate node", nil)
		}
		seen[n.ID] = true
		r := core.NewRecord(nodesCollection)
		r.Id = n.ID
		r.Set("node_key", n.ID)
		r.Set("type", n.Type)
		r.Set("label", n.Label)
		nodes = append(nodes, r)
	}
	seen = map[string]bool{}
	for _, e := range in.Graph.Edges {
		if e.ID == "" || seen[e.ID] {
			return re.BadRequestError("invalid or duplicate edge", nil)
		}
		seen[e.ID] = true
		r := core.NewRecord(edgesCollection)
		r.Id = e.ID
		r.Set("edge_key", e.ID)
		r.Set("source_node", e.SourceNodeID)
		r.Set("target_node", e.TargetNodeID)
		edges = append(edges, r)
	}
	if findings := validateWorkflowGraph(nodes, edges); len(findings) > 0 {
		return re.BadRequestError(strings.Join(findingMessages(findings), "; "), nil)
	}
	return re.JSON(http.StatusOK, map[string]bool{"valid": true})
}

// Workflow collections do not all have an updated field. Hash the complete
// current graph so edits made through any existing entry point invalidate it.
func assistantWorkflowRevision(app core.App, record *core.Record) (string, error) {
	nodes, edges, err := loadWorkflowGraph(app, record.Id)
	if err != nil {
		return "", err
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Id < nodes[j].Id })
	sort.Slice(edges, func(i, j int) bool { return edges[i].Id < edges[j].Id })
	data, err := json.Marshal(map[string]any{"id": record.Id, "name": record.GetString("name"), "status": record.GetString("status"), "version": record.GetInt("version"), "published": record.GetBool("is_published"), "trigger": record.GetString("trigger_type"), "nodes": nodes, "edges": edges})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}
