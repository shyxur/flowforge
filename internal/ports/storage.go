package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
)

// Storage abstracts durable persistence of task state (Postgres, etc.).
// This is the source of truth; Broker is transport/signaling only.
type Storage interface {
	// Create persists a new task. Must return domain.ErrDuplicateIdempotencyKey
	// if IdempotencyKey is non-empty and already exists for that queue
	// (unique constraint enforced at storage level, not app level, to avoid
	// race conditions under concurrent producers).
	Create(ctx context.Context, task *domain.Task) error

	// GetByID fetches a task; domain.ErrTaskNotFound if absent.
	GetByID(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error)

	// FindByIdempotencyKey supports producer-side dedupe checks pre-insert.
	FindByIdempotencyKey(ctx context.Context, orgID uuid.UUID, queue, key string) (*domain.Task, error)

	// ClaimForProcessing atomically transitions a task from pending/expired
	// to processing, setting LockedBy + VisibleAt. Must use SELECT ... FOR
	// UPDATE SKIP LOCKED (or equivalent) to guarantee exactly-one-worker
	// semantics under concurrency.
	ClaimForProcessing(ctx context.Context, orgID, id uuid.UUID, workerID string, now time.Time) (*domain.Task, error)

	// Heartbeat extends VisibleAt for a task the worker still owns, using the
	// given visibility timeout duration (not hardcoded). Returns
	// domain.ErrTaskAlreadyLocked if ownership was reassigned (lost lock).
	Heartbeat(ctx context.Context, orgID, id uuid.UUID, workerID string, visibilityTimeout time.Duration, now time.Time) error

	// Complete marks task as finished successfully.
	Complete(ctx context.Context, orgID, id uuid.UUID, now time.Time) error

	// Fail records a failed attempt with error detail. Caller (engine)
	// decides beforehand whether this leads to retry or DLQ; storage just
	// persists the resulting status transition.
	Fail(ctx context.Context, orgID, id uuid.UUID, errMsg string, nextStatus domain.TaskStatus, visibleAt time.Time, now time.Time) error

	// MoveToDeadLetter marks a task as permanently failed.
	MoveToDeadLetter(ctx context.Context, orgID, id uuid.UUID, errMsg string, now time.Time) error

	// ReclaimExpired finds tasks whose visibility timeout has lapsed while
	// still "processing" (crashed worker) and resets them to pending.
	// Returns reclaimed task IDs so the caller can re-enqueue on the broker.
	ReclaimExpired(ctx context.Context, orgID uuid.UUID, queue string, now time.Time, limit int) ([]*domain.Task, error)

	// ListDeadLetter supports DLQ inspection/replay tooling.
	ListDeadLetter(ctx context.Context, orgID uuid.UUID, queue, cursor string, limit int) (*domain.TaskPage, error)

	// Requeue takes a DLQ (or any) task and resets it to pending with fresh
	// attempt count — used for manual DLQ replay.
	Requeue(ctx context.Context, orgID, id uuid.UUID, now time.Time) error

	ListTasks(ctx context.Context, orgID uuid.UUID, filter domain.TaskFilter) (*domain.TaskPage, error)
	Cancel(ctx context.Context, orgID, id uuid.UUID, now time.Time) (*domain.Task, error)
	SoftDelete(ctx context.Context, orgID, id uuid.UUID, now time.Time) error
	QueueStats(ctx context.Context, orgID uuid.UUID, queue string) (*domain.QueueStats, error)

	FindAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error)

	MarkDispatched(ctx context.Context, orgID, id uuid.UUID, now time.Time) error
	ListUndispatchedPending(ctx context.Context, orgID uuid.UUID, queue string, limit int) ([]*domain.Task, error)

	ListWorkers(ctx context.Context, orgID uuid.UUID) ([]*domain.Worker, error)
	UpsertWorkerHeartbeat(ctx context.Context, worker *domain.Worker) error
	ListActiveQueueScopes(ctx context.Context) ([]domain.QueueScope, error)
	ListDispatchableTasks(ctx context.Context, afterID uuid.UUID, limit int) ([]*domain.Task, error)

	// Ping verifies connectivity (health checks).
	Ping(ctx context.Context) error

	// Close releases underlying connections gracefully.
	Close() error
}

// UnitOfWork allows callers (rare cases, e.g. producer needing to write
// task + outbox event atomically) to wrap multiple Storage calls in a
// single transaction. Optional — most flows use Storage methods directly.
type UnitOfWork interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
