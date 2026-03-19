package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type dashboardPayload struct {
	Workflows      []workflowDTO      `json:"workflows"`
	ActiveWorkflow *activeWorkflowDTO `json:"activeWorkflow"`
	Contacts       []contactDTO       `json:"contacts"`
	Companies      []companyDTO       `json:"companies"`
	RunLogs        []runLogDTO        `json:"runLogs"`
}

type workflowDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Status      string `json:"status"`
	TriggerType string `json:"triggerType"`
}

type activeWorkflowDTO struct {
	Workflow workflowDTO       `json:"workflow"`
	Nodes    []workflowNodeDTO `json:"nodes"`
	Edges    []workflowEdgeDTO `json:"edges"`
}

type workflowNodeDTO struct {
	ID          string           `json:"id"`
	Type        string           `json:"type"`
	Label       string           `json:"label"`
	Description string           `json:"description"`
	Config      map[string]any   `json:"config"`
	Schema      []map[string]any `json:"schema"`
	PositionX   float64          `json:"positionX"`
	PositionY   float64          `json:"positionY"`
	ContactMode string           `json:"contactMode"`
}

type workflowEdgeDTO struct {
	ID           string         `json:"id"`
	SourceNodeID string         `json:"sourceNodeId"`
	TargetNodeID string         `json:"targetNodeId"`
	SourceHandle string         `json:"sourceHandle"`
	TargetHandle string         `json:"targetHandle"`
	Condition    map[string]any `json:"condition"`
}

type contactDTO struct {
	ID           string   `json:"id"`
	FullName     string   `json:"fullName"`
	Email        string   `json:"email"`
	Phone        string   `json:"phone"`
	Company      string   `json:"company"`
	Stage        string   `json:"stage"`
	Tags         []string `json:"tags"`
	LastActivity string   `json:"lastActivity"`
}

type companyDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Industry string `json:"industry"`
}

type runLogDTO struct {
	WorkflowID   string `json:"workflowId"`
	ID           string `json:"id"`
	WorkflowName string `json:"workflowName"`
	ContactName  string `json:"contactName"`
	Status       string `json:"status"`
	StartedAt    string `json:"startedAt"`
	Summary      string `json:"summary"`
}

type runDetailDTO struct {
	ID             string            `json:"id"`
	WorkflowID     string            `json:"workflowId"`
	WorkflowName   string            `json:"workflowName"`
	ContactName    string            `json:"contactName"`
	Status         string            `json:"status"`
	StartedAt      string            `json:"startedAt"`
	EndedAt        string            `json:"endedAt"`
	Summary        string            `json:"summary"`
	TriggerPayload map[string]any    `json:"triggerPayload"`
	NodeRuns       []nodeRunTraceDTO `json:"nodeRuns"`
}

type nodeRunTraceDTO struct {
	ID        string         `json:"id"`
	NodeID    string         `json:"nodeId"`
	NodeLabel string         `json:"nodeLabel"`
	NodeType  string         `json:"nodeType"`
	Status    string         `json:"status"`
	StartedAt string         `json:"startedAt"`
	EndedAt   string         `json:"endedAt"`
	Input     map[string]any `json:"input"`
	Output    map[string]any `json:"output"`
	Logs      []string       `json:"logs"`
	Error     string         `json:"error"`
}

