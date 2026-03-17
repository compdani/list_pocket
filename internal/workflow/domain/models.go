package domain

import "time"

type WorkflowStatus string

const (
	WorkflowStatusDraft     WorkflowStatus = "draft"
	WorkflowStatusPublished WorkflowStatus = "published"
	WorkflowStatusArchived  WorkflowStatus = "archived"
)

type RunStatus string

const (
	RunStatusQueued    RunStatus = "queued"
	RunStatusRunning   RunStatus = "running"
	RunStatusWaiting   RunStatus = "waiting"
	RunStatusSuccess   RunStatus = "success"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

type Workflow struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	Version          int            `json:"version"`
	Status           WorkflowStatus `json:"status"`
	TriggerType      string         `json:"triggerType"`
	IsPublished      bool           `json:"isPublished"`
	CompiledSnapshot map[string]any `json:"compiledSnapshot"`
	DefaultContactID string         `json:"defaultContactId"`
	DefaultCompanyID string         `json:"defaultCompanyId"`
	CreatedBy        string         `json:"createdBy"`
	UpdatedBy        string         `json:"updatedBy"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

type WorkflowNode struct {
	ID          string         `json:"id"`
	WorkflowID  string         `json:"workflowId"`
	Type        string         `json:"type"`
	Label       string         `json:"label"`
	Config      map[string]any `json:"config"`
	Schema      map[string]any `json:"schema"`
	PositionX   float64        `json:"positionX"`
	PositionY   float64        `json:"positionY"`
	ContactMode string         `json:"contactMode"`
}

type WorkflowEdge struct {
	ID           string         `json:"id"`
	WorkflowID   string         `json:"workflowId"`
	SourceNodeID string         `json:"sourceNodeId"`
	TargetNodeID string         `json:"targetNodeId"`
	SourceHandle string         `json:"sourceHandle"`
	TargetHandle string         `json:"targetHandle"`
	Condition    map[string]any `json:"condition"`
}

type WorkflowRun struct {
	ID               string         `json:"id"`
	WorkflowID       string         `json:"workflowId"`
	WorkflowVersion  int            `json:"workflowVersion"`
	Status           RunStatus      `json:"status"`
	TriggerPayload   map[string]any `json:"triggerPayload"`
	RuntimeContext   map[string]any `json:"runtimeContext"`
	CurrentPayload   map[string]any `json:"currentPayload"`
	ResumeFromNode   string         `json:"resumeFromNode"`
	ContactID        string         `json:"contactId"`
	CompanyID        string         `json:"companyId"`
	CompiledSnapshot map[string]any `json:"compiledSnapshot"`
	StartedAt        *time.Time     `json:"startedAt"`
	EndedAt          *time.Time     `json:"endedAt"`
	WakeAt           *time.Time     `json:"wakeAt"`
}

type NodeRun struct {
	ID        string         `json:"id"`
	RunID     string         `json:"runId"`
	NodeID    string         `json:"nodeId"`
	Status    RunStatus      `json:"status"`
	Input     map[string]any `json:"input"`
	Output    map[string]any `json:"output"`
	Logs      []string       `json:"logs"`
	Error     string         `json:"error"`
	StartedAt *time.Time     `json:"startedAt"`
	EndedAt   *time.Time     `json:"endedAt"`
}

type Contact struct {
	ID           string         `json:"id"`
	FirstName    string         `json:"firstName"`
	LastName     string         `json:"lastName"`
	Email        string         `json:"email"`
	Phone        string         `json:"phone"`
	Stage        string         `json:"stage"`
	OwnerID      string         `json:"ownerId"`
	CompanyID    string         `json:"companyId"`
	Tags         []string       `json:"tags"`
	Attributes   map[string]any `json:"attributes"`
	LastActivity *time.Time     `json:"lastActivity"`
}

type Company struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Domain     string         `json:"domain"`
	Industry   string         `json:"industry"`
	OwnerID    string         `json:"ownerId"`
	Attributes map[string]any `json:"attributes"`
}
