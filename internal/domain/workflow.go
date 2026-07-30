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

type WorkflowValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type WorkflowValidationResult struct {
	Valid  bool                      `json:"valid"`
	Errors []WorkflowValidationError `json:"errors"`
}

type WorkflowVersionStatus string

const (
	WorkflowVersionStatusPublished  WorkflowVersionStatus = "published"
	WorkflowVersionStatusDeprecated WorkflowVersionStatus = "deprecated"
)

type WorkflowVersion struct {
	ID          uuid.UUID             `json:"version_id"`
	OrgID       uuid.UUID             `json:"org_id,omitempty"`
	WorkflowID  uuid.UUID             `json:"workflow_id"`
	Version     int                   `json:"version"`
	Name        string                `json:"name"`
	Slug        string                `json:"slug"`
	Description *string               `json:"description"`
	Definition  WorkflowDefinition    `json:"definition"`
	Status      WorkflowVersionStatus `json:"status"`
	PublishedAt time.Time             `json:"published_at"`
	CreatedAt   time.Time             `json:"created_at"`
}

type WorkflowVersionSummary struct {
	Version     int                   `json:"version"`
	VersionID   uuid.UUID             `json:"version_id"`
	Status      WorkflowVersionStatus `json:"status"`
	PublishedAt time.Time             `json:"published_at"`
	Name        string                `json:"name"`
	Slug        string                `json:"slug"`
}

type WorkflowVersionPage struct {
	Versions []WorkflowVersionSummary `json:"items"`
}

type WorkflowPublishResult struct {
	WorkflowID  uuid.UUID             `json:"workflow_id"`
	Version     int                   `json:"version"`
	VersionID   uuid.UUID             `json:"version_id"`
	Status      WorkflowVersionStatus `json:"status"`
	PublishedAt time.Time             `json:"published_at"`
}
