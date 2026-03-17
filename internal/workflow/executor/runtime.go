package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/workflow/domain"
	"github.com/dop251/goja"
	"github.com/pocketbase/pocketbase/core"
)

type ExecutionContext struct {
	App        core.App
	Run        domain.WorkflowRun
	Node       domain.WorkflowNode
	Input      map[string]any
	Previous   map[string]any
	RunContext map[string]any
	Contact    *domain.Contact
	Company    *domain.Company
	Env        map[string]string
}

type NodeResult struct {
	Output         map[string]any
	Logs           []string
	Wait           bool
	Branch         string
	ContextUpdates map[string]any
	WakeAt         *time.Time
}

type NodeExecutor interface {
	Type() string
	Execute(ctx context.Context, executionCtx ExecutionContext) (NodeResult, error)
}

type TriggerExecutor struct{}

func (TriggerExecutor) Type() string { return "trigger" }
func (TriggerExecutor) Execute(ctx context.Context, executionCtx ExecutionContext) (NodeResult, error) {
	return NodeResult{
		Output: executionCtx.Input,
		Logs:   []string{"trigger node received payload"},
		Branch: "next",
		ContextUpdates: map[string]any{
			"security": map[string]any{
				"signatureHeader": executionCtx.Node.Config["signatureHeader"],
				"secretRef":       executionCtx.Node.Config["secretRef"],
			},
			"triggerSchema": executionCtx.Node.Config["payloadSchema"],
			"users": map[string]any{
				"owner": resolveValue(fmt.Sprintf("input.%v", executionCtx.Node.Config["ownerField"]), executionCtx),
			},
		},
	}, nil
}

type ScriptExecutor struct{}

func (ScriptExecutor) Type() string { return "transform" }
func (ScriptExecutor) Execute(ctx context.Context, executionCtx ExecutionContext) (NodeResult, error) {
	script, _ := executionCtx.Node.Config["script"].(string)
	if script == "" {
		return NodeResult{}, errors.New("transform node requires config.script")
	}

	vm := goja.New()
	if err := vm.Set("ctx", executionCtxToMap(executionCtx)); err != nil {
		return NodeResult{}, err
	}

	value, err := vm.RunString(fmt.Sprintf("(function(){ %s\n })()", script))
	if err != nil {
		return NodeResult{}, err
	}

	output, ok := value.Export().(map[string]any)
	if !ok {
		return NodeResult{}, errors.New("transform script must return an object")
	}

	return NodeResult{Output: output, Logs: []string{"transform node executed"}, Branch: "next"}, nil
}

type ConditionExecutor struct{}

func (ConditionExecutor) Type() string { return "condition" }
func (ConditionExecutor) Execute(ctx context.Context, executionCtx ExecutionContext) (NodeResult, error) {
	field, _ := executionCtx.Node.Config["field"].(string)
	operator, _ := executionCtx.Node.Config["operator"].(string)
	expected, _ := executionCtx.Node.Config["value"].(string)

	actual := fmt.Sprintf("%v", resolveValue(field, executionCtx))
	branch := "no"
	switch operator {
	case "greater_than":
		if toFloat(actual) > toFloat(expected) {
			branch = "yes"
		}
	case "contains":
		if strings.Contains(strings.ToLower(actual), strings.ToLower(expected)) {
			branch = "yes"
		}
	default:
		if actual == expected {
			branch = "yes"
		}
	}

	output := copyMap(executionCtx.Input)
	output["conditionResult"] = branch
	output["conditionField"] = field

	return NodeResult{
		Output: output,
		Logs:   []string{fmt.Sprintf("condition evaluated to %s", branch)},
		Branch: branch,
	}, nil
}

type EventStartExecutor struct{}

func (EventStartExecutor) Type() string { return "event_start" }
func (EventStartExecutor) Execute(ctx context.Context, executionCtx ExecutionContext) (NodeResult, error) {
	eventKey, _ := executionCtx.Node.Config["eventKey"].(string)
	if eventKey == "" {
		return NodeResult{}, errors.New("event start node requires config.eventKey")
	}
	sourcePath, _ := executionCtx.Node.Config["sourcePath"].(string)
	fallbackAt, _ := executionCtx.Node.Config["fallbackAt"].(string)

	resolved := fmt.Sprintf("%v", resolveValue(sourcePath, executionCtx))
	if strings.TrimSpace(resolved) == "" {
		resolved = fallbackAt
	}

	output := copyMap(executionCtx.Input)
	output["eventStart"] = resolved
	output["eventKey"] = eventKey

	return NodeResult{
		Output: output,
		Logs:   []string{fmt.Sprintf("event %s anchored at %s", eventKey, resolved)},
		Branch: "next",
		ContextUpdates: map[string]any{
			"events": map[string]any{
				eventKey: resolved,
			},
		},
	}, nil
}

