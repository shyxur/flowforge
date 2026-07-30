package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/ports"
)

var _ ports.MetricRepository = (*PostgresStorage)(nil)

const appendMetricEventSQL = `
	INSERT INTO metric_events (
		id, org_id, source, event_type, resource_type, resource_id,
		queue, status, duration_ms, occurred_at, metadata, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10,$11,$12)
	ON CONFLICT (id) DO NOTHING
`

func (s *PostgresStorage) AppendMetricEvent(ctx context.Context, event domain.MetricEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return domain.ErrInvalidInput
	}
	_, err = s.pool.Exec(ctx, appendMetricEventSQL, metricEventArgs(event, metadata)...)
	if err != nil {
		return fmt.Errorf("postgres: append metric event: %w", err)
	}
	return nil
}

func (s *PostgresStorage) AppendMetricEvents(ctx context.Context, events []domain.MetricEvent) error {
	if len(events) == 0 {
		return nil
	}
	if len(events) > 1000 {
		return domain.ErrInvalidInput
	}
	batch := &pgx.Batch{}
	for _, event := range events {
		if err := event.Validate(); err != nil {
			return err
		}
		metadata, err := json.Marshal(event.Metadata)
		if err != nil {
			return domain.ErrInvalidInput
		}
		batch.Queue(appendMetricEventSQL, metricEventArgs(event, metadata)...)
	}
	results := s.pool.SendBatch(ctx, batch)
	for range events {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("postgres: append metric event batch: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("postgres: close metric event batch: %w", err)
	}
	return nil
}

func metricEventArgs(event domain.MetricEvent, metadata []byte) []any {
	return []any{
		event.ID, event.OrganizationID, event.Source, event.EventType,
		event.ResourceType, event.ResourceID, event.Queue, event.Status,
		event.DurationMS, event.OccurredAt, metadata, event.CreatedAt,
	}
}

func (s *PostgresStorage) ListMetricEvents(
	ctx context.Context,
	orgID uuid.UUID,
	filter domain.MetricEventFilter,
) (*domain.MetricEventPage, error) {
	if orgID == uuid.Nil {
		return nil, domain.ErrInvalidInput
	}
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	conditions := []string{"org_id=$1", "occurred_at >= $2", "occurred_at < $3"}
	args := []any{orgID, filter.From, filter.To}
	if filter.Source != "" {
		args = append(args, filter.Source)
		conditions = append(conditions, fmt.Sprintf("source=$%d", len(args)))
	}
	if filter.EventType != "" {
		args = append(args, filter.EventType)
		conditions = append(conditions, fmt.Sprintf("event_type=$%d", len(args)))
	}
	if filter.Cursor != "" {
		occurredAt, id, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, domain.ErrInvalidInput
		}
		args = append(args, occurredAt, id)
		conditions = append(conditions, fmt.Sprintf(
			"(occurred_at, id) < ($%d, $%d)", len(args)-1, len(args),
		))
	}
	args = append(args, filter.Limit+1)
	query := `
		SELECT id, org_id, source, event_type, resource_type, resource_id,
			COALESCE(queue,''), COALESCE(status,''), duration_ms, occurred_at,
			metadata, created_at
		FROM metric_events
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY occurred_at DESC, id DESC
		LIMIT $` + fmt.Sprint(len(args))
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list metric events: %w", err)
	}
	defer rows.Close()
	items := make([]domain.MetricEvent, 0, filter.Limit+1)
	for rows.Next() {
		var event domain.MetricEvent
		var metadata []byte
		if err := rows.Scan(
			&event.ID, &event.OrganizationID, &event.Source, &event.EventType,
			&event.ResourceType, &event.ResourceID, &event.Queue, &event.Status,
			&event.DurationMS, &event.OccurredAt, &metadata, &event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan metric event: %w", err)
		}
		if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
			return nil, fmt.Errorf("postgres: decode metric metadata: %w", err)
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list metric events rows: %w", err)
	}
	page := &domain.MetricEventPage{Items: items}
	if len(items) > filter.Limit {
		last := items[filter.Limit-1]
		page.Items = items[:filter.Limit]
		page.NextCursor = encodeCursor(last.OccurredAt, last.ID)
	}
	return page, nil
}
