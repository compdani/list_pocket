package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type webhookCaptureMode string

const (
	webhookCaptureInferSchema webhookCaptureMode = "infer_schema"
	webhookCaptureTestRun     webhookCaptureMode = "test_run"
)

type webhookCaptureSession struct {
	ID            string
	WorkflowID    string
	TriggerNodeID string
	Path          string
	Mode          webhookCaptureMode
	Status        string
	RunID         string
	Error         string
	Payload       map[string]any
	Schema        map[string]any
	ExpiresAt     time.Time
}

type webhookCaptureStore struct {
	mu       sync.Mutex
	sequence uint64
	items    map[string]*webhookCaptureSession
}

var captureStore = &webhookCaptureStore{items: map[string]*webhookCaptureSession{}}

func (s *webhookCaptureStore) create(workflowID string, triggerNodeID string, path string, mode webhookCaptureMode) *webhookCaptureSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("capture_%d", atomic.AddUint64(&s.sequence, 1))
	session := &webhookCaptureSession{
		ID:            id,
		WorkflowID:    workflowID,
		TriggerNodeID: triggerNodeID,
		Path:          normalizeWebhookPath(path),
		Mode:          mode,
		Status:        "waiting",
		ExpiresAt:     time.Now().UTC().Add(5 * time.Minute),
	}
	s.items[id] = session
	return cloneWebhookCaptureSession(session)
}

func (s *webhookCaptureStore) get(id string) *webhookCaptureSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.items[id]
	if session == nil {
		return nil
	}
	s.expireLocked()
	return cloneWebhookCaptureSession(session)
}

func (s *webhookCaptureStore) findWaiting(path string) *webhookCaptureSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	normalized := normalizeWebhookPath(path)
	for _, session := range s.items {
		if session.Status == "waiting" && session.Path == normalized {
			return cloneWebhookCaptureSession(session)
		}
	}
	return nil
}

func (s *webhookCaptureStore) complete(id string, payload map[string]any, schema map[string]any, runID string, status string, errMsg string) *webhookCaptureSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.items[id]
	if session == nil {
		return nil
	}
	session.Payload = payload
	session.Schema = schema
	session.RunID = runID
	session.Status = status
	session.Error = errMsg
	return cloneWebhookCaptureSession(session)
}

func (s *webhookCaptureStore) expireLocked() {
	now := time.Now().UTC()
	for _, session := range s.items {
		if session.Status == "waiting" && session.ExpiresAt.Before(now) {
			session.Status = "expired"
		}
	}
}

func cloneWebhookCaptureSession(session *webhookCaptureSession) *webhookCaptureSession {
	if session == nil {
		return nil
	}
	clone := *session
	if session.Payload != nil {
		clone.Payload = copyWebhookMap(session.Payload)
	}
	if session.Schema != nil {
		clone.Schema = copyWebhookMap(session.Schema)
	}
	return &clone
}

func copyWebhookMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	encoded, _ := json.Marshal(source)
	cloned := map[string]any{}
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

type armWebhookCaptureRequest struct {
	Mode string `json:"mode"`
}

func armWebhookCaptureHandler(re *core.RequestEvent) error {
	workflowID := re.Request.PathValue("id")
	workflow, err := re.App.FindRecordById("workflows", workflowID)
	if err != nil {
		return re.JSON(404, map[string]string{"error": "workflow not found"})
	}

	req := armWebhookCaptureRequest{}
	if err := re.BindBody(&req); err != nil {
		return re.JSON(400, map[string]string{"error": err.Error()})
	}

	nodes, _, err := loadWorkflowGraph(re.App, workflow.Id)
	if err != nil {
		return re.JSON(500, map[string]string{"error": err.Error()})
	}

	triggerNode := findTriggerRecord(nodes)
	if triggerNode == nil {
		return re.JSON(400, map[string]string{"error": "workflow has no trigger node"})
	}

	config := toStringAnyMap(triggerNode.Get("config"))
	mode := webhookCaptureMode(req.Mode)
	if mode != webhookCaptureInferSchema && mode != webhookCaptureTestRun {
		return re.JSON(400, map[string]string{"error": "invalid webhook capture mode"})
	}

	session := captureStore.create(workflow.Id, triggerNode.Id, asString(config["path"], "/"), mode)
	return re.JSON(200, serializeWebhookCaptureSession(session))
}

func getWebhookCaptureHandler(re *core.RequestEvent) error {
	sessionID := re.Request.PathValue("sessionId")
	session := captureStore.get(sessionID)
	if session == nil {
		return re.JSON(404, map[string]string{"error": "capture session not found"})
	}
	return re.JSON(200, serializeWebhookCaptureSession(session))
}

func serializeWebhookCaptureSession(session *webhookCaptureSession) map[string]any {
	payloadJSON := ""
	schemaJSON := ""
	if session.Payload != nil {
		encoded, _ := json.MarshalIndent(session.Payload, "", "  ")
		payloadJSON = string(encoded)
	}
	if session.Schema != nil {
		encoded, _ := json.MarshalIndent(session.Schema, "", "  ")
		schemaJSON = string(encoded)
	}

	return map[string]any{
		"id":            session.ID,
		"workflowId":    session.WorkflowID,
		"triggerNodeId": session.TriggerNodeID,
		"mode":          string(session.Mode),
		"status":        session.Status,
		"endpoint":      "/api/hooks" + session.Path,
		"runId":         session.RunID,
		"error":         session.Error,
		"payloadJson":   payloadJSON,
		"schemaJson":    schemaJSON,
		"expiresAt":     session.ExpiresAt.Format(time.RFC3339),
	}
}

func inferWebhookSchema(payload map[string]any) map[string]any {
	return inferSchemaValue(payload)
}

func inferSchemaValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		properties := map[string]any{}
		required := make([]string, 0, len(typed))
		for key, child := range typed {
			required = append(required, key)
			properties[key] = inferSchemaValue(child)
		}
		return map[string]any{"type": "object", "required": required, "properties": properties}
	case []any:
		itemSchema := map[string]any{"type": "string"}
		if len(typed) > 0 {
			itemSchema = inferSchemaValue(typed[0])
		}
		return map[string]any{"type": "array", "items": itemSchema}
	case bool:
		return map[string]any{"type": "boolean"}
	case float64:
		return map[string]any{"type": "number"}
	case string:
		schema := map[string]any{"type": "string"}
		if _, err := time.Parse(time.RFC3339, typed); err == nil {
			schema["format"] = "date-time"
		}
		return schema
	case nil:
		return map[string]any{"type": "null"}
	default:
		return map[string]any{"type": "string"}
	}
}

func executeWebhookCaptureTestRun(ctx context.Context, app core.App, workflow *core.Record, compiledSnapshot map[string]any, config map[string]any, webhookPayload map[string]any) (string, error) {
	contact, _, err := resolveWebhookContact(app, config, webhookPayload)
	if err != nil {
		return "", err
	}

	triggerPayload := map[string]any{"mode": "webhook", "source": "webhook_capture", "webhook": webhookPayload}
	if contact != nil {
		contactKey := asString(config["contactKey"], "email")
		triggerPayload["contactId"] = contact.Id
		triggerPayload["lookupField"] = contactKey
		triggerPayload["lookupValue"] = contact.GetString(contactKey)
	}

	run, err := createWorkflowRunRecord(app, workflow, compiledSnapshot, triggerPayload, contact, "", "Webhook capture test run queued.")
	if err != nil {
		return "", err
	}
	if err := executeQueuedRun(ctx, app, newRunEngineForApp(app), run); err != nil {
		return "", err
	}

	return run.Id, nil
}
