package usecase

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/testutil"
)

func TestTaskLifecycleEventCreatesTenantScopedDeliveriesForMatchingActiveEndpoints(t *testing.T) {
	orgID, otherOrgID := uuid.New(), uuid.New()
	matchingID, inactiveID, otherEventID, otherOrgEndpointID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	endpoints := []*domain.WebhookEndpoint{
		{ID: matchingID, OrgID: orgID, IsActive: true, EventTypes: []domain.WebhookEventType{domain.WebhookEventTaskCreated}},
		{ID: inactiveID, OrgID: orgID, IsActive: false, EventTypes: []domain.WebhookEventType{domain.WebhookEventTaskCreated}},
		{ID: otherEventID, OrgID: orgID, IsActive: true, EventTypes: []domain.WebhookEventType{domain.WebhookEventTaskFailed}},
		{ID: otherOrgEndpointID, OrgID: otherOrgID, IsActive: true, EventTypes: []domain.WebhookEventType{domain.WebhookEventTaskCreated}},
	}
	endpointRepository := &testutil.WebhookEndpointRepositoryStub{
		ListActiveForEventFunc: func(_ context.Context, gotOrgID uuid.UUID, eventType domain.WebhookEventType) ([]*domain.WebhookEndpoint, error) {
			var matched []*domain.WebhookEndpoint
			for _, endpoint := range endpoints {
				if endpoint.OrgID != gotOrgID || !endpoint.IsActive {
					continue
				}
				for _, subscribedEvent := range endpoint.EventTypes {
					if subscribedEvent == eventType {
						matched = append(matched, endpoint)
					}
				}
			}
			return matched, nil
		},
	}
	var deliveries []*domain.WebhookDelivery
	deliveryRepository := &testutil.WebhookDeliveryRepositoryStub{
		CreateFunc: func(_ context.Context, delivery *domain.WebhookDelivery) error {
			deliveries = append(deliveries, delivery)
			return nil
		},
	}
	service := NewWebhookEventService(endpointRepository, deliveryRepository, 5)
	task := domain.NewTask(orgID, "email", json.RawMessage(`{"x":1}`), "key", "fingerprint", 3, 0)

	if err := service.PublishTaskEvent(context.Background(), domain.WebhookEventTaskCreated, task); err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(deliveries))
	}
	delivery := deliveries[0]
	if delivery.OrgID != orgID || delivery.EndpointID != matchingID ||
		delivery.EventType != domain.WebhookEventTaskCreated ||
		delivery.Status != domain.WebhookDeliveryPending {
		t.Fatalf("delivery = %+v", delivery)
	}
	var payload domain.WebhookEventPayload
	if err := json.Unmarshal(delivery.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.OrgID != orgID || payload.Task.ID != task.ID ||
		payload.EventType != domain.WebhookEventTaskCreated || payload.EventID == uuid.Nil {
		t.Fatalf("payload = %+v", payload)
	}
}
