package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/testutil"
)

func TestQueueFlowWorkflowTaskDispatcher(t *testing.T) {
	var created *domain.Task
	storage := &testutil.StorageStub{
		FindByIdempotencyKeyFunc: func(context.Context, uuid.UUID, string, string) (*domain.Task, error) {
			return nil, domain.ErrTaskNotFound
		},
		CreateFunc: func(_ context.Context, task *domain.Task) error {
			created = task
			return nil
		},
	}
	service := NewService(storage, &testutil.BrokerStub{})
	dispatcher := NewQueueFlowWorkflowTaskDispatcher(service)
	orgID, executionID := uuid.New(), uuid.New()
	task, err := dispatcher.DispatchWorkflowTask(
		context.Background(), orgID, executionID, "charge",
		map[string]any{
			"queue":       "payments",
			"payload":     map[string]any{"kind": "charge"},
			"max_retries": 3,
		},
		json.RawMessage(`{"fallback":true}`),
	)
	if err != nil || task == nil || created == nil {
		t.Fatalf("task=%+v created=%+v err=%v", task, created, err)
	}
	if created.Queue != "payments" ||
		created.IdempotencyKey != "workflow:"+executionID.String()+":charge" ||
		string(created.Payload) != `{"kind":"charge"}` ||
		created.MaxAttempts != 4 {
		t.Fatalf("created task = %+v payload=%s", created, created.Payload)
	}
}

func TestEventForgeWorkflowWebhookDispatcher(t *testing.T) {
	orgID, workflowID, executionID, endpointID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	endpoint := &domain.WebhookEndpoint{
		ID: endpointID, OrgID: orgID, IsActive: true,
	}
	deliveries := make(map[uuid.UUID]*domain.WebhookDelivery)
	endpoints := &testutil.WebhookEndpointRepositoryStub{
		GetFunc: func(_ context.Context, requestedOrgID, requestedID uuid.UUID) (*domain.WebhookEndpoint, error) {
			if requestedOrgID != orgID || requestedID != endpointID {
				return nil, domain.ErrWebhookEndpointNotFound
			}
			return endpoint, nil
		},
	}
	repository := &testutil.WebhookDeliveryRepositoryStub{
		CreateFunc: func(_ context.Context, delivery *domain.WebhookDelivery) error {
			deliveries[delivery.ID] = delivery
			return nil
		},
		GetFunc: func(_ context.Context, requestedOrgID, requestedID uuid.UUID) (*domain.WebhookDelivery, error) {
			delivery := deliveries[requestedID]
			if delivery == nil || requestedOrgID != orgID {
				return nil, domain.ErrWebhookDeliveryNotFound
			}
			return delivery, nil
		},
	}
	dispatcher := NewEventForgeWorkflowWebhookDispatcher(endpoints, repository, 4)
	execution := &domain.WorkflowExecution{
		ID: executionID, OrgID: orgID, WorkflowID: workflowID,
	}
	node := domain.WorkflowNode{
		ID: "notify", Type: domain.WorkflowNodeWebhook, Name: "Notify",
		Config: map[string]any{
			"endpoint_id": endpointID.String(),
			"payload":     map[string]any{"kind": "workflow"},
		},
	}
	first, err := dispatcher.DispatchWorkflowWebhook(
		context.Background(), execution, node, json.RawMessage(`{"fallback":true}`),
	)
	if err != nil || first == nil {
		t.Fatalf("first delivery = %+v err=%v", first, err)
	}
	second, err := dispatcher.DispatchWorkflowWebhook(
		context.Background(), execution, node, json.RawMessage(`{"fallback":true}`),
	)
	if err != nil || second.ID != first.ID || len(deliveries) != 1 {
		t.Fatalf("idempotent delivery = %+v count=%d err=%v", second, len(deliveries), err)
	}
	if first.EventType != domain.WebhookEventWorkflowNode ||
		first.Status != domain.WebhookDeliveryPending ||
		first.MaxAttempts != 4 {
		t.Fatalf("delivery = %+v", first)
	}
	var payload domain.WorkflowWebhookPayload
	if err := json.Unmarshal(first.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.WorkflowExecutionID != executionID || payload.NodeID != "notify" ||
		string(payload.Input) != `{"kind":"workflow"}` {
		t.Fatalf("payload = %+v raw=%s", payload, first.Payload)
	}

	endpoint.IsActive = false
	otherNode := node
	otherNode.ID = "inactive"
	if _, err := dispatcher.DispatchWorkflowWebhook(
		context.Background(), execution, otherNode, nil,
	); !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("inactive endpoint error = %v", err)
	}
}

func TestWorkflowDelayConfigBounds(t *testing.T) {
	if _, err := parseWorkflowDelayConfig(map[string]any{"duration_seconds": maxWorkflowDelaySeconds}); err != nil {
		t.Fatal(err)
	}
	if _, err := parseWorkflowDelayConfig(map[string]any{"duration_seconds": -1}); err == nil {
		t.Fatal("negative delay accepted")
	}
	if _, err := parseWorkflowDelayConfig(map[string]any{"duration_seconds": maxWorkflowDelaySeconds + 1}); err == nil {
		t.Fatal("oversized delay accepted")
	}
}
