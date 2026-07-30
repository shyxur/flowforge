package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/ports"
)

var (
	_ ports.WebhookEndpointRepository = (*PostgresStorage)(nil)
	_ ports.WebhookDeliveryRepository = (*PostgresStorage)(nil)
)

const webhookEndpointColumns = `
	id, org_id, name, url, secret_hash, secret_ciphertext, event_types, is_active,
	deleted_at, created_at, updated_at
`

func scanWebhookEndpoint(scanner interface{ Scan(...any) error }) (*domain.WebhookEndpoint, error) {
	var endpoint domain.WebhookEndpoint
	var eventTypes []string
	var secretCiphertext sql.NullString
	var deletedAt sql.NullTime
	if err := scanner.Scan(
		&endpoint.ID, &endpoint.OrgID, &endpoint.Name, &endpoint.URL,
		&endpoint.SecretHash, &secretCiphertext, &eventTypes, &endpoint.IsActive,
		&deletedAt, &endpoint.CreatedAt, &endpoint.UpdatedAt,
	); err != nil {
		return nil, err
	}
	endpoint.EventTypes = make([]domain.WebhookEventType, len(eventTypes))
	for i, eventType := range eventTypes {
		endpoint.EventTypes[i] = domain.WebhookEventType(eventType)
	}
	if secretCiphertext.Valid {
		endpoint.SecretCiphertext = secretCiphertext.String
	}
	if deletedAt.Valid {
		endpoint.DeletedAt = &deletedAt.Time
	}
	return &endpoint, nil
}

func webhookEventTypeStrings(eventTypes []domain.WebhookEventType) []string {
	values := make([]string, len(eventTypes))
	for i, eventType := range eventTypes {
		values[i] = string(eventType)
	}
	return values
}

func (s *PostgresStorage) CreateWebhookEndpoint(ctx context.Context, endpoint *domain.WebhookEndpoint) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_endpoints (
			id, org_id, name, url, secret_hash, secret_ciphertext, event_types, is_active,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, endpoint.ID, endpoint.OrgID, endpoint.Name, endpoint.URL, endpoint.SecretHash,
		endpoint.SecretCiphertext, webhookEventTypeStrings(endpoint.EventTypes), endpoint.IsActive,
		endpoint.CreatedAt, endpoint.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create webhook endpoint: %w", err)
	}
	return nil
}

func (s *PostgresStorage) ListWebhookEndpoints(ctx context.Context, orgID uuid.UUID) ([]*domain.WebhookEndpoint, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM webhook_endpoints
		WHERE org_id=$1 AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
	`, webhookEndpointColumns), orgID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list webhook endpoints: %w", err)
	}
	defer rows.Close()

	endpoints := make([]*domain.WebhookEndpoint, 0)
	for rows.Next() {
		endpoint, scanErr := scanWebhookEndpoint(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: list webhook endpoints scan: %w", scanErr)
		}
		endpoints = append(endpoints, endpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list webhook endpoints rows: %w", err)
	}
	return endpoints, nil
}

func (s *PostgresStorage) GetWebhookEndpoint(ctx context.Context, orgID, id uuid.UUID) (*domain.WebhookEndpoint, error) {
	endpoint, err := scanWebhookEndpoint(s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM webhook_endpoints
		WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL
	`, webhookEndpointColumns), orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrWebhookEndpointNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get webhook endpoint: %w", err)
	}
	return endpoint, nil
}

func (s *PostgresStorage) UpdateWebhookEndpoint(ctx context.Context, endpoint *domain.WebhookEndpoint) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE webhook_endpoints
		SET name=$1, url=$2, secret_hash=$3, secret_ciphertext=$4, event_types=$5,
			is_active=$6, updated_at=$7
		WHERE org_id=$8 AND id=$9 AND deleted_at IS NULL
	`, endpoint.Name, endpoint.URL, endpoint.SecretHash, endpoint.SecretCiphertext,
		webhookEventTypeStrings(endpoint.EventTypes), endpoint.IsActive,
		endpoint.UpdatedAt, endpoint.OrgID, endpoint.ID)
	if err != nil {
		return fmt.Errorf("postgres: update webhook endpoint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrWebhookEndpointNotFound
	}
	return nil
}

func (s *PostgresStorage) SoftDeleteWebhookEndpoint(ctx context.Context, orgID, id uuid.UUID, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE webhook_endpoints
		SET deleted_at=$1, is_active=false, updated_at=$1
		WHERE org_id=$2 AND id=$3 AND deleted_at IS NULL
	`, now, orgID, id)
	if err != nil {
		return fmt.Errorf("postgres: soft delete webhook endpoint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrWebhookEndpointNotFound
	}
	return nil
}

