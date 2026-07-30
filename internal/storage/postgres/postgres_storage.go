package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/ports"
)

type PostgresStorage struct {
	pool *pgxpool.Pool
}

var _ ports.Storage = (*PostgresStorage)(nil)

func NewPostgresStorage(ctx context.Context, dsn string) (*PostgresStorage, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	return &PostgresStorage{pool: pool}, nil
}

func (s *PostgresStorage) Create(ctx context.Context, task *domain.Task) error {
	const q = `
			INSERT INTO tasks (id, org_id, queue, payload, status, idempotency_key,
				request_fingerprint, priority, backoff_strategy, task_timeout_ms,
				attempts, max_attempts, visibility_timeout_ms, visible_at,
				created_at, updated_at, scheduled_at, trace_id)
			VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NULLIF($18,''))
		`
	_, err := s.pool.Exec(ctx, q,
		task.ID, task.OrgID, task.Queue, task.Payload, task.Status, task.IdempotencyKey,
		task.RequestFingerprint, task.Priority, task.BackoffStrategy, task.TaskTimeout.Milliseconds(),
		task.Attempts, task.MaxAttempts, task.VisibilityTimeout.Milliseconds(),
		task.VisibleAt, task.CreatedAt, task.UpdatedAt, task.ScheduledAt, task.TraceID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return domain.ErrDuplicateIdempotencyKey
		}
		return fmt.Errorf("postgres: create task: %w", err)
	}
	return nil
}

func (s *PostgresStorage) GetByID(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM tasks WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL`, taskColumns), orgID, id)
	t, err := scanTaskRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get task: %w", err)
	}
	return t, nil
}

func (s *PostgresStorage) FindByIdempotencyKey(ctx context.Context, orgID uuid.UUID, queue, key string) (*domain.Task, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM tasks WHERE org_id=$1 AND queue=$2 AND idempotency_key=$3`, taskColumns),
		orgID, queue, key)
	t, err := scanTaskRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find by idempotency key: %w", err)
	}
	return t, nil
}

// ClaimForProcessing uses SELECT ... FOR UPDATE SKIP LOCKED so concurrent
// workers never double-claim the same row, and never block on contention
// (skip to next candidate instead of waiting).
func (s *PostgresStorage) ClaimForProcessing(ctx context.Context, orgID, id uuid.UUID, workerID string, now time.Time) (*domain.Task, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: claim begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, fmt.Sprintf(`
			SELECT %s FROM tasks
			WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL
			  AND visible_at <= $3
			  AND (status='pending' OR (status='processing' AND visible_at < $3))
			FOR UPDATE SKIP LOCKED
		`, taskColumns), orgID, id, now)

	t, err := scanTaskRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTaskAlreadyLocked
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: claim select: %w", err)
	}

	t.MarkProcessing(workerID, now)

	_, err = tx.Exec(ctx, `
		UPDATE tasks SET status=$1, locked_by=$2, attempts=$3, visible_at=$4,
			last_heartbeat_at=$5, updated_at=$6
		WHERE org_id=$7 AND id=$8
	`, t.Status, t.LockedBy, t.Attempts, t.VisibleAt, t.LastHeartbeatAt, now, orgID, t.ID)
	if err != nil {
		return nil, fmt.Errorf("postgres: claim update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("postgres: claim commit: %w", err)
	}
	return t, nil
}

