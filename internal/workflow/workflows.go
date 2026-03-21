package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type workflowMutationRequest struct {
	Name  string            `json:"name"`
	Nodes []workflowNodeDTO `json:"nodes"`
	Edges []workflowEdgeDTO `json:"edges"`
}

type workflowValidationResponse struct {
	Errors   []string             `json:"errors"`
	Findings []workflowFindingDTO `json:"findings"`
	Valid    bool                 `json:"valid"`
}

type workflowFindingDTO struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Severity   string `json:"severity"`
	TargetID   string `json:"targetId"`
	TargetType string `json:"targetType"`
}

type workflowCreateRequest struct {
	Name string `json:"name"`
}

type workflowRunRequest struct {
	ContactID string `json:"contactId"`
}

func createWorkflowHandler(re *core.RequestEvent) error {
	collection, err := re.App.FindCollectionByNameOrId("workflows")
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	req := workflowCreateRequest{}
	_ = re.BindBody(&req)

	workflow := core.NewRecord(collection)
	workflow.Set("name", defaultWorkflowName(strings.TrimSpace(req.Name)))
	workflow.Set("description", "")
	workflow.Set("version", 1)
	workflow.Set("status", "draft")
	workflow.Set("trigger_type", "manual")
	workflow.Set("is_published", false)
	workflow.Set("compiled_snapshot", map[string]any{})

	if err := re.App.Save(workflow); err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	payload, err := buildDashboardPayload(re.App, workflow.Id)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return re.JSON(http.StatusOK, payload)
}

