package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/ports"
)

type QueueFlowWorkflowTaskDispatcher struct {
	service *Service
}

func NewQueueFlowWorkflowTaskDispatcher(service *Service) *QueueFlowWorkflowTaskDispatcher {
	return &QueueFlowWorkflowTaskDispatcher{service: service}
}

func (dispatcher *QueueFlowWorkflowTaskDispatcher) DispatchWorkflowTask(
	ctx context.Context,
	orgID, executionID uuid.UUID,
	nodeID string,
	config map[string]any,
	executionInput json.RawMessage,
) (*domain.Task, error) {
	parsed, err := parseWorkflowTaskConfig(config)
	if err != nil {
		return nil, domain.ErrInvalidInput
	}
	payload := parsed.Payload
	if len(payload) == 0 {
		payload = executionInput
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	timeout := time.Duration(parsed.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = time.Minute
	}
	task, _, err := dispatcher.service.CreateTask(ctx, CreateTaskInput{
		OrgID:             orgID,
		IdempotencyKey:    fmt.Sprintf("workflow:%s:%s", executionID, nodeID),
		Queue:             parsed.Queue,
		Payload:           payload,
		Priority:          parsed.Priority,
		MaxRetries:        parsed.MaxRetries,
		Timeout:           timeout,
		VisibilityTimeout: 30 * time.Second,
		BackoffStrategy:   "exponential",
		TraceID:           executionID.String(),
	})
	return task, err
}

func (dispatcher *QueueFlowWorkflowTaskDispatcher) GetWorkflowTask(
	ctx context.Context,
	orgID, taskID uuid.UUID,
) (*domain.Task, error) {
	return dispatcher.service.GetTask(ctx, orgID, taskID)
}

type EventForgeWorkflowWebhookDispatcher struct {
	endpoints   ports.WebhookEndpointRepository
	deliveries  ports.WebhookDeliveryRepository
	maxAttempts int
}

func NewEventForgeWorkflowWebhookDispatcher(
	endpoints ports.WebhookEndpointRepository,
	deliveries ports.WebhookDeliveryRepository,
	maxAttempts int,
) *EventForgeWorkflowWebhookDispatcher {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	return &EventForgeWorkflowWebhookDispatcher{
		endpoints: endpoints, deliveries: deliveries, maxAttempts: maxAttempts,
	}
}

func (dispatcher *EventForgeWorkflowWebhookDispatcher) DispatchWorkflowWebhook(
	ctx context.Context,
	execution *domain.WorkflowExecution,
	node domain.WorkflowNode,
	executionInput json.RawMessage,
) (*domain.WebhookDelivery, error) {
	config, err := parseWorkflowWebhookConfig(node.Config)
	if err != nil {
		return nil, domain.ErrInvalidInput
	}
	endpointID, err := uuid.Parse(config.EndpointID)
	if err != nil {
		return nil, domain.ErrInvalidInput
	}
	endpoint, err := dispatcher.endpoints.GetWebhookEndpoint(ctx, execution.OrgID, endpointID)
	if err != nil {
		return nil, err
	}
	if !endpoint.IsActive {
		return nil, domain.ErrInvalidStateTransition
	}

	deliveryID := uuid.NewSHA1(
		uuid.NameSpaceOID,
		[]byte(execution.ID.String()+":"+node.ID),
	)
	if existing, getErr := dispatcher.deliveries.GetWebhookDelivery(
		ctx, execution.OrgID, deliveryID,
	); getErr == nil {
		return existing, nil
	}
	payloadInput := config.Payload
	if len(payloadInput) == 0 {
		payloadInput = executionInput
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	payload, err := json.Marshal(domain.WorkflowWebhookPayload{
		EventID:             uuid.New(),
		EventType:           domain.WebhookEventWorkflowNode,
		CreatedAt:           now,
		OrgID:               execution.OrgID,
		WorkflowID:          execution.WorkflowID,
		WorkflowExecutionID: execution.ID,
		NodeID:              node.ID,
		Input:               payloadInput,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal workflow webhook payload: %w", err)
	}
	delivery := &domain.WebhookDelivery{
		ID:          deliveryID,
		OrgID:       execution.OrgID,
		EndpointID:  endpointID,
		EventType:   domain.WebhookEventWorkflowNode,
		Payload:     payload,
		Status:      domain.WebhookDeliveryPending,
		MaxAttempts: dispatcher.maxAttempts,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := dispatcher.deliveries.CreateWebhookDelivery(ctx, delivery); err != nil {
		existing, getErr := dispatcher.deliveries.GetWebhookDelivery(
			ctx, execution.OrgID, deliveryID,
		)
		if getErr == nil {
			return existing, nil
		}
		return nil, errors.Join(err, getErr)
	}
	return delivery, nil
}

func (dispatcher *EventForgeWorkflowWebhookDispatcher) GetWorkflowWebhook(
	ctx context.Context,
	orgID, deliveryID uuid.UUID,
) (*domain.WebhookDelivery, error) {
	return dispatcher.deliveries.GetWebhookDelivery(ctx, orgID, deliveryID)
}
