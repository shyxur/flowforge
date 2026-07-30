package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/ports"
)

type WebhookDeliveryLogService struct {
	deliveries ports.WebhookDeliveryRepository
}

func NewWebhookDeliveryLogService(deliveries ports.WebhookDeliveryRepository) *WebhookDeliveryLogService {
	return &WebhookDeliveryLogService{deliveries: deliveries}
}

func (service *WebhookDeliveryLogService) ListDeliveries(
	ctx context.Context,
	orgID uuid.UUID,
	filter domain.WebhookDeliveryFilter,
) (*domain.WebhookDeliveryPage, error) {
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, domain.ErrInvalidInput
	}
	if filter.EventType != "" && !filter.EventType.Valid() {
		return nil, domain.ErrInvalidInput
	}
	return service.deliveries.ListWebhookDeliveries(ctx, orgID, filter)
}

func (service *WebhookDeliveryLogService) GetDelivery(
	ctx context.Context,
	orgID, id uuid.UUID,
) (*domain.WebhookDelivery, error) {
	return service.deliveries.GetWebhookDelivery(ctx, orgID, id)
}

func (service *WebhookDeliveryLogService) RetryDelivery(
	ctx context.Context,
	orgID, id uuid.UUID,
) (*domain.WebhookDelivery, error) {
	delivery, err := service.deliveries.GetWebhookDelivery(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if delivery.Status != domain.WebhookDeliveryFailed &&
		delivery.Status != domain.WebhookDeliveryRetrying {
		return nil, domain.ErrInvalidStateTransition
	}
	return service.deliveries.RetryWebhookDelivery(ctx, orgID, id, time.Now().UTC())
}