type HTTPExecutor struct{}

func (HTTPExecutor) Type() string { return "http_request" }
func (HTTPExecutor) Execute(ctx context.Context, executionCtx ExecutionContext) (NodeResult, error) {
	url, _ := executionCtx.Node.Config["url"].(string)
	method, _ := executionCtx.Node.Config["method"].(string)
	sourcePath, _ := executionCtx.Node.Config["sourcePath"].(string)
	authMode, _ := executionCtx.Node.Config["authMode"].(string)
	secretRef, _ := executionCtx.Node.Config["secretRef"].(string)

	if strings.TrimSpace(url) == "" {
		return NodeResult{}, errors.New("http request node requires config.url")
	}
	if method == "" {
		method = http.MethodPost
	}
	if sourcePath == "" {
		sourcePath = "previous"
	}

	payload := resolveValue(sourcePath, executionCtx)
	bodyBytes := []byte("{}")
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return NodeResult{}, fmt.Errorf("http request payload could not be encoded: %w", err)
		}
		bodyBytes = encoded
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), url, bytes.NewReader(bodyBytes))
	if err != nil {
		return NodeResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	if secretRef != "" {
		secretValue := resolveSecret(secretRef, executionCtx)
		switch authMode {
		case "bearer":
			if secretValue != "" {
				req.Header.Set("Authorization", "Bearer "+secretValue)
			}
		case "secret_header":
			if secretValue != "" {
				req.Header.Set("X-Workflow-Secret", secretValue)
			}
		}
	}

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return NodeResult{}, err
	}
	defer resp.Body.Close()

	var responseBody any
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		responseBody = map[string]any{"raw": fmt.Sprintf("non-json response with status %d", resp.StatusCode)}
	}

	return NodeResult{
		Output: map[string]any{
			"status": resp.StatusCode,
			"body":   responseBody,
			"url":    url,
			"method": strings.ToUpper(method),
		},
		Logs:   []string{fmt.Sprintf("http request completed with status %d", resp.StatusCode)},
		Branch: "next",
	}, nil
}

type PocketBaseExecutor struct{}

func (PocketBaseExecutor) Type() string { return "pb_update" }
func (PocketBaseExecutor) Execute(ctx context.Context, executionCtx ExecutionContext) (NodeResult, error) {
	if executionCtx.App == nil {
		return NodeResult{}, errors.New("PocketBase executor requires app context")
	}

	collectionName, _ := executionCtx.Node.Config["collection"].(string)
	if strings.TrimSpace(collectionName) == "" {
		return NodeResult{}, errors.New("pb_update node requires config.collection")
	}

	collection, err := executionCtx.App.FindCollectionByNameOrId(collectionName)
	if err != nil {
		return NodeResult{}, fmt.Errorf("pb_update collection %q not found", collectionName)
	}

	action, _ := executionCtx.Node.Config["action"].(string)
	if action == "" {
		action = "update"
	}

	fieldMap := toStringMap(executionCtx.Node.Config["fieldMap"])
	resolvedFields := make(map[string]any, len(fieldMap))
	for field, rawValue := range fieldMap {
		resolvedFields[field] = resolveConfiguredValue(rawValue, executionCtx)
	}
	if len(resolvedFields) == 0 {
		if sourcePath, _ := executionCtx.Node.Config["sourcePath"].(string); sourcePath != "" {
			if resolved, ok := resolveConfiguredValue(sourcePath, executionCtx).(map[string]any); ok {
				resolvedFields = copyMap(resolved)
			}
		}
	}

	recordIDPath, _ := executionCtx.Node.Config["recordIdPath"].(string)
	recordID := strings.TrimSpace(asString(resolveConfiguredValue(recordIDPath, executionCtx), ""))
	if recordID == "" {
		switch collectionName {
		case "contacts":
			if executionCtx.Contact != nil {
				recordID = executionCtx.Contact.ID
			}
		case "companies":
			if executionCtx.Company != nil {
				recordID = executionCtx.Company.ID
			}
		}
	}

	record, err := loadTargetRecord(executionCtx.App, collectionName, collection, action, recordID)
	if err != nil {
		return NodeResult{}, err
	}

	for field, value := range resolvedFields {
		record.Set(field, value)
	}
	if err := executionCtx.App.Save(record); err != nil {
		return NodeResult{}, fmt.Errorf("pb_update %s failed: %w", action, err)
	}

	output := copyMap(executionCtx.Input)
	output["crmWriteback"] = map[string]any{
		"collection": collectionName,
		"action":     action,
		"recordId":   record.Id,
		"fields":     resolvedFields,
	}

	contextUpdates := map[string]any{}
	switch collectionName {
	case "contacts":
		contextUpdates["contact"] = map[string]any{
			"id":        record.Id,
			"firstName": record.GetString("first_name"),
			"lastName":  record.GetString("last_name"),
			"email":     record.GetString("email"),
			"phone":     record.GetString("phone"),
			"stage":     record.GetString("stage"),
			"ownerId":   record.GetString("owner_id"),
			"companyId": record.GetString("company"),
		}
		contextUpdates["users"] = map[string]any{"owner": record.GetString("owner_id")}
	case "companies":
		contextUpdates["company"] = map[string]any{
			"id":       record.Id,
			"name":     record.GetString("name"),
			"domain":   record.GetString("domain"),
			"industry": record.GetString("industry"),
			"ownerId":  record.GetString("owner_id"),
		}
	}

	return NodeResult{
		Output:         output,
		Logs:           []string{fmt.Sprintf("PocketBase %s completed for %s:%s", action, collectionName, record.Id)},
		Branch:         "next",
		ContextUpdates: contextUpdates,
	}, nil
}