func buildDashboardPayload(app core.App, activeWorkflowID string) (*dashboardPayload, error) {
	workflows, err := app.FindRecordsByFilter("workflows", "", "", 25, 0)
	if err != nil {
		return nil, err
	}
	subscribers, err := app.FindRecordsByFilter("subscribers", "", "-updated", 50, 0)
	if err != nil {
		return nil, err
	}
	runs, err := app.FindRecordsByFilter("workflow_runs", "", "-started_at", 20, 0)
	if err != nil {
		return nil, err
	}

	payload := &dashboardPayload{
		Workflows: make([]workflowDTO, 0, len(workflows)),
		Contacts:  make([]contactDTO, 0, len(subscribers)),
		Companies: []companyDTO{},
		RunLogs:   make([]runLogDTO, 0, len(runs)),
	}

	contactNames := map[string]string{}
	for _, subscriber := range subscribers {
		attribs := toStringAnyMap(subscriber.Get("attribs"))
		fullName := subscriber.GetString("name")
		if fullName == "" {
			fullName = subscriber.GetString("email")
		}
		contactNames[subscriber.Id] = fullName
		payload.Contacts = append(payload.Contacts, contactDTO{
			ID:           subscriber.Id,
			FullName:     fullName,
			Email:        subscriber.GetString("email"),
			Phone:        asString(attribs["phone"], ""),
			Company:      asString(attribs["company"], ""),
			Stage:        subscriber.GetString("status"),
			Tags:         toStringSlice(attribs["tags"]),
			LastActivity: formatDateValue(subscriber.GetDateTime("updated").Time()),
		})
	}

	workflowNames := map[string]string{}
	for _, workflow := range workflows {
		dto := workflowDTO{
			ID: workflow.Id, Name: workflow.GetString("name"), Version: workflow.GetInt("version"), Status: workflow.GetString("status"), TriggerType: workflow.GetString("trigger_type"),
		}
		payload.Workflows = append(payload.Workflows, dto)
		workflowNames[workflow.Id] = dto.Name
	}

	for _, run := range runs {
		payload.RunLogs = append(payload.RunLogs, runLogDTO{
			WorkflowID: run.GetString("workflow"), ID: run.Id, WorkflowName: workflowNames[run.GetString("workflow")], ContactName: contactNames[run.GetString("contact")], Status: run.GetString("status"), StartedAt: formatDateValue(run.GetDateTime("started_at").Time()), Summary: run.GetString("summary"),
		})
	}

	if len(workflows) == 0 {
		return payload, nil
	}

	selectedWorkflow := workflows[0]
	if activeWorkflowID != "" {
		for _, workflow := range workflows {
			if workflow.Id == activeWorkflowID {
				selectedWorkflow = workflow
				break
			}
		}
	}

	active, err := buildActiveWorkflow(app, selectedWorkflow)
	if err != nil {
		return nil, err
	}
	payload.ActiveWorkflow = active
	return payload, nil
}

func buildActiveWorkflow(app core.App, workflow *core.Record) (*activeWorkflowDTO, error) {
	filter := fmt.Sprintf(`workflow="%s" && is_current=true`, workflow.Id)
	nodes, err := app.FindRecordsByFilter("workflow_nodes", filter, "position_x", 100, 0)
	if err != nil {
		return nil, err
	}
	edges, err := app.FindRecordsByFilter("workflow_edges", filter, "", 100, 0)
	if err != nil {
		return nil, err
	}

	result := &activeWorkflowDTO{
		Workflow: workflowDTO{ID: workflow.Id, Name: workflow.GetString("name"), Version: workflow.GetInt("version"), Status: workflow.GetString("status"), TriggerType: workflow.GetString("trigger_type")},
		Nodes:    make([]workflowNodeDTO, 0, len(nodes)),
		Edges:    make([]workflowEdgeDTO, 0, len(edges)),
	}

	nodeKeysByRecordID := make(map[string]string, len(nodes))
	for _, node := range nodes {
		nodeKey := stableNodeKey(node)
		nodeKeysByRecordID[node.Id] = nodeKey
		result.Nodes = append(result.Nodes, workflowNodeDTO{
			ID: nodeKey, Type: node.GetString("type"), Label: node.GetString("label"), Description: describeNode(node.GetString("type"), node.GetString("contact_mode")), Config: toStringAnyMap(node.Get("config")), Schema: toSchemaSlice(node.Get("schema")), PositionX: node.GetFloat("position_x"), PositionY: node.GetFloat("position_y"), ContactMode: node.GetString("contact_mode"),
		})
	}

	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].PositionX < result.Nodes[j].PositionX })

	for _, edge := range edges {
		edgeKey := edge.GetString("edge_key")
		if edgeKey == "" {
			edgeKey = edge.Id
		}
		result.Edges = append(result.Edges, workflowEdgeDTO{
			ID: edgeKey, SourceNodeID: nodeKeysByRecordID[edge.GetString("source_node")], TargetNodeID: nodeKeysByRecordID[edge.GetString("target_node")], SourceHandle: edge.GetString("source_handle"), TargetHandle: edge.GetString("target_handle"), Condition: toStringAnyMap(edge.Get("condition")),
		})
	}

	return result, nil
}

