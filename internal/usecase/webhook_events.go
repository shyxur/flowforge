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

type WebhookEventService struct {
	endpoints   ports.WebhookEndpointRepository
	deliveries  ports.WebhookDeliveryRepository
	maxAttempts int
}

func NewWebhookEventService(
	endpoints ports.WebhookEndpointRepository,
	deliveries ports.WebhookDeliveryRepository,
	maxAttempts int,
) *WebhookEventService {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	return &WebhookEventService{
		endpoints: endpoints, deliveries: deliveries, maxAttempts: maxAttempts,
	}
}

func (service *WebhookEventService) PublishTaskEvent(
	ctx context.Context,
	eventType domain.WebhookEventType,
	task *domain.Task,
) error {
	if task == nil || !eventType.Valid() {
		return domain.ErrInvalidInput
	}
	endpoints, err := service.endpoints.ListActiveWebhookEndpointsForEvent(ctx, task.OrgID, eventType)
	if err != nil {
		return err
	}
	if len(endpoints) == 0 {
		return nil
	}

	now := time.Now().UTC()
	payload, err := json.Marshal(domain.WebhookEventPayload{
		EventID: uuid.New(), EventType: eventType, CreatedAt: now, OrgID: task.OrgID,
		Task: domain.WebhookTaskSummary{
			ID: task.ID, Queue: task.Queue, Status: task.Status, Priority: task.Priority,
			Attempts: task.Attempts, MaxAttempts: task.MaxAttempts, LastError: task.LastError,
			CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal webhook event: %w", err)
	}

	var publishErrors []error
	for _, endpoint := range endpoints {
		delivery := &domain.WebhookDelivery{
			ID: uuid.New(), OrgID: task.OrgID, EndpointID: endpoint.ID,
			EventType: eventType, Payload: payload, Status: domain.WebhookDeliveryPending,
			MaxAttempts: service.maxAttempts, CreatedAt: now, UpdatedAt: now,
		}
		if err := service.deliveries.CreateWebhookDelivery(ctx, delivery); err != nil {
			publishErrors = append(publishErrors, err)
		}
	}
	return errors.Join(publishErrors...)
}