type WaitExecutor struct{}

func (WaitExecutor) Type() string { return "wait_until" }
func (WaitExecutor) Execute(ctx context.Context, executionCtx ExecutionContext) (NodeResult, error) {
	referencePath, _ := executionCtx.Node.Config["referencePath"].(string)
	offsetDays, _ := executionCtx.Node.Config["offsetDays"].(string)
	offsetHours, _ := executionCtx.Node.Config["offsetHours"].(string)
	skipIfPast, _ := executionCtx.Node.Config["skipIfPast"].(string)

	waitAt := parseTime(fmt.Sprintf("%v", resolveValue(referencePath, executionCtx)))
	if waitAt.IsZero() {
		return NodeResult{}, fmt.Errorf("wait node could not resolve reference path %q", referencePath)
	}

	waitAt = waitAt.Add(time.Duration(toFloat(offsetDays)*24) * time.Hour)
	waitAt = waitAt.Add(time.Duration(toFloat(offsetHours)) * time.Hour)

	output := copyMap(executionCtx.Input)
	output["waitUntil"] = waitAt.Format(time.RFC3339)

	if waitAt.Before(time.Now().UTC()) && skipIfPast == "yes" {
		output["waitSkipped"] = true
		return NodeResult{
			Output: output,
			Logs:   []string{fmt.Sprintf("wait skipped because %s is already in the past", waitAt.Format(time.RFC3339))},
			Branch: "next",
		}, nil
	}

	return NodeResult{
		Output: output,
		Logs:   []string{fmt.Sprintf("waiting until %s", waitAt.Format(time.RFC3339))},
		Wait:   true,
		Branch: "next",
		WakeAt: &waitAt,
	}, nil
}

type Engine struct {
	app       core.App
	executors map[string]NodeExecutor
}

type RunResult struct {
	NodeRuns       []domain.NodeRun
	CurrentPayload map[string]any
	RunContext     map[string]any
	NextNodeID     string
	WakeAt         *time.Time
}

func NewEngine(app core.App, executors ...NodeExecutor) *Engine {
	index := make(map[string]NodeExecutor, len(executors))
	for _, executor := range executors {
		index[executor.Type()] = executor
	}
	return &Engine{app: app, executors: index}
}

