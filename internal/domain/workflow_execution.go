package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type WorkflowExecutionStatus string

const (
	WorkflowExecutionPending   WorkflowExecutionStatus = "pending"
	WorkflowExecutionRunning   WorkflowExecutionStatus = "running"
	WorkflowExecutionSucceeded WorkflowExecutionStatus = "succeeded"
	WorkflowExecutionFailed    WorkflowExecutionStatus = "failed"
	WorkflowExecutionCancelled WorkflowExecutionStatus = "cancelled"
)

func (status WorkflowExecutionStatus) Valid() bool {
	switch status {
	case WorkflowExecutionPending, WorkflowExecutionRunning, WorkflowExecutionSucceeded,
		WorkflowExecutionFailed, WorkflowExecutionCancelled:
		return true
	default:
		return false
	}
}

func (status WorkflowExecutionStatus) Terminal() bool {
	return status == WorkflowExecutionSucceeded ||
		status == WorkflowExecutionFailed ||
		status == WorkflowExecutionCancelled
}

type WorkflowNodeExecutionStatus string

const (
	WorkflowNodePending   WorkflowNodeExecutionStatus = "pending"
	WorkflowNodeQueued    WorkflowNodeExecutionStatus = "queued"
	WorkflowNodeRunning   WorkflowNodeExecutionStatus = "running"
	WorkflowNodeSucceeded WorkflowNodeExecutionStatus = "succeeded"
	WorkflowNodeFailed    WorkflowNodeExecutionStatus = "failed"
	WorkflowNodeSkipped   WorkflowNodeExecutionStatus = "skipped"
	WorkflowNodeCancelled WorkflowNodeExecutionStatus = "cancelled"
)

func (status WorkflowNodeExecutionStatus) Terminal() bool {
	switch status {
	case WorkflowNodeSucceeded, WorkflowNodeFailed, WorkflowNodeSkipped, WorkflowNodeCancelled:
		return true
	default:
		return false
	}
}

type WorkflowExecution struct {
	ID                 uuid.UUID               `json:"execution_id"`
	OrgID              uuid.UUID               `json:"org_id"`
	WorkflowID         uuid.UUID               `json:"workflow_id"`
	WorkflowVersionID  uuid.UUID               `json:"workflow_version_id"`
	WorkflowVersion    int                     `json:"workflow_version"`
	Status             WorkflowExecutionStatus `json:"status"`
	Input              json.RawMessage         `json:"input,omitempty"`
	Output             json.RawMessage         `json:"output,omitempty"`
	ErrorCode          string                  `json:"error_code,omitempty"`
	ErrorMessage       string                  `json:"error_message,omitempty"`
	IdempotencyKey     string                  `json:"-"`
	RequestFingerprint string                  `json:"-"`
	StartedAt          *time.Time              `json:"started_at,omitempty"`
	CompletedAt        *time.Time              `json:"completed_at,omitempty"`
	CreatedAt          time.Time               `json:"created_at"`
	UpdatedAt          time.Time               `json:"updated_at"`
}

type WorkflowNodeExecution struct {
	ID                  uuid.UUID                   `json:"id"`
	OrgID               uuid.UUID                   `json:"org_id"`
	WorkflowExecutionID uuid.UUID                   `json:"workflow_execution_id"`
	NodeID              string                      `json:"node_id"`
	NodeType            WorkflowNodeType            `json:"node_type"`
	Status              WorkflowNodeExecutionStatus `json:"status"`
	Attempt             int                         `json:"attempt"`
	Input               json.RawMessage             `json:"input,omitempty"`
	Output              json.RawMessage             `json:"output,omitempty"`
	ErrorCode           string                      `json:"error_code,omitempty"`
	ErrorMessage        string                      `json:"error_message,omitempty"`
	QueueTaskID         *uuid.UUID                  `json:"queue_task_id,omitempty"`
	WebhookDeliveryID   *uuid.UUID                  `json:"webhook_delivery_id,omitempty"`
	AvailableAt         *time.Time                  `json:"available_at,omitempty"`
	StartedAt           *time.Time                  `json:"started_at,omitempty"`
	CompletedAt         *time.Time                  `json:"completed_at,omitempty"`
	CreatedAt           time.Time                   `json:"created_at"`
	UpdatedAt           time.Time                   `json:"updated_at"`
}

type WorkflowExecutionFilter struct {
	Status WorkflowExecutionStatus
	Cursor string
	Limit  int
}

type WorkflowExecutionPage struct {
	Executions []*WorkflowExecution `json:"items"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

type WorkflowExecutionDetail struct {
	*WorkflowExecution
	Nodes []*WorkflowNodeExecution `json:"nodes"`
}

type WorkflowExecutionStartResult struct {
	ExecutionID     uuid.UUID               `json:"execution_id"`
	WorkflowID      uuid.UUID               `json:"workflow_id"`
	WorkflowVersion int                     `json:"workflow_version"`
	Status          WorkflowExecutionStatus `json:"status"`
	CreatedAt       time.Time               `json:"created_at"`
}

type WorkflowWebhookPayload struct {
	EventID             uuid.UUID        `json:"event_id"`
	EventType           WebhookEventType `json:"event_type"`
	CreatedAt           time.Time        `json:"created_at"`
	OrgID               uuid.UUID        `json:"org_id"`
	WorkflowID          uuid.UUID        `json:"workflow_id"`
	WorkflowExecutionID uuid.UUID        `json:"workflow_execution_id"`
	NodeID              string           `json:"node_id"`
	Input               json.RawMessage  `json:"input,omitempty"`
}
