package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type MetricSource string

const (
	MetricSourceQueueFlow  MetricSource = "queueflow"
	MetricSourceEventForge MetricSource = "eventforge"
	MetricSourceTaskCanvas MetricSource = "taskcanvas"
	MetricSourceWorker     MetricSource = "worker"
	MetricSourceQueue      MetricSource = "queue"
)

func (source MetricSource) Valid() bool {
	_, ok := metricEventsBySource[source]
	return ok
}

type MetricEventType string

const (
	MetricTaskIngested       MetricEventType = "task.ingested"
	MetricTaskStarted        MetricEventType = "task.started"
	MetricTaskSucceeded      MetricEventType = "task.succeeded"
	MetricTaskFailed         MetricEventType = "task.failed"
	MetricTaskRetryScheduled MetricEventType = "task.retry_scheduled"
	MetricTaskDeadLettered   MetricEventType = "task.dead_lettered"
	MetricTaskCancelled      MetricEventType = "task.cancelled"

	MetricDeliveryCreated        MetricEventType = "delivery.created"
	MetricDeliveryStarted        MetricEventType = "delivery.started"
	MetricDeliverySucceeded      MetricEventType = "delivery.succeeded"
	MetricDeliveryFailed         MetricEventType = "delivery.failed"
	MetricDeliveryRetryScheduled MetricEventType = "delivery.retry_scheduled"
	MetricDeliveryExhausted      MetricEventType = "delivery.exhausted"

	MetricWorkflowExecutionCreated   MetricEventType = "workflow_execution.created"
	MetricWorkflowExecutionStarted   MetricEventType = "workflow_execution.started"
	MetricWorkflowExecutionSucceeded MetricEventType = "workflow_execution.succeeded"
	MetricWorkflowExecutionFailed    MetricEventType = "workflow_execution.failed"
	MetricWorkflowExecutionCancelled MetricEventType = "workflow_execution.cancelled"
	MetricNodeExecutionStarted       MetricEventType = "node_execution.started"
	MetricNodeExecutionSucceeded     MetricEventType = "node_execution.succeeded"
	MetricNodeExecutionFailed        MetricEventType = "node_execution.failed"
	MetricNodeExecutionSkipped       MetricEventType = "node_execution.skipped"
	MetricNodeExecutionCancelled     MetricEventType = "node_execution.cancelled"

	MetricWorkerRegistered MetricEventType = "worker.registered"
	MetricWorkerHeartbeat  MetricEventType = "worker.heartbeat"
	MetricWorkerStopped    MetricEventType = "worker.stopped"
	MetricWorkerStale      MetricEventType = "worker.stale"

	MetricQueueSnapshotCaptured MetricEventType = "queue.snapshot.captured"
)

type MetricResourceType string

const (
	MetricResourceTask              MetricResourceType = "task"
	MetricResourceWebhookDelivery   MetricResourceType = "webhook_delivery"
	MetricResourceWorkflowExecution MetricResourceType = "workflow_execution"
	MetricResourceNodeExecution     MetricResourceType = "workflow_node_execution"
	MetricResourceWorker            MetricResourceType = "worker"
	MetricResourceQueueSnapshot     MetricResourceType = "queue_snapshot"
)

