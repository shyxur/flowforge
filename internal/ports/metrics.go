package ports

import (
	"context"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
)

type MetricRepository interface {
	AppendMetricEvent(context.Context, domain.MetricEvent) error
	AppendMetricEvents(context.Context, []domain.MetricEvent) error
	ListMetricEvents(context.Context, uuid.UUID, domain.MetricEventFilter) (*domain.MetricEventPage, error)
}

// MetricRecorder is deliberately non-blocking and has no error return:
// observability failures must not alter business lifecycle outcomes.
type MetricRecorder interface {
	RecordMetric(domain.MetricEvent)
}
