package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type WebhookEventType string

const (
	WebhookEventTaskCreated    WebhookEventType = "task.created"
	WebhookEventTaskProcessing WebhookEventType = "task.processing"
	WebhookEventTaskCompleted  WebhookEventType = "task.completed"
	WebhookEventTaskFailed     WebhookEventType = "task.failed"
	WebhookEventTaskDeadLetter WebhookEventType = "task.dead_letter"
	WebhookEventTaskCancelled  WebhookEventType = "task.cancelled"
)

func (eventType WebhookEventType) Valid() bool {
	switch eventType {
	case WebhookEventTaskCreated, WebhookEventTaskProcessing, WebhookEventTaskCompleted,
		WebhookEventTaskFailed, WebhookEventTaskDeadLetter, WebhookEventTaskCancelled:
		return true
	default:
		return false
	}
}

type WebhookDeliveryStatus string

const (
	WebhookDeliveryPending    WebhookDeliveryStatus = "pending"
	WebhookDeliveryProcessing WebhookDeliveryStatus = "processing"
	WebhookDeliverySucceeded  WebhookDeliveryStatus = "succeeded"
	WebhookDeliveryFailed     WebhookDeliveryStatus = "failed"
	WebhookDeliveryDeadLetter WebhookDeliveryStatus = "dead_letter"
)

type WebhookEndpoint struct {
	ID         uuid.UUID          `json:"id"`
	OrgID      uuid.UUID          `json:"org_id"`
	Name       string             `json:"name"`
	URL        string             `json:"url"`
	SecretHash string             `json:"-"`
	EventTypes []WebhookEventType `json:"event_types"`
	IsActive   bool               `json:"is_active"`
	DeletedAt  *time.Time         `json:"deleted_at,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

type WebhookDelivery struct {
	ID             uuid.UUID             `json:"id"`
	OrgID          uuid.UUID             `json:"org_id"`
	EndpointID     uuid.UUID             `json:"endpoint_id"`
	EventType      WebhookEventType      `json:"event_type"`
	Payload        json.RawMessage       `json:"payload"`
	Status         WebhookDeliveryStatus `json:"status"`
	AttemptCount   int                   `json:"attempt_count"`
	MaxAttempts    int                   `json:"max_attempts"`
	NextAttemptAt  *time.Time            `json:"next_attempt_at,omitempty"`
	LastAttemptAt  *time.Time            `json:"last_attempt_at,omitempty"`
	ResponseStatus *int                  `json:"response_status,omitempty"`
	ResponseBody   *string               `json:"response_body,omitempty"`
	LastError      *string               `json:"last_error,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}