// MetricMetadata is intentionally closed and low-cardinality. Never add raw
// payloads, URLs, idempotency keys, secrets, or error messages here.
type MetricMetadata struct {
	Attempt        *int   `json:"attempt,omitempty"`
	MaxAttempts    *int   `json:"max_attempts,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	PreviousStatus string `json:"previous_status,omitempty"`
}

type MetricEvent struct {
	ID             uuid.UUID          `json:"id"`
	OrganizationID uuid.UUID          `json:"organization_id"`
	Source         MetricSource       `json:"source"`
	EventType      MetricEventType    `json:"event_type"`
	ResourceType   MetricResourceType `json:"resource_type"`
	ResourceID     string             `json:"resource_id"`
	Queue          string             `json:"queue,omitempty"`
	Status         string             `json:"status,omitempty"`
	DurationMS     *int64             `json:"duration_ms,omitempty"`
	OccurredAt     time.Time          `json:"occurred_at"`
	Metadata       MetricMetadata     `json:"metadata"`
	CreatedAt      time.Time          `json:"created_at"`
}

type NewMetricEventInput struct {
	OrganizationID uuid.UUID
	Source         MetricSource
	EventType      MetricEventType
	ResourceType   MetricResourceType
	ResourceID     string
	Queue          string
	Status         string
	DurationMS     *int64
	OccurredAt     time.Time
	Metadata       MetricMetadata
	TransitionKey  string
}

var metricNamespace = uuid.MustParse("3cdd98e8-7f59-4f10-9dfa-967ac6c625e0")

var metricEventsBySource = map[MetricSource]map[MetricEventType]struct{}{
	MetricSourceQueueFlow: setMetricTypes(
		MetricTaskIngested, MetricTaskStarted, MetricTaskSucceeded, MetricTaskFailed,
		MetricTaskRetryScheduled, MetricTaskDeadLettered, MetricTaskCancelled,
	),
	MetricSourceEventForge: setMetricTypes(
		MetricDeliveryCreated, MetricDeliveryStarted, MetricDeliverySucceeded,
		MetricDeliveryFailed, MetricDeliveryRetryScheduled, MetricDeliveryExhausted,
	),
	MetricSourceTaskCanvas: setMetricTypes(
		MetricWorkflowExecutionCreated, MetricWorkflowExecutionStarted,
		MetricWorkflowExecutionSucceeded, MetricWorkflowExecutionFailed,
		MetricWorkflowExecutionCancelled, MetricNodeExecutionStarted,
		MetricNodeExecutionSucceeded, MetricNodeExecutionFailed,
		MetricNodeExecutionSkipped, MetricNodeExecutionCancelled,
	),
	MetricSourceWorker: setMetricTypes(
		MetricWorkerRegistered, MetricWorkerHeartbeat, MetricWorkerStopped, MetricWorkerStale,
	),
	MetricSourceQueue: setMetricTypes(MetricQueueSnapshotCaptured),
}

var metricResourcesBySource = map[MetricSource]MetricResourceType{
	MetricSourceQueueFlow:  MetricResourceTask,
	MetricSourceEventForge: MetricResourceWebhookDelivery,
	MetricSourceWorker:     MetricResourceWorker,
	MetricSourceQueue:      MetricResourceQueueSnapshot,
}

func NewMetricEvent(input NewMetricEventInput) (MetricEvent, error) {
	occurredAt := input.OccurredAt.UTC().Truncate(time.Microsecond)
	event := MetricEvent{
		OrganizationID: input.OrganizationID,
		Source:         input.Source, EventType: input.EventType,
		ResourceType: input.ResourceType, ResourceID: strings.TrimSpace(input.ResourceID),
		Queue: strings.TrimSpace(input.Queue), Status: strings.TrimSpace(input.Status),
		DurationMS: input.DurationMS, OccurredAt: occurredAt,
		Metadata: input.Metadata, CreatedAt: occurredAt,
	}
	identity := strings.Join([]string{
		event.OrganizationID.String(), string(event.Source), string(event.EventType),
		string(event.ResourceType), event.ResourceID, strings.TrimSpace(input.TransitionKey),
	}, "\x1f")
	event.ID = uuid.NewSHA1(metricNamespace, []byte(identity))
	if err := event.Validate(); err != nil {
		return MetricEvent{}, err
	}
	return event, nil
}

func (event MetricEvent) Validate() error {
	if event.ID == uuid.Nil || event.OrganizationID == uuid.Nil || event.OccurredAt.IsZero() ||
		event.CreatedAt.IsZero() || event.ResourceID == "" || len(event.ResourceID) > 255 ||
		len(event.Queue) > 64 || len(event.Status) > 64 {
		return ErrInvalidInput
	}
	if !metricEventAllowed(event.Source, event.EventType) ||
		!metricResourceAllowed(event.Source, event.ResourceType) {
		return ErrInvalidInput
	}
	if event.DurationMS != nil && *event.DurationMS < 0 {
		return ErrInvalidInput
	}
	if event.Metadata.Attempt != nil && *event.Metadata.Attempt < 0 ||
		event.Metadata.MaxAttempts != nil && *event.Metadata.MaxAttempts < 0 ||
		len(event.Metadata.ErrorCode) > 64 || len(event.Metadata.PreviousStatus) > 64 {
		return ErrInvalidInput
	}
	encoded, err := json.Marshal(event.Metadata)
	if err != nil || len(encoded) > 512 {
		return ErrInvalidInput
	}
	return nil
}

func metricEventAllowed(source MetricSource, eventType MetricEventType) bool {
	_, ok := metricEventsBySource[source][eventType]
	return ok
}

func metricResourceAllowed(source MetricSource, resource MetricResourceType) bool {
	if source == MetricSourceTaskCanvas {
		return resource == MetricResourceWorkflowExecution || resource == MetricResourceNodeExecution
	}
	return metricResourcesBySource[source] == resource
}

func setMetricTypes(values ...MetricEventType) map[MetricEventType]struct{} {
	result := make(map[MetricEventType]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

type MetricEventFilter struct {
	From      time.Time
	To        time.Time
	Source    MetricSource
	EventType MetricEventType
	Cursor    string
	Limit     int
}

func (filter MetricEventFilter) Validate() error {
	if filter.From.IsZero() || filter.To.IsZero() || !filter.From.Before(filter.To) ||
		filter.To.Sub(filter.From) > 31*24*time.Hour || filter.Limit < 1 || filter.Limit > 1000 {
		return ErrInvalidInput
	}
	if filter.Source != "" && filter.EventType != "" && !metricEventAllowed(filter.Source, filter.EventType) {
		return ErrInvalidInput
	}
	if filter.Source != "" && !filter.Source.Valid() {
		return ErrInvalidInput
	}
	if filter.EventType != "" && filter.Source == "" {
		return ErrInvalidInput
	}
	return nil
}

type MetricEventPage struct {
	Items      []MetricEvent `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

func MetricTransitionKey(parts ...any) string {
	values := make([]string, len(parts))
	for i, part := range parts {
		values[i] = fmt.Sprint(part)
	}
	return strings.Join(values, ":")
}
