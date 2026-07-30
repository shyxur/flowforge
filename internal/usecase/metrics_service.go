package usecase

import (
	"context"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/ports"
)

type MetricsService struct {
	repository ports.MetricRepository
}

func NewMetricsService(repository ports.MetricRepository) *MetricsService {
	return &MetricsService{repository: repository}
}

func (service *MetricsService) AppendMetricEvent(ctx context.Context, event domain.MetricEvent) error {
	return service.repository.AppendMetricEvent(ctx, event)
}

func (service *MetricsService) AppendMetricEvents(ctx context.Context, events []domain.MetricEvent) error {
	return service.repository.AppendMetricEvents(ctx, events)
}

func (service *MetricsService) ListMetricEvents(
	ctx context.Context,
	orgID uuid.UUID,
	filter domain.MetricEventFilter,
) (*domain.MetricEventPage, error) {
	return service.repository.ListMetricEvents(ctx, orgID, filter)
}