func (s *PostgresStorage) Heartbeat(ctx context.Context, orgID, id uuid.UUID, workerID string, visibilityTimeout time.Duration, now time.Time) error {
	visibleAt := now.Add(visibilityTimeout)
	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks SET visible_at=$1, last_heartbeat_at=$2, updated_at=$2
			WHERE org_id=$3 AND id=$4 AND locked_by=$5 AND status='processing'
		`, visibleAt, now, orgID, id, workerID)
	if err != nil {
		return fmt.Errorf("postgres: heartbeat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTaskAlreadyLocked
	}
	return nil
}

func (s *PostgresStorage) Complete(ctx context.Context, orgID, id uuid.UUID, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
			UPDATE tasks SET status='completed', completed_at=$1, locked_by=NULL,
				last_heartbeat_at=NULL, updated_at=$1
			WHERE org_id=$2 AND id=$3 AND status='processing'
		`, now, orgID, id)
	if err != nil {
		return fmt.Errorf("postgres: complete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

func (s *PostgresStorage) Fail(ctx context.Context, orgID, id uuid.UUID, errMsg string, nextStatus domain.TaskStatus, visibleAt time.Time, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks SET status=$1, last_error=$2, visible_at=$3, locked_by=NULL,
			last_heartbeat_at=NULL, dispatched_at=NULL, updated_at=$4
			WHERE org_id=$5 AND id=$6 AND status='processing'
		`, nextStatus, errMsg, visibleAt, now, orgID, id)
	if err != nil {
		return fmt.Errorf("postgres: fail: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

func (s *PostgresStorage) MoveToDeadLetter(ctx context.Context, orgID, id uuid.UUID, errMsg string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks SET status='dead_letter', last_error=$1, locked_by=NULL,
			last_heartbeat_at=NULL, updated_at=$2
			WHERE org_id=$3 AND id=$4 AND status='processing'
		`, errMsg, now, orgID, id)
	if err != nil {
		return fmt.Errorf("postgres: move to dlq: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

// ReclaimExpired finds processing tasks whose visible_at has lapsed (crashed
// worker never heartbeat'd in time) and atomically resets them to pending.
// FOR UPDATE SKIP LOCKED avoids stepping on concurrent reclaimers.
func (s *PostgresStorage) ReclaimExpired(ctx context.Context, orgID uuid.UUID, queue string, now time.Time, limit int) ([]*domain.Task, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: reclaim begin: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM tasks
			WHERE org_id=$1 AND queue=$2 AND status='processing' AND visible_at < $3
			ORDER BY visible_at ASC
			LIMIT $4
			FOR UPDATE SKIP LOCKED
		`, taskColumns), orgID, queue, now, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: reclaim select: %w", err)
	}

	var reclaimed []*domain.Task
	var ids []uuid.UUID
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("postgres: reclaim scan: %w", err)
		}
		reclaimed = append(reclaimed, t)
		ids = append(ids, t.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, tx.Commit(ctx)
	}

	_, err = tx.Exec(ctx, `
			UPDATE tasks SET status='pending', locked_by=NULL, last_heartbeat_at=NULL,
				visible_at=$1, dispatched_at=NULL, updated_at=$1
			WHERE org_id=$2 AND id = ANY($3)
		`, now, orgID, ids)
	if err != nil {
		return nil, fmt.Errorf("postgres: reclaim update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("postgres: reclaim commit: %w", err)
	}
	return reclaimed, nil
}

func (s *PostgresStorage) ListDeadLetter(ctx context.Context, orgID uuid.UUID, queue, cursor string, limit int) (*domain.TaskPage, error) {
	return s.ListTasks(ctx, orgID, domain.TaskFilter{
		Queue: queue, Status: domain.StatusDeadLetter, Cursor: cursor, Limit: limit,
	})
}

func (s *PostgresStorage) Requeue(ctx context.Context, orgID, id uuid.UUID, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
			UPDATE tasks SET status='pending', attempts=0, locked_by=NULL,
				last_heartbeat_at=NULL, last_error=NULL, visible_at=$1,
				dispatched_at=NULL, completed_at=NULL, updated_at=$1
			WHERE org_id=$2 AND id=$3
			  AND status IN ('failed','dead_letter','cancelled')
			  AND deleted_at IS NULL
		`, now, orgID, id)
	if err != nil {
		return fmt.Errorf("postgres: requeue: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrInvalidStateTransition
	}
	return nil
}

func (s *PostgresStorage) ListTasks(ctx context.Context, orgID uuid.UUID, filter domain.TaskFilter) (*domain.TaskPage, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}

	args := []any{orgID}
	var q strings.Builder
	fmt.Fprintf(&q, "SELECT %s FROM tasks WHERE org_id=$1 AND deleted_at IS NULL", taskColumns)
	if filter.Queue != "" {
		args = append(args, filter.Queue)
		fmt.Fprintf(&q, " AND queue=$%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		fmt.Fprintf(&q, " AND status=$%d", len(args))
	}
	if filter.Cursor != "" {
		cursorTime, cursorID, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, domain.ErrInvalidInput
		}
		args = append(args, cursorTime, cursorID)
		fmt.Fprintf(&q, " AND (created_at,id) < ($%d,$%d)", len(args)-1, len(args))
	}
	args = append(args, filter.Limit+1)
	fmt.Fprintf(&q, " ORDER BY created_at DESC,id DESC LIMIT $%d", len(args))

	rows, err := s.pool.Query(ctx, q.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list tasks: %w", err)
	}
	defer rows.Close()

	page := &domain.TaskPage{}
	for rows.Next() {
		task, scanErr := scanTaskRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: list tasks scan: %w", scanErr)
		}
		page.Tasks = append(page.Tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list tasks rows: %w", err)
	}
	if len(page.Tasks) > filter.Limit {
		page.Tasks = page.Tasks[:filter.Limit]
		last := page.Tasks[len(page.Tasks)-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s *PostgresStorage) Cancel(ctx context.Context, orgID, id uuid.UUID, now time.Time) (*domain.Task, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE tasks SET status='cancelled', locked_by=NULL, last_heartbeat_at=NULL,
			updated_at=$1
		WHERE org_id=$2 AND id=$3 AND status='pending' AND deleted_at IS NULL
		RETURNING %s
	`, taskColumns), now, orgID, id)
	task, err := scanTaskRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrInvalidStateTransition
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: cancel task: %w", err)
	}
	return task, nil
}

func (s *PostgresStorage) SoftDelete(ctx context.Context, orgID, id uuid.UUID, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks SET deleted_at=$1, updated_at=$1
		WHERE org_id=$2 AND id=$3 AND deleted_at IS NULL
		  AND status IN ('completed','failed','dead_letter','cancelled')
	`, now, orgID, id)
	if err != nil {
		return fmt.Errorf("postgres: soft delete task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrInvalidStateTransition
	}
	return nil
}

func (s *PostgresStorage) QueueStats(ctx context.Context, orgID uuid.UUID, queue string) (*domain.QueueStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT status, count(*) FROM tasks
		WHERE org_id=$1 AND queue=$2 AND deleted_at IS NULL
		GROUP BY status
	`, orgID, queue)
	if err != nil {
		return nil, fmt.Errorf("postgres: queue stats: %w", err)
	}
	defer rows.Close()
	stats := &domain.QueueStats{Queue: queue, ByStatus: make(map[string]int64)}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("postgres: queue stats scan: %w", err)
		}
		stats.ByStatus[status] = count
		stats.Total += count
	}
	return stats, rows.Err()
}

func (s *PostgresStorage) FindAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	var key domain.APIKey
	var revokedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT org_id, scopes, revoked_at
		FROM api_keys WHERE key_hash=$1
	`, keyHash).Scan(&key.OrgID, &key.Scopes, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find api key: %w", err)
	}
	key.RevokedAt = revokedAt
	return &key, nil
}

func (s *PostgresStorage) MarkDispatched(ctx context.Context, orgID, id uuid.UUID, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks SET dispatched_at=$1, updated_at=$1
		WHERE org_id=$2 AND id=$3 AND status='pending'
	`, now, orgID, id)
	if err != nil {
		return fmt.Errorf("postgres: mark dispatched: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

func (s *PostgresStorage) ListUndispatchedPending(ctx context.Context, orgID uuid.UUID, queue string, limit int) ([]*domain.Task, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM tasks
		WHERE org_id=$1 AND queue=$2 AND status='pending'
		  AND dispatched_at IS NULL AND visible_at <= now() AND deleted_at IS NULL
		ORDER BY created_at ASC LIMIT $3
	`, taskColumns), orgID, queue, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list undispatched: %w", err)
	}
	defer rows.Close()
	var tasks []*domain.Task
	for rows.Next() {
		task, scanErr := scanTaskRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: list undispatched scan: %w", scanErr)
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *PostgresStorage) ListWorkers(ctx context.Context, orgID uuid.UUID) ([]*domain.Worker, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT worker_id, org_id, queue, status, last_heartbeat_at, created_at, updated_at
		FROM workers WHERE org_id=$1 ORDER BY worker_id
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list workers: %w", err)
	}
	defer rows.Close()
	var workers []*domain.Worker
	for rows.Next() {
		var worker domain.Worker
		if err := rows.Scan(&worker.ID, &worker.OrgID, &worker.Queue, &worker.Status,
			&worker.LastHeartbeatAt, &worker.CreatedAt, &worker.UpdatedAt); err != nil {
			return nil, fmt.Errorf("postgres: list workers scan: %w", err)
		}
		workers = append(workers, &worker)
	}
	return workers, rows.Err()
}

func (s *PostgresStorage) UpsertWorkerHeartbeat(ctx context.Context, worker *domain.Worker) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO workers (org_id, worker_id, queue, status, last_heartbeat_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$6)
		ON CONFLICT (org_id,worker_id) DO UPDATE SET
			queue=EXCLUDED.queue, status=EXCLUDED.status,
			last_heartbeat_at=EXCLUDED.last_heartbeat_at, updated_at=EXCLUDED.updated_at
	`, worker.OrgID, worker.ID, worker.Queue, worker.Status, worker.LastHeartbeatAt, worker.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: worker heartbeat: %w", err)
	}
	return nil
}

func (s *PostgresStorage) ListActiveQueueScopes(ctx context.Context) ([]domain.QueueScope, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT org_id, queue
		FROM tasks
		WHERE deleted_at IS NULL AND status IN ('pending','processing')
		GROUP BY org_id, queue
		ORDER BY org_id, queue
	`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list active queue scopes: %w", err)
	}
	defer rows.Close()
	var scopes []domain.QueueScope
	for rows.Next() {
		var scope domain.QueueScope
		if err := rows.Scan(&scope.OrgID, &scope.Queue); err != nil {
			return nil, fmt.Errorf("postgres: list active queue scopes scan: %w", err)
		}
		scopes = append(scopes, scope)
	}
	return scopes, rows.Err()
}

func (s *PostgresStorage) ListDispatchableTasks(ctx context.Context, afterID uuid.UUID, limit int) ([]*domain.Task, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM tasks
		WHERE status='pending' AND deleted_at IS NULL AND id > $1
		ORDER BY id ASC
		LIMIT $2
	`, taskColumns), afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list dispatchable tasks: %w", err)
	}
	defer rows.Close()
	var tasks []*domain.Task
	for rows.Next() {
		task, err := scanTaskRow(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: list dispatchable tasks scan: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func encodeCursor(createdAt time.Time, id uuid.UUID) string {
	value := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, domain.ErrInvalidInput
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(parts[1])
	return createdAt, id, err
}

func (s *PostgresStorage) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *PostgresStorage) Close() error {
	s.pool.Close()
	return nil
}