func (s *PostgresStorage) ListActiveWebhookEndpointsForEvent(
	ctx context.Context,
	orgID uuid.UUID,
	eventType domain.WebhookEventType,
) ([]*domain.WebhookEndpoint, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM webhook_endpoints
		WHERE org_id=$1 AND is_active=true AND deleted_at IS NULL
		  AND $2 = ANY(event_types)
		ORDER BY created_at ASC, id ASC
	`, webhookEndpointColumns), orgID, eventType)
	if err != nil {
		return nil, fmt.Errorf("postgres: list active webhook endpoints for event: %w", err)
	}
	defer rows.Close()

	endpoints := make([]*domain.WebhookEndpoint, 0)
	for rows.Next() {
		endpoint, scanErr := scanWebhookEndpoint(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: list active webhook endpoints for event scan: %w", scanErr)
		}
		endpoints = append(endpoints, endpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list active webhook endpoints for event rows: %w", err)
	}
	return endpoints, nil
}

const webhookDeliveryColumns = `
	id, org_id, endpoint_id, event_type, payload, status,
	attempt_count, max_attempts, next_attempt_at, last_attempt_at,
	response_status, response_body, last_error, created_at, updated_at
`

func scanWebhookDelivery(scanner interface{ Scan(...any) error }) (*domain.WebhookDelivery, error) {
	var delivery domain.WebhookDelivery
	var nextAttemptAt, lastAttemptAt sql.NullTime
	var responseStatus sql.NullInt32
	var responseBody, lastError sql.NullString
	if err := scanner.Scan(
		&delivery.ID, &delivery.OrgID, &delivery.EndpointID, &delivery.EventType,
		&delivery.Payload, &delivery.Status, &delivery.AttemptCount, &delivery.MaxAttempts,
		&nextAttemptAt, &lastAttemptAt, &responseStatus, &responseBody, &lastError,
		&delivery.CreatedAt, &delivery.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if nextAttemptAt.Valid {
		delivery.NextAttemptAt = &nextAttemptAt.Time
	}
	if lastAttemptAt.Valid {
		delivery.LastAttemptAt = &lastAttemptAt.Time
	}
	if responseStatus.Valid {
		value := int(responseStatus.Int32)
		delivery.ResponseStatus = &value
	}
	if responseBody.Valid {
		delivery.ResponseBody = &responseBody.String
	}
	if lastError.Valid {
		delivery.LastError = &lastError.String
	}
	return &delivery, nil
}

func (s *PostgresStorage) CreateWebhookDelivery(ctx context.Context, delivery *domain.WebhookDelivery) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (
			id, org_id, endpoint_id, event_type, payload, status,
			attempt_count, max_attempts, next_attempt_at, last_attempt_at,
			response_status, response_body, last_error, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, delivery.ID, delivery.OrgID, delivery.EndpointID, delivery.EventType,
		delivery.Payload, delivery.Status, delivery.AttemptCount, delivery.MaxAttempts,
		delivery.NextAttemptAt, delivery.LastAttemptAt, delivery.ResponseStatus,
		delivery.ResponseBody, delivery.LastError, delivery.CreatedAt, delivery.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create webhook delivery: %w", err)
	}
	return nil
}

func (s *PostgresStorage) GetWebhookDelivery(ctx context.Context, orgID, id uuid.UUID) (*domain.WebhookDelivery, error) {
	delivery, err := scanWebhookDelivery(s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM webhook_deliveries WHERE org_id=$1 AND id=$2
	`, webhookDeliveryColumns), orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrWebhookDeliveryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get webhook delivery: %w", err)
	}
	return delivery, nil
}