func deleteWorkflowHandler(re *core.RequestEvent) error {
	workflowID := re.Request.PathValue("id")
	workflow, err := re.App.FindRecordById("workflows", workflowID)
	if err != nil {
		return re.JSON(http.StatusNotFound, map[string]string{"error": "workflow not found"})
	}

	runs, err := re.App.FindRecordsByFilter("workflow_runs", fmt.Sprintf(`workflow="%s"`, workflowID), "", 1000, 0)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	for _, run := range runs {
		activities, findErr := re.App.FindRecordsByFilter("activities", fmt.Sprintf(`workflow_run="%s"`, run.Id), "", 1000, 0)
		if findErr != nil {
			return re.JSON(http.StatusInternalServerError, map[string]string{"error": findErr.Error()})
		}
		for _, activity := range activities {
			if deleteErr := re.App.Delete(activity); deleteErr != nil {
				return re.JSON(http.StatusInternalServerError, map[string]string{"error": deleteErr.Error()})
			}
		}

		nodeRuns, findErr := re.App.FindRecordsByFilter("node_runs", fmt.Sprintf(`run="%s"`, run.Id), "", 1000, 0)
		if findErr != nil {
			return re.JSON(http.StatusInternalServerError, map[string]string{"error": findErr.Error()})
		}
		for _, nodeRun := range nodeRuns {
			if deleteErr := re.App.Delete(nodeRun); deleteErr != nil {
				return re.JSON(http.StatusInternalServerError, map[string]string{"error": deleteErr.Error()})
			}
		}

		if err := re.App.Delete(run); err != nil {
			return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
	}

	edges, err := re.App.FindRecordsByFilter("workflow_edges", fmt.Sprintf(`workflow="%s"`, workflowID), "", 1000, 0)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	for _, edge := range edges {
		if err := re.App.Delete(edge); err != nil {
			return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
	}

	nodes, err := re.App.FindRecordsByFilter("workflow_nodes", fmt.Sprintf(`workflow="%s"`, workflowID), "", 1000, 0)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	for _, node := range nodes {
		if err := re.App.Delete(node); err != nil {
			return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
	}

	if err := re.App.Delete(workflow); err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	payload, err := buildDashboardPayload(re.App, "")
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return re.JSON(http.StatusOK, payload)
}

func saveWorkflowHandler(re *core.RequestEvent) error {
	workflow, err := re.App.FindRecordById("workflows", re.Request.PathValue("id"))
	if err != nil {
		return re.JSON(http.StatusNotFound, map[string]string{"error": "workflow not found"})
	}

	req := workflowMutationRequest{}
	if err := re.BindBody(&req); err != nil {
		return re.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := applyWorkflowMutation(re.App, workflow, req); err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	payload, err := buildDashboardPayload(re.App, workflow.Id)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return re.JSON(http.StatusOK, payload)
}

func defaultWorkflowName(name string) string {
	if name != "" {
		return name
	}

	return "Untitled Workflow"
}

func validateWorkflowHandler(re *core.RequestEvent) error {
	workflow, err := re.App.FindRecordById("workflows", re.Request.PathValue("id"))
	if err != nil {
		return re.JSON(http.StatusNotFound, map[string]string{"error": "workflow not found"})
	}

	nodes, edges, err := loadWorkflowGraph(re.App, workflow.Id)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	findings := validateWorkflowGraph(nodes, edges)
	return re.JSON(http.StatusOK, workflowValidationResponse{
		Errors:   findingMessages(findings),
		Findings: findings,
		Valid:    len(findings) == 0,
	})
}

func publishWorkflowHandler(re *core.RequestEvent) error {
	workflow, err := re.App.FindRecordById("workflows", re.Request.PathValue("id"))
	if err != nil {
		return re.JSON(http.StatusNotFound, map[string]string{"error": "workflow not found"})
	}

	nodes, edges, err := loadWorkflowGraph(re.App, workflow.Id)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if findings := validateWorkflowGraph(nodes, edges); len(findings) > 0 {
		return re.JSON(http.StatusBadRequest, map[string]string{
			"error": "workflow cannot be published: " + strings.Join(findingMessages(findings), "; "),
		})
	}

	workflow.Set("status", "published")
	workflow.Set("is_published", true)
	if err := re.App.Save(workflow); err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	payload, err := buildDashboardPayload(re.App, workflow.Id)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return re.JSON(http.StatusOK, payload)
}

func runWorkflowHandler(re *core.RequestEvent) error {
	req := workflowRunRequest{}
	_ = re.BindBody(&req)

	workflow, err := re.App.FindRecordById("workflows", re.Request.PathValue("id"))
	if err != nil {
		return re.JSON(http.StatusNotFound, map[string]string{"error": "workflow not found"})
	}

	nodes, edges, err := loadWorkflowGraph(re.App, workflow.Id)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	compiledSnapshot, err := buildCompiledWorkflowSnapshot(workflow, nodes, edges)
	if err != nil {
		return re.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	triggerNode := findTriggerRecord(nodes)
	triggerMode := "manual"
	contactStrategy := "lookup-or-create"
	contactKey := "email"
	triggerTag := ""
	testWebhookPayload := ""
	demoContactID := ""
	if triggerNode != nil {
		config := toStringAnyMap(triggerNode.Get("config"))
		if value, ok := config["mode"].(string); ok && value != "" {
			triggerMode = value
		}
		if value, ok := config["contactStrategy"].(string); ok && value != "" {
			contactStrategy = value
		}
		if value, ok := config["contactKey"].(string); ok && value != "" {
			contactKey = value
		}
		if value, ok := config["tagName"].(string); ok && value != "" {
			triggerTag = value
		}
		if value, ok := config["samplePayload"].(string); ok && value != "" {
			testWebhookPayload = value
		}
		if value, ok := config["demoContactId"].(string); ok && value != "" {
			demoContactID = strings.TrimSpace(value)
		}
	}

	var contact *core.Record
	triggerPayload := map[string]any{"mode": "manual_test", "source": "builder"}

	if triggerMode == "tag_added" || triggerMode == "tag_removed" {
		selectedContactID := strings.TrimSpace(req.ContactID)
		if selectedContactID == "" {
			selectedContactID = demoContactID
		}
		if selectedContactID != "" {
			contact, err = re.App.FindRecordById("subscribers", selectedContactID)
			if err != nil {
				return re.JSON(http.StatusBadRequest, map[string]string{"error": "selected demo contact was not found"})
			}
		}
		if contact == nil {
			contacts, findErr := re.App.FindRecordsByFilter("subscribers", "", "-updated", 1, 0)
			if findErr != nil || len(contacts) == 0 {
				return re.JSON(http.StatusInternalServerError, map[string]string{"error": "no contact available for tag-triggered test run"})
			}
			contact = contacts[0]
		}
		if triggerTag == "" {
			existingTags := toStringSlice(toStringAnyMap(contact.Get("attribs"))["tags"])
			if len(existingTags) > 0 {
				triggerTag = existingTags[0]
			} else {
				triggerTag = "demo-booked"
			}
		}

		beforeTags := []string{}
		afterTags := []string{}
		if triggerMode == "tag_added" {
			beforeTags = withoutTag(toStringSlice(toStringAnyMap(contact.Get("attribs"))["tags"]), triggerTag)
			afterTags = append(copyStringSlice(beforeTags), triggerTag)
		} else {
			beforeTags = append(copyStringSlice(toStringSlice(toStringAnyMap(contact.Get("attribs"))["tags"])), triggerTag)
			afterTags = withoutTag(beforeTags, triggerTag)
		}

		triggerPayload["contactId"] = contact.Id
		triggerPayload["tag"] = triggerTag
		triggerPayload["tagsBefore"] = beforeTags
		triggerPayload["tagsAfter"] = afterTags
		triggerPayload["mode"] = triggerMode
	} else if contactStrategy != "deferred" {
		selectedContactID := strings.TrimSpace(req.ContactID)
		if selectedContactID == "" {
			selectedContactID = demoContactID
		}
		if selectedContactID != "" {
			contact, err = re.App.FindRecordById("subscribers", selectedContactID)
			if err != nil {
				return re.JSON(http.StatusBadRequest, map[string]string{"error": "selected demo contact was not found"})
			}
		}
		if contact == nil {
			contacts, findErr := re.App.FindRecordsByFilter("subscribers", "", "-updated", 1, 0)
			if findErr != nil || len(contacts) == 0 {
				return re.JSON(http.StatusInternalServerError, map[string]string{"error": "no contact available for contact-bound test run"})
			}
			contact = contacts[0]
		}
		triggerPayload["contactId"] = contact.Id
		triggerPayload["lookupField"] = contactKey
		triggerPayload["lookupValue"] = contact.GetString(contactKey)
	} else {
		webhookPayload := map[string]any{"email": "new-lead@example.com", "name": "Raw Webhook Lead", "ownerId": "sales-01", "amount": 4200}
		if testWebhookPayload != "" {
			var decoded map[string]any
			if err := json.Unmarshal([]byte(testWebhookPayload), &decoded); err == nil {
				webhookPayload = decoded
			}
		}
		triggerPayload["webhook"] = webhookPayload
	}

	run, err := createWorkflowRunRecord(re.App, workflow, compiledSnapshot, triggerPayload, contact, "", "Manual test run queued from builder.")
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if err := executeQueuedRun(context.Background(), re.App, newRunEngineForApp(re.App), run); err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	payload, err := buildDashboardPayload(re.App, workflow.Id)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return re.JSON(http.StatusOK, payload)
}

func cancelRunHandler(re *core.RequestEvent) error {
	run, err := re.App.FindRecordById("workflow_runs", re.Request.PathValue("id"))
	if err != nil {
		return re.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}

	status := run.GetString("status")
	if status != "queued" && status != "waiting" {
		return re.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("run cannot be cancelled from status %q", status)})
	}

	now := time.Now().UTC()
	run.Set("status", "cancelled")
	run.Set("summary", "Workflow run was cancelled from the admin.")
	run.Set("resume_from_node", "")
	run.Set("wake_at", nil)
	run.Set("ended_at", now)
	if err := re.App.Save(run); err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	detail, err := buildRunDetail(re.App, run.Id)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return re.JSON(http.StatusOK, detail)
}

func webhookTriggerHandler(re *core.RequestEvent) error {
	hookPath := normalizeWebhookPath(re.Request.PathValue("hookPath"))
	if capture := captureStore.findWaiting(hookPath); capture != nil {
		return handleWebhookCapture(re, capture, hookPath)
	}

	triggerNode, workflow, err := findWorkflowByWebhookPath(re.App, hookPath)
	if err != nil {
		return re.JSON(http.StatusNotFound, map[string]any{"status": http.StatusNotFound, "message": "The requested resource wasn't found.", "data": map[string]any{}})
	}

	config := toStringAnyMap(triggerNode.Get("config"))
	if err := verifyWebhookSecurity(re, config); err != nil {
		return re.JSON(http.StatusUnauthorized, map[string]any{"status": http.StatusUnauthorized, "message": err.Error(), "data": map[string]any{}})
	}

	nodes, edges, err := loadWorkflowGraph(re.App, workflow.Id)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	compiledSnapshot, err := buildCompiledWorkflowSnapshot(workflow, nodes, edges)
	if err != nil {
		return re.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	body, err := io.ReadAll(re.Request.Body)
	if err != nil {
		return re.JSON(http.StatusBadRequest, map[string]string{"error": "failed to read webhook body"})
	}

	webhookPayload := map[string]any{}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &webhookPayload); err != nil {
			return re.JSON(http.StatusBadRequest, map[string]string{"error": "webhook body must be valid JSON"})
		}
	}

	contact, _, err := resolveWebhookContact(re.App, config, webhookPayload)
	if err != nil {
		return re.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	triggerPayload := map[string]any{"mode": "webhook", "source": "webhook", "webhook": webhookPayload}
	if contact != nil {
		contactKey := asString(config["contactKey"], "email")
		triggerPayload["contactId"] = contact.Id
		triggerPayload["lookupField"] = contactKey
		triggerPayload["lookupValue"] = contact.GetString(contactKey)
	}

	run, err := createWorkflowRunRecord(re.App, workflow, compiledSnapshot, triggerPayload, contact, "", "Webhook run queued from trigger.")
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if err := executeQueuedRun(context.Background(), re.App, newRunEngineForApp(re.App), run); err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return re.JSON(http.StatusAccepted, map[string]any{"status": "accepted", "runId": run.Id})
}

func handleWebhookCapture(re *core.RequestEvent, capture *webhookCaptureSession, hookPath string) error {
	triggerNode, workflow, err := findWorkflowByWebhookPath(re.App, hookPath)
	if err != nil {
		captureStore.complete(capture.ID, nil, nil, "", "failed", err.Error())
		return re.JSON(http.StatusNotFound, map[string]any{"status": http.StatusNotFound, "message": "The requested resource wasn't found.", "data": map[string]any{}})
	}

	config := toStringAnyMap(triggerNode.Get("config"))
	if err := verifyWebhookSecurity(re, config); err != nil {
		captureStore.complete(capture.ID, nil, nil, "", "failed", err.Error())
		return re.JSON(http.StatusUnauthorized, map[string]any{"status": http.StatusUnauthorized, "message": err.Error(), "data": map[string]any{}})
	}

	body, err := io.ReadAll(re.Request.Body)
	if err != nil {
		return re.JSON(http.StatusBadRequest, map[string]string{"error": "failed to read webhook body"})
	}

	webhookPayload := map[string]any{}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &webhookPayload); err != nil {
			captureStore.complete(capture.ID, nil, nil, "", "failed", "webhook body must be valid JSON")
			return re.JSON(http.StatusBadRequest, map[string]string{"error": "webhook body must be valid JSON"})
		}
	}

	schema := inferWebhookSchema(webhookPayload)
	if capture.Mode == webhookCaptureInferSchema {
		captureStore.complete(capture.ID, webhookPayload, schema, "", "captured", "")
		return re.JSON(http.StatusAccepted, map[string]any{"status": "captured", "mode": string(capture.Mode)})
	}

	nodes, edges, err := loadWorkflowGraph(re.App, workflow.Id)
	if err != nil {
		captureStore.complete(capture.ID, webhookPayload, schema, "", "failed", err.Error())
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	compiledSnapshot, err := buildCompiledWorkflowSnapshot(workflow, nodes, edges)
	if err != nil {
		captureStore.complete(capture.ID, webhookPayload, schema, "", "failed", err.Error())
		return re.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	runID, err := executeWebhookCaptureTestRun(context.Background(), re.App, workflow, compiledSnapshot, config, webhookPayload)
	if err != nil {
		captureStore.complete(capture.ID, webhookPayload, schema, "", "failed", err.Error())
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	captureStore.complete(capture.ID, webhookPayload, schema, runID, "executed", "")
	return re.JSON(http.StatusAccepted, map[string]any{"status": "executed", "runId": runID})
}

func applyWorkflowMutation(app core.App, workflow *core.Record, req workflowMutationRequest) error {
	existingNodes, existingEdges, err := loadWorkflowGraph(app, workflow.Id)
	if err != nil {
		return err
	}

	existingNodeByKey := make(map[string]*core.Record, len(existingNodes))
	for _, node := range existingNodes {
		existingNodeByKey[stableNodeKey(node)] = node
	}

	nextNodeIDs := make(map[string]struct{}, len(req.Nodes))
	currentNodeRecordByKey := make(map[string]*core.Record, len(req.Nodes))
	for _, nodeDTO := range req.Nodes {
		nextNodeIDs[nodeDTO.ID] = struct{}{}
		currentNode, err := upsertWorkflowNodeVersion(app, workflow, existingNodeByKey[nodeDTO.ID], nodeDTO)
		if err != nil {
			return err
		}
		currentNodeRecordByKey[nodeDTO.ID] = currentNode
	}

	existingEdgeByKey := make(map[string]*core.Record, len(existingEdges))
	for _, edge := range existingEdges {
		existingEdgeByKey[stableEdgeKey(edge)] = edge
	}

	nextEdgeIDs := make(map[string]struct{}, len(req.Edges))
	for _, edgeDTO := range req.Edges {
		nextEdgeIDs[edgeDTO.ID] = struct{}{}
		sourceNode := currentNodeRecordByKey[edgeDTO.SourceNodeID]
		targetNode := currentNodeRecordByKey[edgeDTO.TargetNodeID]
		if sourceNode == nil || targetNode == nil {
			return fmt.Errorf("edge %q references a missing node", edgeDTO.ID)
		}
		if _, err := upsertWorkflowEdgeVersion(app, workflow, existingEdgeByKey[edgeDTO.ID], edgeDTO, sourceNode.Id, targetNode.Id); err != nil {
			return err
		}
	}

	for _, edge := range existingEdges {
		if _, ok := nextEdgeIDs[stableEdgeKey(edge)]; !ok {
			edge.Set("is_current", false)
			if err := app.Save(edge); err != nil {
				return err
			}
		}
	}
	for _, node := range existingNodes {
		if _, ok := nextNodeIDs[stableNodeKey(node)]; !ok {
			node.Set("is_current", false)
			if err := app.Save(node); err != nil {
				return err
			}
		}
	}

	workflow.Set("compiled_snapshot", map[string]any{
		"version":      workflow.GetInt("version"),
		"entryNode":    firstNodeID(req.Nodes),
		"nodeCount":    len(req.Nodes),
		"edgeCount":    len(req.Edges),
		"contactAware": true,
		"nodeKeys":     collectNodeKeys(req.Nodes),
		"edgeKeys":     collectEdgeKeys(req.Edges),
	})
	workflow.Set("name", defaultWorkflowName(strings.TrimSpace(req.Name)))
	workflow.Set("trigger_type", triggerTypeFromNodes(req.Nodes))
	return app.Save(workflow)
}

func loadWorkflowGraph(app core.App, workflowID string) ([]*core.Record, []*core.Record, error) {
	nodes, err := app.FindRecordsByFilter("workflow_nodes", fmt.Sprintf(`workflow="%s" && is_current=true`, workflowID), "", 500, 0)
	if err != nil {
		return nil, nil, err
	}
	edges, err := app.FindRecordsByFilter("workflow_edges", fmt.Sprintf(`workflow="%s" && is_current=true`, workflowID), "", 500, 0)
	if err != nil {
		return nil, nil, err
	}
	return nodes, edges, nil
}

func validateWorkflowGraph(nodes []*core.Record, edges []*core.Record) []workflowFindingDTO {
	if len(nodes) == 0 {
		return []workflowFindingDTO{{Code: "workflow.empty", Message: "workflow must contain at least one node", Severity: "error", TargetType: "workflow"}}
	}

	nodeByID := make(map[string]*core.Record, len(nodes))
	triggerCount := 0
	triggerID := ""
	for _, node := range nodes {
		nodeByID[stableNodeKey(node)] = node
		if node.GetString("type") == "trigger" {
			triggerCount++
			triggerID = stableNodeKey(node)
		}
	}

	findings := make([]workflowFindingDTO, 0)
	if triggerCount != 1 {
		findings = append(findings, workflowFindingDTO{Code: "trigger.count", Message: "workflow must contain exactly one trigger node", Severity: "error", TargetID: triggerID, TargetType: "node"})
	}

	connected := make(map[string]bool, len(nodes))
	for _, edge := range edges {
		sourceRecordID := edge.GetString("source_node")
		targetRecordID := edge.GetString("target_node")
		source := ""
		target := ""
		if sourceNode := findRecordByID(nodes, sourceRecordID); sourceNode != nil {
			source = stableNodeKey(sourceNode)
		}
		if targetNode := findRecordByID(nodes, targetRecordID); targetNode != nil {
			target = stableNodeKey(targetNode)
		}
		if source == "" || target == "" {
			findings = append(findings, workflowFindingDTO{Code: "edge.endpoints", Message: "workflow contains an edge without both endpoints", Severity: "error", TargetID: stableEdgeKey(edge), TargetType: "edge"})
			continue
		}
		if nodeByID[source] == nil || nodeByID[target] == nil {
			findings = append(findings, workflowFindingDTO{Code: "edge.missing_node", Message: "workflow contains an edge that references a missing node", Severity: "error", TargetID: stableEdgeKey(edge), TargetType: "edge"})
			continue
		}

		connected[source] = true
		connected[target] = true
	}

	for _, node := range nodes {
		if node.GetString("type") == "trigger" {
			continue
		}
		if !connected[stableNodeKey(node)] {
			findings = append(findings, workflowFindingDTO{Code: "node.disconnected", Message: fmt.Sprintf("node %q is disconnected", node.GetString("label")), Severity: "error", TargetID: stableNodeKey(node), TargetType: "node"})
		}
	}

	return findings
}

func findingMessages(findings []workflowFindingDTO) []string {
	result := make([]string, 0, len(findings))
	for _, finding := range findings {
		result = append(result, finding.Message)
	}
	return result
}

func firstNodeID(nodes []workflowNodeDTO) string {
	if len(nodes) == 0 {
		return ""
	}
	return nodes[0].ID
}

func triggerTypeFromNodes(nodes []workflowNodeDTO) string {
	for _, node := range nodes {
		if node.Type != "trigger" {
			continue
		}
		switch mode := asString(node.Config["mode"], "manual"); mode {
		case "webhook", "tag_added", "tag_removed":
			return mode
		default:
			return "manual"
		}
	}
	return "manual"
}

func findTriggerRecord(nodes []*core.Record) *core.Record {
	for _, node := range nodes {
		if node.GetString("type") == "trigger" {
			return node
		}
	}
	return nil
}

func createWorkflowRunRecord(app core.App, workflow *core.Record, compiledSnapshot map[string]any, triggerPayload map[string]any, contact *core.Record, companyID string, summary string) (*core.Record, error) {
	now := time.Now().UTC()
	run := core.NewRecord(mustCollection(app, "workflow_runs"))
	run.Set("workflow", workflow.Id)
	run.Set("workflow_version", workflow.GetInt("version"))
	run.Set("status", "queued")
	run.Set("trigger_payload", triggerPayload)
	if contact != nil {
		run.Set("contact", contact.Id)
	}
	if companyID != "" {
		run.Set("company", companyID)
	}
	run.Set("compiled_snapshot", compiledSnapshot)
	run.Set("started_at", now)
	run.Set("summary", summary)
	return run, app.Save(run)
}

func findWorkflowByWebhookPath(app core.App, hookPath string) (*core.Record, *core.Record, error) {
	nodes, err := app.FindRecordsByFilter("workflow_nodes", `is_current=true && type="trigger"`, "", 500, 0)
	if err != nil {
		return nil, nil, err
	}
	for _, node := range nodes {
		config := toStringAnyMap(node.Get("config"))
		if asString(config["mode"], "webhook") != "webhook" {
			continue
		}
		if normalizeWebhookPath(asString(config["path"], "")) != hookPath {
			continue
		}
		workflow, err := app.FindRecordById("workflows", node.GetString("workflow"))
		if err != nil {
			return nil, nil, err
		}
		return node, workflow, nil
	}
	return nil, nil, fmt.Errorf("workflow not found")
}

func verifyWebhookSecurity(re *core.RequestEvent, config map[string]any) error {
	headerName := asString(config["signatureHeader"], "")
	secretRef := asString(config["secretRef"], "")
	if headerName == "" || secretRef == "" {
		return nil
	}

	expected := resolveSecretRef(secretRef)
	if expected == "" {
		return fmt.Errorf("webhook secret is not configured")
	}
	if re.Request.Header.Get(headerName) != expected {
		return fmt.Errorf("webhook signature is invalid")
	}
	return nil
}

func resolveSecretRef(secretRef string) string {
	if strings.HasPrefix(secretRef, "env.") {
		return os.Getenv(strings.TrimPrefix(secretRef, "env."))
	}
	return secretRef
}

func resolveWebhookContact(app core.App, config map[string]any, webhookPayload map[string]any) (*core.Record, string, error) {
	contactStrategy := asString(config["contactStrategy"], "lookup-or-create")
	if contactStrategy == "deferred" {
		return nil, "", nil
	}

	contactKey := asString(config["contactKey"], "email")
	lookupValue := fmt.Sprintf("%v", webhookPayload[contactKey])
	if strings.TrimSpace(lookupValue) == "" {
		return nil, "", fmt.Errorf("webhook payload is missing contact lookup field %q", contactKey)
	}

	contacts, err := app.FindRecordsByFilter("subscribers", fmt.Sprintf(`%s="%s"`, contactKey, escapeFilterValue(lookupValue)), "", 1, 0)
	if err == nil && len(contacts) > 0 {
		return contacts[0], "", nil
	}
	if contactStrategy == "require-existing" {
		return nil, "", fmt.Errorf("no existing contact matched %s=%s", contactKey, lookupValue)
	}

	contact := core.NewRecord(mustCollection(app, "subscribers"))
	firstName, lastName := splitName(asString(webhookPayload["name"], "Webhook Lead"))
	fullName := strings.TrimSpace(strings.TrimSpace(firstName + " " + lastName))
	if fullName == "" {
		fullName = asString(webhookPayload["email"], "Webhook Lead")
	}
	contact.Set("uuid", createWorkflowUUID())
	contact.Set("phone", asString(webhookPayload["phone"], ""))
	contact.Set("first_name", firstName)
	contact.Set("last_name", lastName)
	contact.Set("name", fullName)
	contact.Set("email", asString(webhookPayload["email"], fmt.Sprintf("%s@example.com", strings.ReplaceAll(strings.ToLower(firstName), " ", "-"))))
	contact.Set("status", "enabled")
	attribs := copyWebhookMap(webhookPayload)
	attribs["tags"] = []string{"webhook"}
	contact.Set("attribs", attribs)
	if err := app.Save(contact); err != nil {
		return nil, "", err
	}

	return contact, "", nil
}

func escapeFilterValue(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
}

func splitName(name string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return "Webhook", "Lead"
	}
	if len(parts) == 1 {
		return parts[0], "Lead"
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func withoutTag(tags []string, target string) []string {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != target {
			result = append(result, tag)
		}
	}
	return result
}

func copyStringSlice(items []string) []string {
	return append([]string(nil), items...)
}

func normalizeWebhookPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	trimmed = "/" + strings.Trim(trimmed, "/")
	if strings.HasPrefix(trimmed, "/api/hooks/") {
		trimmed = "/" + strings.TrimPrefix(trimmed, "/api/hooks/")
	}
	if strings.HasPrefix(trimmed, "/hooks/") {
		trimmed = "/" + strings.TrimPrefix(trimmed, "/hooks/")
	}
	return trimmed
}

func collectNodeKeys(nodes []workflowNodeDTO) []string {
	keys := make([]string, 0, len(nodes))
	for _, node := range nodes {
		keys = append(keys, node.ID)
	}
	return keys
}

func collectEdgeKeys(edges []workflowEdgeDTO) []string {
	keys := make([]string, 0, len(edges))
	for _, edge := range edges {
		keys = append(keys, edge.ID)
	}
	return keys
}

func stableNodeKey(node *core.Record) string {
	if node == nil {
		return ""
	}
	if key := node.GetString("node_key"); key != "" {
		return key
	}
	return node.Id
}

func stableEdgeKey(edge *core.Record) string {
	if edge == nil {
		return ""
	}
	if key := edge.GetString("edge_key"); key != "" {
		return key
	}
	return edge.Id
}

func findRecordByID(records []*core.Record, recordID string) *core.Record {
	for _, record := range records {
		if record.Id == recordID {
			return record
		}
	}
	return nil
}

func upsertWorkflowNodeVersion(app core.App, workflow *core.Record, current *core.Record, nodeDTO workflowNodeDTO) (*core.Record, error) {
	if current != nil && nodeVersionMatches(current, nodeDTO) {
		return current, nil
	}

	version := 1
	if current != nil {
		version = current.GetInt("version") + 1
		current.Set("is_current", false)
		if err := app.Save(current); err != nil {
			return nil, err
		}
	}

	node := core.NewRecord(mustCollection(app, "workflow_nodes"))
	node.Set("workflow", workflow.Id)
	node.Set("node_key", nodeDTO.ID)
	node.Set("version", version)
	node.Set("is_current", true)
	node.Set("type", nodeDTO.Type)
	node.Set("label", nodeDTO.Label)
	node.Set("config", nodeDTO.Config)
	node.Set("schema", nodeDTO.Schema)
	node.Set("position_x", nodeDTO.PositionX)
	node.Set("position_y", nodeDTO.PositionY)
	node.Set("contact_mode", nodeDTO.ContactMode)
	if err := app.Save(node); err != nil {
		return nil, err
	}
	return node, nil
}

func upsertWorkflowEdgeVersion(app core.App, workflow *core.Record, current *core.Record, edgeDTO workflowEdgeDTO, sourceNodeRecordID string, targetNodeRecordID string) (*core.Record, error) {
	if current != nil && edgeVersionMatches(current, edgeDTO, sourceNodeRecordID, targetNodeRecordID) {
		return current, nil
	}

	version := 1
	if current != nil {
		version = current.GetInt("version") + 1
		current.Set("is_current", false)
		if err := app.Save(current); err != nil {
			return nil, err
		}
	}

	edge := core.NewRecord(mustCollection(app, "workflow_edges"))
	edge.Set("workflow", workflow.Id)
	edge.Set("edge_key", edgeDTO.ID)
	edge.Set("version", version)
	edge.Set("is_current", true)
	edge.Set("source_node", sourceNodeRecordID)
	edge.Set("target_node", targetNodeRecordID)
	edge.Set("source_handle", edgeDTO.SourceHandle)
	edge.Set("target_handle", edgeDTO.TargetHandle)
	edge.Set("condition", edgeDTO.Condition)
	if err := app.Save(edge); err != nil {
		return nil, err
	}
	return edge, nil
}

func nodeVersionMatches(node *core.Record, nodeDTO workflowNodeDTO) bool {
	return node.GetString("type") == nodeDTO.Type &&
		node.GetString("label") == nodeDTO.Label &&
		node.GetString("contact_mode") == nodeDTO.ContactMode &&
		node.GetFloat("position_x") == nodeDTO.PositionX &&
		node.GetFloat("position_y") == nodeDTO.PositionY &&
		jsonEqual(node.Get("config"), nodeDTO.Config) &&
		jsonEqual(node.Get("schema"), nodeDTO.Schema)
}

func edgeVersionMatches(edge *core.Record, edgeDTO workflowEdgeDTO, sourceNodeRecordID string, targetNodeRecordID string) bool {
	return edge.GetString("source_node") == sourceNodeRecordID &&
		edge.GetString("target_node") == targetNodeRecordID &&
		edge.GetString("source_handle") == edgeDTO.SourceHandle &&
		edge.GetString("target_handle") == edgeDTO.TargetHandle &&
		jsonEqual(edge.Get("condition"), edgeDTO.Condition)
}

func jsonEqual(left any, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr == nil && rightErr == nil {
		return string(leftJSON) == string(rightJSON)
	}
	return reflect.DeepEqual(left, right)
}