func buildRunDetail(app core.App, runID string) (*runDetailDTO, error) {
	run, err := app.FindRecordById("workflow_runs", runID)
	if err != nil {
		return nil, err
	}

	workflowName := ""
	if workflowID := run.GetString("workflow"); workflowID != "" {
		if workflow, workflowErr := app.FindRecordById("workflows", workflowID); workflowErr == nil {
			workflowName = workflow.GetString("name")
		}
	}

	contactName := ""
	if contactID := run.GetString("contact"); contactID != "" {
		if contact, contactErr := app.FindRecordById("subscribers", contactID); contactErr == nil {
			contactName = contact.GetString("name")
			if contactName == "" {
				contactName = contact.GetString("email")
			}
		}
	}

	nodeRunRecords, err := app.FindRecordsByFilter("node_runs", fmt.Sprintf(`run="%s"`, runID), "started_at", 200, 0)
	if err != nil {
		return nil, err
	}

	detail := &runDetailDTO{
		ID:             run.Id,
		WorkflowID:     run.GetString("workflow"),
		WorkflowName:   workflowName,
		ContactName:    contactName,
		Status:         run.GetString("status"),
		StartedAt:      formatDateValue(run.GetDateTime("started_at").Time()),
		EndedAt:        formatDateValue(run.GetDateTime("ended_at").Time()),
		Summary:        run.GetString("summary"),
		TriggerPayload: toStringAnyMap(run.Get("trigger_payload")),
		NodeRuns:       make([]nodeRunTraceDTO, 0, len(nodeRunRecords)),
	}

	for _, nodeRun := range nodeRunRecords {
		nodeLabel := ""
		nodeType := ""
		if nodeID := nodeRun.GetString("node"); nodeID != "" {
			if node, nodeErr := app.FindRecordById("workflow_nodes", nodeID); nodeErr == nil {
				nodeLabel = node.GetString("label")
				nodeType = node.GetString("type")
			}
		}

		detail.NodeRuns = append(detail.NodeRuns, nodeRunTraceDTO{
			ID: nodeRun.Id, NodeID: nodeRun.GetString("node"), NodeLabel: nodeLabel, NodeType: nodeType, Status: nodeRun.GetString("status"), StartedAt: formatDateValue(nodeRun.GetDateTime("started_at").Time()), EndedAt: formatDateValue(nodeRun.GetDateTime("ended_at").Time()), Input: toStringAnyMap(nodeRun.Get("input")), Output: toStringAnyMap(nodeRun.Get("output")), Logs: toStringSlice(nodeRun.Get("logs")), Error: nodeRun.GetString("error"),
		})
	}

	return detail, nil
}

func describeNode(nodeType string, contactMode string) string {
	switch nodeType {
	case "trigger":
		return "Receives inbound lead payloads and starts the run."
	case "transform":
		return "Runs a controlled Goja transform to enrich contact context."
	case "condition":
		return "Branches workflow execution using lead or contact values."
	case "event_start":
		return "Anchors a named event date for later wait calculations."
	case "pb_update":
		return "Writes workflow outcomes back into the CRM record."
	case "campaign_launch":
		return "Launches an existing campaign so waits can drive drip steps."
	case "wait_until":
		return "Pauses the workflow until a calculated event-relative time."
	default:
		return "Workflow node"
	}
}

func formatDateValue(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

func toStringSlice(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		if strings, ok := raw.([]string); ok {
			return strings
		}
		var decoded []string
		if decodeJSONValue(raw, &decoded) == nil {
			return decoded
		}
		return nil
	}

	result := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			result = append(result, value)
		}
	}
	return result
}

func toStringAnyMap(raw any) map[string]any {
	if value, ok := raw.(map[string]any); ok {
		return value
	}

	decoded := map[string]any{}
	if decodeJSONValue(raw, &decoded) == nil {
		return decoded
	}
	return map[string]any{}
}

func toSchemaSlice(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]map[string]any); ok {
			return typed
		}
		var decoded []map[string]any
		if decodeJSONValue(raw, &decoded) == nil {
			return decoded
		}
		return nil
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mapped, ok := item.(map[string]any); ok {
			result = append(result, mapped)
		}
	}
	return result
}

func decodeJSONValue(raw any, target any) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}
