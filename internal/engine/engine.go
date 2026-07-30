package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/distributed-task-queue/internal/domain"
	"github.com/shyxur/distributed-task-queue/internal/ports"
	"go.uber.org/zap"
)

// Engine owns retry/backoff/timeout/DLQ decision logic. It is broker- and
// storage-agnostic beyond the ports interfaces — worker pool drives it.
type Engine struct {
	storage     ports.Storage
	broker      ports.Broker
	retryPolicy domain.RetryPolicy
	taskTimeout time.Duration // per-task execution deadline
	logger      *zap.Logger
}

func NewEngine(storage ports.Storage, broker ports.Broker, retryPolicy domain.RetryPolicy, taskTimeout time.Duration, logger *zap.Logger) *Engine {
	return &Engine{
		storage:     storage,
		broker:      broker,
		retryPolicy: retryPolicy,
		taskTimeout: taskTimeout,
		logger:      logger,
	}
}

// Result summarizes execution outcome for metrics/logging by the caller.
type Result struct {
	TaskID  uuid.UUID
	Outcome string // "completed" | "retried" | "dead_letter"
	Err     error
}

// Execute claims-independent: assumes task is already claimed (status=processing,
// locked_by=workerID). Runs the handler under a timeout, then applies
// retry/backoff/DLQ policy based on outcome.
func (e *Engine) Execute(ctx context.Context, task *domain.Task, workerID string, handler domain.JobHandler) Result {
	timeout := e.taskTimeout
	if task.TaskTimeout > 0 {
		timeout = task.TaskTimeout
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := e.runHandler(execCtx, handler, task.Payload)
	now := time.Now().UTC()

	if err == nil {
		if ackErr := e.storage.Complete(ctx, task.OrgID, task.ID, now); ackErr != nil {
			e.logger.Error("engine: complete failed", zap.String("task_id", task.ID.String()), zap.Error(ackErr))
			return Result{TaskID: task.ID, Outcome: "completed", Err: ackErr}
		}
		if ackErr := e.broker.Ack(ctx, task.OrgID, task.Queue, task.ID); ackErr != nil {
			e.logger.Warn("engine: broker ack failed (storage already completed)", zap.Error(ackErr))
		}
		return Result{TaskID: task.ID, Outcome: "completed"}
	}

	return e.handleFailure(ctx, task, err, now)
}

// runHandler invokes the handler and normalizes timeout/panic into errors.
func (e *Engine) runHandler(ctx context.Context, handler domain.JobHandler, payload []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- handler.Handle(ctx, payload)
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("task execution timeout: %w", ctx.Err())
	case handlerErr := <-done:
		return handlerErr
	}
}

// handleFailure decides retry vs DLQ. Fatal errors (not wrapped in
// domain.RetryableError) still go through the normal retry budget — the
// wrapper only exists for handlers that want to explicitly force-retry
// something that might otherwise look permanent. Exhausted budget always
// routes to DLQ regardless of error type.
func (e *Engine) handleFailure(ctx context.Context, task *domain.Task, execErr error, now time.Time) Result {
	task.LastError = execErr.Error()

	if task.IsExhausted() {
		if err := e.storage.MoveToDeadLetter(ctx, task.OrgID, task.ID, execErr.Error(), now); err != nil {
			e.logger.Error("engine: move to dlq failed", zap.String("task_id", task.ID.String()), zap.Error(err))
			return Result{TaskID: task.ID, Outcome: "dead_letter", Err: err}
		}
		if err := e.broker.MoveToDeadLetter(ctx, task.OrgID, task.Queue, task.ID); err != nil {
			e.logger.Warn("engine: broker move to dlq failed", zap.Error(err))
		}
		e.logger.Warn("engine: task exhausted, routed to DLQ",
			zap.String("task_id", task.ID.String()), zap.Int("attempts", task.Attempts))
		return Result{TaskID: task.ID, Outcome: "dead_letter", Err: execErr}
	}

	backoff := e.nextBackoff(task)
	visibleAt := now.Add(backoff)

	if err := e.storage.Fail(ctx, task.OrgID, task.ID, execErr.Error(), domain.StatusPending, visibleAt, now); err != nil {
		e.logger.Error("engine: fail update failed", zap.String("task_id", task.ID.String()), zap.Error(err))
		return Result{TaskID: task.ID, Outcome: "retried", Err: err}
	}
	if err := e.broker.Nack(ctx, task.OrgID, task.Queue, task.ID, backoff); err != nil {
		e.logger.Warn("engine: broker nack failed", zap.Error(err))
	} else if err := e.storage.MarkDispatched(ctx, task.OrgID, task.ID, now); err != nil {
		e.logger.Warn("engine: retry dispatch marker failed", zap.Error(err))
	}

	e.logger.Info("engine: task retry scheduled",
		zap.String("task_id", task.ID.String()), zap.Int("attempt", task.Attempts), zap.Duration("backoff", backoff))
	return Result{TaskID: task.ID, Outcome: "retried", Err: execErr}
}