func (e *Engine) ExecuteFrom(ctx context.Context, compiled *CompiledWorkflow, run domain.WorkflowRun, startNodeID string, payload map[string]any, runContext map[string]any, contact *domain.Contact, company *domain.Company) (RunResult, error) {
	nodeRuns := make([]domain.NodeRun, 0, len(compiled.Nodes))
	currentPayload := copyMap(payload)
	if len(currentPayload) == 0 {
		currentPayload = copyMap(run.TriggerPayload)
	}
	currentNodeID := startNodeID
	if currentNodeID == "" {
		currentNodeID = compiled.TriggerNodeID()
	}
	visited := 0
	runContext = buildRunContext(run, runContext, contact, company)

	for currentNodeID != "" {
		visited++
		if visited > len(compiled.Nodes)+len(compiled.Edges)+1 {
			return RunResult{NodeRuns: nodeRuns, CurrentPayload: currentPayload, RunContext: runContext}, errors.New("workflow traversal exceeded safe limit")
		}

		node, ok := findNode(compiled.Nodes, currentNodeID)
		if !ok {
			return RunResult{NodeRuns: nodeRuns, CurrentPayload: currentPayload, RunContext: runContext}, fmt.Errorf("node %s not found", currentNodeID)
		}

		started := time.Now()
		nodeRun := domain.NodeRun{ID: fmt.Sprintf("%s:%s", run.ID, node.ID), RunID: run.ID, NodeID: node.ID, Status: domain.RunStatusRunning, Input: currentPayload, StartedAt: &started}

		exec, ok := e.executors[node.Type]
		if !ok {
			nodeRun.Status = domain.RunStatusFailed
			nodeRun.Error = "unsupported node type"
			nodeRuns = append(nodeRuns, nodeRun)
			return RunResult{NodeRuns: nodeRuns, CurrentPayload: currentPayload, RunContext: runContext}, fmt.Errorf("unsupported node type: %s", node.Type)
		}

		result, err := exec.Execute(ctx, ExecutionContext{App: e.app, Run: run, Node: node, Input: currentPayload, Previous: currentPayload, RunContext: runContext, Contact: contact, Company: company})
		ended := time.Now()
		nodeRun.EndedAt = &ended
		if err != nil {
			nodeRun.Status = domain.RunStatusFailed
			nodeRun.Error = err.Error()
			nodeRuns = append(nodeRuns, nodeRun)
			return RunResult{NodeRuns: nodeRuns, CurrentPayload: currentPayload, RunContext: runContext}, err
		}

		if result.Wait {
			nodeRun.Status = domain.RunStatusWaiting
		} else {
			nodeRun.Status = domain.RunStatusSuccess
		}
		nodeRun.Output = result.Output
		nodeRun.Logs = result.Logs
		nodeRuns = append(nodeRuns, nodeRun)

		currentPayload = result.Output
		applyContextUpdates(runContext, result.ContextUpdates)
		runContext["previous"] = copyMap(result.Output)
		nextNode := nextNodeID(compiled.Edges, node.ID, result.Branch)
		if result.Wait {
			return RunResult{NodeRuns: nodeRuns, CurrentPayload: currentPayload, RunContext: runContext, NextNodeID: nextNode, WakeAt: result.WakeAt}, nil
		}
		currentNodeID = nextNode
	}

	return RunResult{NodeRuns: nodeRuns, CurrentPayload: currentPayload, RunContext: runContext}, nil
}

func executionCtxToMap(executionCtx ExecutionContext) map[string]any {
	return map[string]any{"input": executionCtx.Input, "previous": executionCtx.Previous, "run": executionCtx.RunContext, "env": executionCtx.Env, "contact": executionCtx.Contact, "company": executionCtx.Company}
}

func findNode(nodes []domain.WorkflowNode, nodeID string) (domain.WorkflowNode, bool) {
	for _, node := range nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	return domain.WorkflowNode{}, false
}

func nextNodeID(edges []domain.WorkflowEdge, nodeID string, branch string) string {
	fallback := ""
	for _, edge := range edges {
		if edge.SourceNodeID != nodeID {
			continue
		}
		edgeBranch := edge.SourceHandle
		if edgeBranch == "" {
			if value, ok := edge.Condition["branch"].(string); ok {
				edgeBranch = value
			}
		}
		if edgeBranch == "" || edgeBranch == "next" {
			if fallback == "" {
				fallback = edge.TargetNodeID
			}
			continue
		}
		if edgeBranch == branch {
			return edge.TargetNodeID
		}
	}
	return fallback
}

func copyMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(source))
	maps.Copy(result, source)
	return result
}