func (s *PostgresStorage) ListWebhookDeliveries(
	ctx context.Context,
	orgID uuid.UUID,
	filter domain.WebhookDeliveryFilter,
) (*domain.WebhookDeliveryPage, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	args := []any{orgID}
	var query strings.Builder
	fmt.Fprintf(&query, "SELECT %s FROM webhook_deliveries WHERE org_id=$1", webhookDeliveryColumns)
	if filter.EndpointID != nil {
		args = append(args, *filter.EndpointID)
		fmt.Fprintf(&query, " AND endpoint_id=$%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		fmt.Fprintf(&query, " AND status=$%d", len(args))
	}
	if filter.EventType != "" {
		args = append(args, filter.EventType)
		fmt.Fprintf(&query, " AND event_type=$%d", len(args))
	}
	if filter.Cursor != "" {
		cursorTime, cursorID, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, domain.ErrInvalidInput
		}
		args = append(args, cursorTime, cursorID)
		fmt.Fprintf(&query, " AND (created_at,id) < ($%d,$%d)", len(args)-1, len(args))
	}
	args = append(args, filter.Limit+1)
	fmt.Fprintf(&query, " ORDER BY created_at DESC,id DESC LIMIT $%d", len(args))

	rows, err := s.pool.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list webhook deliveries: %w", err)
	}
	defer rows.Close()
	page := &domain.WebhookDeliveryPage{}
	for rows.Next() {
		delivery, scanErr := scanWebhookDelivery(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: list webhook deliveries scan: %w", scanErr)
		}
		page.Deliveries = append(page.Deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list webhook deliveries rows: %w", err)
	}
	if len(page.Deliveries) > filter.Limit {
		page.Deliveries = page.Deliveries[:filter.Limit]
		last := page.Deliveries[len(page.Deliveries)-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s *PostgresStorage) ListDueWebhookDeliveries(ctx context.Context, now time.Time, limit int) ([]*domain.WebhookDelivery, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM webhook_deliveries
		WHERE status IN ('pending','retrying')
		  AND attempt_count < max_attempts
		  AND (next_attempt_at IS NULL OR next_attempt_at <= $1)
		ORDER BY next_attempt_at ASC NULLS FIRST, created_at ASC
		LIMIT $2
	`, webhookDeliveryColumns), now, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list due webhook deliveries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]*domain.WebhookDelivery, 0)
	for rows.Next() {
		delivery, scanErr := scanWebhookDelivery(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: list due webhook deliveries scan: %w", scanErr)
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list due webhook deliveries rows: %w", err)
	}
	return deliveries, nil
}

func (s *PostgresStorage) ClaimDueWebhookDeliveries(ctx context.Context, now time.Time, limit int) ([]*domain.WebhookDelivery, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		WITH candidates AS (
			SELECT id
			FROM webhook_deliveries
			WHERE status IN ('pending','retrying')
			  AND attempt_count < max_attempts
			  AND (next_attempt_at IS NULL OR next_attempt_at <= $1)
			ORDER BY next_attempt_at ASC NULLS FIRST, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		UPDATE webhook_deliveries AS delivery
		SET status='delivering',
			attempt_count=delivery.attempt_count+1,
			last_attempt_at=$1,
			next_attempt_at=NULL,
			updated_at=$1
		FROM candidates
		WHERE delivery.id=candidates.id
		RETURNING %s
	`, prefixedWebhookDeliveryColumns("delivery")), now, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: claim due webhook deliveries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]*domain.WebhookDelivery, 0)
	for rows.Next() {
		delivery, scanErr := scanWebhookDelivery(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: claim due webhook deliveries scan: %w", scanErr)
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: claim due webhook deliveries rows: %w", err)
	}
	return deliveries, nil
}

func prefixedWebhookDeliveryColumns(prefix string) string {
	return `
		` + prefix + `.id, ` + prefix + `.org_id, ` + prefix + `.endpoint_id,
		` + prefix + `.event_type, ` + prefix + `.payload, ` + prefix + `.status,
		` + prefix + `.attempt_count, ` + prefix + `.max_attempts,
		` + prefix + `.next_attempt_at, ` + prefix + `.last_attempt_at,
		` + prefix + `.response_status, ` + prefix + `.response_body,
		` + prefix + `.last_error, ` + prefix + `.created_at, ` + prefix + `.updated_at
	`
}

func (s *PostgresStorage) UpdateWebhookDelivery(ctx context.Context, delivery *domain.WebhookDelivery) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status=$1, attempt_count=$2, max_attempts=$3,
			next_attempt_at=$4, last_attempt_at=$5, response_status=$6,
			response_body=$7, last_error=$8, updated_at=$9
		WHERE org_id=$10 AND id=$11
	`, delivery.Status, delivery.AttemptCount, delivery.MaxAttempts,
		delivery.NextAttemptAt, delivery.LastAttemptAt, delivery.ResponseStatus,
		delivery.ResponseBody, delivery.LastError, delivery.UpdatedAt,
		delivery.OrgID, delivery.ID)
	if err != nil {
		return fmt.Errorf("postgres: update webhook delivery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrWebhookDeliveryNotFound
	}
	return nil
}

func (s *PostgresStorage) RetryWebhookDelivery(
	ctx context.Context,
	orgID, id uuid.UUID,
	now time.Time,
) (*domain.WebhookDelivery, error) {
	delivery, err := scanWebhookDelivery(s.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE webhook_deliveries
		SET status='pending', attempt_count=0, next_attempt_at=$1,
			last_attempt_at=NULL, response_status=NULL, response_body=NULL,
			last_error=NULL, updated_at=$1
		WHERE org_id=$2 AND id=$3 AND status IN ('failed','retrying')
		RETURNING %s
	`, webhookDeliveryColumns), now, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := s.GetWebhookDelivery(ctx, orgID, id)
		if errors.Is(getErr, domain.ErrWebhookDeliveryNotFound) {
			return nil, domain.ErrWebhookDeliveryNotFound
		}
		if getErr != nil {
			return nil, getErr
		}
		if existing != nil {
			return nil, domain.ErrInvalidStateTransition
		}
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: retry webhook delivery: %w", err)
	}
	return delivery, nil
}
