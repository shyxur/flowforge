package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type WorkflowStatus string

const (
	WorkflowStatusDraft     WorkflowStatus = "draft"
	WorkflowStatusPublished WorkflowStatus = "published"
	WorkflowStatusArchived  WorkflowStatus = "archived"
)

func (s WorkflowStatus) Valid() bool {
	switch s {
	case WorkflowStatusDraft, WorkflowStatusPublished, WorkflowStatusArchived:
		return true
	default:
		return false
	}
}

type WorkflowNodeType string

const (
	WorkflowNodeTrigger   WorkflowNodeType = "trigger"
	WorkflowNodeTask      WorkflowNodeType = "task"
	WorkflowNodeWebhook   WorkflowNodeType = "webhook"
	WorkflowNodeCondition WorkflowNodeType = "condition"
	WorkflowNodeDelay     WorkflowNodeType = "delay"
)

func (t WorkflowNodeType) Valid() bool {
	switch t {
	case WorkflowNodeTrigger, WorkflowNodeTask, WorkflowNodeWebhook, WorkflowNodeCondition, WorkflowNodeDelay:
		return true
	default:
		return false
	}
}

type WorkflowDefinition struct {
	Nodes []WorkflowNode `json:"nodes"`
	Edges []WorkflowEdge `json:"edges"`
}

type WorkflowNode struct {
	ID     string           `json:"id"`
	Type   WorkflowNodeType `json:"type"`
	Name   string           `json:"name"`
	Config map[string]any   `json:"config"`
}

type WorkflowEdge struct {
	ID        string          `json:"id"`
	From      string          `json:"from"`
	To        string          `json:"to"`
	Condition json.RawMessage `json:"condition"`
}

type Workflow struct {
	ID          uuid.UUID          `json:"id"`
	OrgID       uuid.UUID          `json:"org_id"`
	Name        string             `json:"name"`
	Slug        string             `json:"slug"`
	Description *string            `json:"description"`
	Status      WorkflowStatus     `json:"status"`
	Definition  WorkflowDefinition `json:"definition"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	DeletedAt   *time.Time         `json:"-"`
}

type WorkflowFilter struct {
	Status WorkflowStatus
	Cursor string
	Limit  int
}

type WorkflowPage struct {
	Workflows  []*Workflow `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
}