func buildRunContext(run domain.WorkflowRun, existing map[string]any, contact *domain.Contact, company *domain.Company) map[string]any {
	runContext := copyMap(existing)
	if len(runContext) == 0 {
		runContext = map[string]any{
			"triggerPayload": copyMap(run.TriggerPayload),
			"workflow":       map[string]any{"id": run.WorkflowID, "version": run.WorkflowVersion},
			"events":         map[string]any{},
			"users":          map[string]any{},
		}
	}
	if contact != nil {
		runContext["contact"] = map[string]any{"id": contact.ID, "firstName": contact.FirstName, "lastName": contact.LastName, "email": contact.Email, "phone": contact.Phone, "stage": contact.Stage, "ownerId": contact.OwnerID, "companyId": contact.CompanyID}
		users, ok := runContext["users"].(map[string]any)
		if !ok {
			users = map[string]any{}
		}
		users["owner"] = contact.OwnerID
		runContext["users"] = users
	}
	if company != nil {
		runContext["company"] = map[string]any{"id": company.ID, "name": company.Name, "domain": company.Domain, "industry": company.Industry, "ownerId": company.OwnerID}
	}
	return runContext
}

func applyContextUpdates(target map[string]any, updates map[string]any) {
	for key, value := range updates {
		existingMap, existingIsMap := target[key].(map[string]any)
		nextMap, nextIsMap := value.(map[string]any)
		if existingIsMap && nextIsMap {
			applyContextUpdates(existingMap, nextMap)
			target[key] = existingMap
			continue
		}
		target[key] = value
	}
}

func resolveValue(path string, executionCtx ExecutionContext) any {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	switch path {
	case "previous":
		return executionCtx.Previous
	case "input":
		return executionCtx.Input
	case "run":
		return executionCtx.RunContext
	}

	parts := strings.Split(path, ".")
	root, rest := parts[0], parts[1:]

	var current any
	switch root {
	case "previous":
		current = executionCtx.Previous
	case "input":
		current = executionCtx.Input
	case "run":
		current = executionCtx.RunContext
	case "contact":
		current = executionCtxToMap(executionCtx)["contact"]
	case "company":
		current = executionCtxToMap(executionCtx)["company"]
	default:
		current = executionCtx.Input
		rest = parts
	}

	for _, part := range rest {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case *domain.Contact:
			current = map[string]any{"id": typed.ID, "firstName": typed.FirstName, "lastName": typed.LastName, "email": typed.Email, "phone": typed.Phone, "stage": typed.Stage, "ownerId": typed.OwnerID, "companyId": typed.CompanyID}[part]
		case *domain.Company:
			current = map[string]any{"id": typed.ID, "name": typed.Name, "domain": typed.Domain, "industry": typed.Industry, "ownerId": typed.OwnerID}[part]
		default:
			return nil
		}
	}
	return current
}

func toFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func resolveSecret(secretRef string, executionCtx ExecutionContext) string {
	if strings.HasPrefix(secretRef, "env.") {
		return executionCtx.Env[strings.TrimPrefix(secretRef, "env.")]
	}
	if value := resolveValue(secretRef, executionCtx); value != nil {
		return fmt.Sprintf("%v", value)
	}
	return secretRef
}

func loadTargetRecord(app core.App, collectionName string, collection *core.Collection, action string, recordID string) (*core.Record, error) {
	switch action {
	case "create":
		return core.NewRecord(collection), nil
	case "upsert":
		if recordID == "" {
			return core.NewRecord(collection), nil
		}
		record, err := app.FindRecordById(collectionName, recordID)
		if err != nil {
			return core.NewRecord(collection), nil
		}
		return record, nil
	case "update", "":
		if recordID == "" {
			return nil, errors.New("pb_update update requires a resolved record id")
		}
		record, err := app.FindRecordById(collectionName, recordID)
		if err != nil {
			return nil, fmt.Errorf("pb_update target record %q not found in %s", recordID, collectionName)
		}
		return record, nil
	default:
		return nil, fmt.Errorf("pb_update action %q is not supported", action)
	}
}

func resolveConfiguredValue(raw string, executionCtx ExecutionContext) any {
	if value := resolveValue(raw, executionCtx); value != nil {
		return value
	}
	return raw
}

func toStringMap(raw any) map[string]string {
	if raw == nil {
		return map[string]string{}
	}
	if typed, ok := raw.(map[string]string); ok {
		return typed
	}
	if typed, ok := raw.(map[string]any); ok {
		result := make(map[string]string, len(typed))
		for key, value := range typed {
			result[key] = fmt.Sprintf("%v", value)
		}
		return result
	}
	decoded := map[string]string{}
	if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
		if err := json.Unmarshal([]byte(text), &decoded); err == nil {
			return decoded
		}
	}
	return map[string]string{}
}

func asString(value any, fallback string) string {
	text := strings.TrimSpace(fmt.Sprintf("%v", value))
	if text == "" || text == "<nil>" {
		return fallback
	}
	return text
}
