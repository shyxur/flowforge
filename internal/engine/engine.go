package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	metricspkg "github.com/shyxur/windylane/internal/metrics"
	"github.com/shyxur/windylane/internal/ports"
	"go.uber.org/zap"
)

// Engine owns retry/backoff/timeout/DLQ decision logic. It is broker- and
// storage-agnostic beyond the ports interfaces — worker pool drives it.
type Engine struct {
	storage     ports.Storage
	broker      ports.Broker
	retryPolicy domain.RetryPolicy
	taskTimeout time.Duration // per-task execution deadline
	events      ports.TaskEventPublisher
	logger      *zap.Logger
	metrics     ports.MetricRecorder
}

func (e *Engine) WithMetricRecorder(recorder ports.MetricRecorder) *Engine {
	e.metrics = recorder
	return e
}

func NewEngine(storage ports.Storage, broker ports.Broker, retryPolicy domain.RetryPolicy, taskTimeout time.Duration, logger *zap.Logger, eventPublishers ...ports.TaskEventPublisher) *Engine {
	engine := &Engine{
		storage:     storage,
		broker:      broker,
		retryPolicy: retryPolicy,
		taskTimeout: taskTimeout,
		logger:      logger,
	}
	if len(eventPublishers) > 0 {
		engine.events = eventPublishers[0]
	}
	return engine
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
		task.Status = domain.StatusCompleted
		task.CompletedAt = &now
		e.recordTaskMetric(task, domain.MetricTaskSucceeded, now, "")
		task.UpdatedAt = now
		e.publishTaskEvent(ctx, domain.WebhookEventTaskCompleted, task)
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
		task.Status = domain.StatusDeadLetter
		e.recordTaskMetric(task, domain.MetricTaskFailed, now, "handler_error")
		e.recordTaskMetric(task, domain.MetricTaskDeadLettered, now, "handler_error")
		task.UpdatedAt = now
		e.publishTaskEvent(ctx, domain.WebhookEventTaskDeadLetter, task)
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
	task.Status = domain.StatusPending
	task.VisibleAt = visibleAt
	e.recordTaskMetric(task, domain.MetricTaskFailed, now, "handler_error")
	e.recordTaskMetric(task, domain.MetricTaskRetryScheduled, now, "handler_error")
	task.UpdatedAt = now
	e.publishTaskEvent(ctx, domain.WebhookEventTaskFailed, task)
	if err := e.broker.Nack(ctx, task.OrgID, task.Queue, task.ID, backoff); err != nil {
		e.logger.Warn("engine: broker nack failed", zap.Error(err))
	} else if err := e.storage.MarkDispatched(ctx, task.OrgID, task.ID, now); err != nil {
		e.logger.Warn("engine: retry dispatch marker failed", zap.Error(err))
	}

	e.logger.Info("engine: task retry scheduled",
		zap.String("task_id", task.ID.String()), zap.Int("attempt", task.Attempts), zap.Duration("backoff", backoff))
	return Result{TaskID: task.ID, Outcome: "retried", Err: execErr}
}

func (e *Engine) PublishProcessingEvent(ctx context.Context, task *domain.Task) {
	e.recordTaskMetric(task, domain.MetricTaskStarted, time.Now().UTC(), "")
	e.publishTaskEvent(ctx, domain.WebhookEventTaskProcessing, task)
}

func (e *Engine) recordTaskMetric(
	task *domain.Task,
	eventType domain.MetricEventType,
	now time.Time,
	errorCode string,
) {
	if task == nil {
		return
	}
	attempt, maxAttempts := task.Attempts, task.MaxAttempts
	var durationMS *int64
	if eventType == domain.MetricTaskSucceeded || eventType == domain.MetricTaskFailed {
		value := now.Sub(task.UpdatedAt).Milliseconds()
		if value < 0 {
			value = 0
		}
		durationMS = &value
	}
	metricspkg.Record(e.metrics, domain.NewMetricEventInput{
		OrganizationID: task.OrgID, Source: domain.MetricSourceQueueFlow,
		EventType: eventType, ResourceType: domain.MetricResourceTask,
		ResourceID: task.ID.String(), Queue: task.Queue, Status: string(task.Status),
		DurationMS: durationMS, OccurredAt: now,
		Metadata: domain.MetricMetadata{
			Attempt: &attempt, MaxAttempts: &maxAttempts, ErrorCode: errorCode,
		},
		TransitionKey: domain.MetricTransitionKey(task.Attempts, eventType),
	})
}

func (e *Engine) publishTaskEvent(ctx context.Context, eventType domain.WebhookEventType, task *domain.Task) {
	if e.events == nil {
		return
	}
	if err := e.events.PublishTaskEvent(ctx, eventType, task); err != nil {
		e.logger.Warn("engine: publish task webhook event failed",
			zap.String("task_id", task.ID.String()),
			zap.String("event_type", string(eventType)),
			zap.Error(err))
	}
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