func (e *Engine) nextBackoff(task *domain.Task) time.Duration {
	switch task.BackoffStrategy {
	case "fixed":
		return e.retryPolicy.InitialBackoff
	case "linear":
		backoff := time.Duration(task.Attempts) * e.retryPolicy.InitialBackoff
		if backoff > e.retryPolicy.MaxBackoff {
			return e.retryPolicy.MaxBackoff
		}
		return backoff
	default:
		return e.retryPolicy.NextBackoff(task.Attempts)
	}
}

// ReclaimLoop periodically scans for crashed-worker tasks (visibility
// expired) and re-enqueues them on the broker.
func (e *Engine) ReclaimLoop(ctx context.Context, orgID uuid.UUID, queue string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reclaimed, err := e.storage.ReclaimExpired(ctx, orgID, queue, time.Now().UTC(), 100)
			if err != nil {
				e.logger.Error("engine: reclaim scan failed", zap.Error(err))
				continue
			}
			for _, t := range reclaimed {
				if err := e.broker.Enqueue(ctx, t); err != nil {
					e.logger.Error("engine: reclaim re-enqueue failed", zap.String("task_id", t.ID.String()), zap.Error(err))
					continue
				}
				if err := e.storage.MarkDispatched(ctx, t.OrgID, t.ID, time.Now().UTC()); err != nil {
					e.logger.Warn("engine: reclaim dispatch marker failed", zap.Error(err))
				}
			}
			if len(reclaimed) > 0 {
				e.logger.Info("engine: reclaimed expired tasks", zap.Int("count", len(reclaimed)), zap.String("queue", queue))
			}
		}
	}
}

// DelayedPromotionLoop periodically promotes due delayed (backoff) tasks
// from the broker's delayed set back to the active pending queue.
func (e *Engine) DelayedPromotionLoop(ctx context.Context, orgID uuid.UUID, queue string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := e.broker.PromoteDueDelayed(ctx, orgID, queue)
			if err != nil {
				e.logger.Error("engine: promote delayed failed", zap.Error(err))
				continue
			}
			if n > 0 {
				e.logger.Debug("engine: promoted delayed tasks", zap.Int("count", n))
			}
		}
	}
}

// ReconciliationLoop repairs the Postgres-commit/Redis-enqueue gap. Duplicate
// dispatch is acceptable because claiming remains authoritative in Postgres.
func (e *Engine) ReconciliationLoop(ctx context.Context, orgID uuid.UUID, queue string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tasks, err := e.storage.ListUndispatchedPending(ctx, orgID, queue, 100)
			if err != nil {
				e.logger.Error("engine: reconciliation scan failed", zap.Error(err))
				continue
			}
			for _, task := range tasks {
				if err := e.broker.Enqueue(ctx, task); err != nil {
					e.logger.Warn("engine: reconciliation enqueue failed", zap.Error(err))
					continue
				}
				if err := e.storage.MarkDispatched(ctx, orgID, task.ID, time.Now().UTC()); err != nil {
					e.logger.Warn("engine: reconciliation dispatch marker failed", zap.Error(err))
				}
			}
		}
	}
}

// ErrHandlerNotFound is returned by a registry lookup miss.
var ErrHandlerNotFound = errors.New("no handler registered for queue")
