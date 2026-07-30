package testutil

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/flowforge/internal/domain"
)

type StorageStub struct {
	CreateFunc                  func(context.Context, *domain.Task) error
	GetByIDFunc                 func(context.Context, uuid.UUID, uuid.UUID) (*domain.Task, error)
	FindByIdempotencyKeyFunc    func(context.Context, uuid.UUID, string, string) (*domain.Task, error)
	ClaimForProcessingFunc      func(context.Context, uuid.UUID, uuid.UUID, string, time.Time) (*domain.Task, error)
	HeartbeatFunc               func(context.Context, uuid.UUID, uuid.UUID, string, time.Duration, time.Time) error
	CompleteFunc                func(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	FailFunc                    func(context.Context, uuid.UUID, uuid.UUID, string, domain.TaskStatus, time.Time, time.Time) error
	MoveToDeadLetterFunc        func(context.Context, uuid.UUID, uuid.UUID, string, time.Time) error
	ReclaimExpiredFunc          func(context.Context, uuid.UUID, string, time.Time, int) ([]*domain.Task, error)
	ListDeadLetterFunc          func(context.Context, uuid.UUID, string, string, int) (*domain.TaskPage, error)
	RequeueFunc                 func(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	ListTasksFunc               func(context.Context, uuid.UUID, domain.TaskFilter) (*domain.TaskPage, error)
	CancelFunc                  func(context.Context, uuid.UUID, uuid.UUID, time.Time) (*domain.Task, error)
	SoftDeleteFunc              func(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	QueueStatsFunc              func(context.Context, uuid.UUID, string) (*domain.QueueStats, error)
	FindAPIKeyByHashFunc        func(context.Context, string) (*domain.APIKey, error)
	MarkDispatchedFunc          func(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	ListUndispatchedPendingFunc func(context.Context, uuid.UUID, string, int) ([]*domain.Task, error)
	ListWorkersFunc             func(context.Context, uuid.UUID) ([]*domain.Worker, error)
	UpsertWorkerHeartbeatFunc   func(context.Context, *domain.Worker) error
	ListActiveQueueScopesFunc   func(context.Context) ([]domain.QueueScope, error)
	PingFunc                    func(context.Context) error
}

func (s *StorageStub) Create(ctx context.Context, task *domain.Task) error {
	if s.CreateFunc != nil {
		return s.CreateFunc(ctx, task)
	}
	return nil
}
func (s *StorageStub) GetByID(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error) {
	if s.GetByIDFunc != nil {
		return s.GetByIDFunc(ctx, orgID, id)
	}
	return nil, domain.ErrTaskNotFound
}
func (s *StorageStub) FindByIdempotencyKey(ctx context.Context, orgID uuid.UUID, queue, key string) (*domain.Task, error) {
	if s.FindByIdempotencyKeyFunc != nil {
		return s.FindByIdempotencyKeyFunc(ctx, orgID, queue, key)
	}
	return nil, domain.ErrTaskNotFound
}
func (s *StorageStub) ClaimForProcessing(ctx context.Context, orgID, id uuid.UUID, workerID string, now time.Time) (*domain.Task, error) {
	if s.ClaimForProcessingFunc != nil {
		return s.ClaimForProcessingFunc(ctx, orgID, id, workerID, now)
	}
	return nil, domain.ErrTaskAlreadyLocked
}
func (s *StorageStub) Heartbeat(ctx context.Context, orgID, id uuid.UUID, workerID string, timeout time.Duration, now time.Time) error {
	if s.HeartbeatFunc != nil {
		return s.HeartbeatFunc(ctx, orgID, id, workerID, timeout, now)
	}
	return nil
}
func (s *StorageStub) Complete(ctx context.Context, orgID, id uuid.UUID, now time.Time) error {
	if s.CompleteFunc != nil {
		return s.CompleteFunc(ctx, orgID, id, now)
	}
	return nil
}
func (s *StorageStub) Fail(ctx context.Context, orgID, id uuid.UUID, message string, status domain.TaskStatus, visibleAt, now time.Time) error {
	if s.FailFunc != nil {
		return s.FailFunc(ctx, orgID, id, message, status, visibleAt, now)
	}
	return nil
}
func (s *StorageStub) MoveToDeadLetter(ctx context.Context, orgID, id uuid.UUID, message string, now time.Time) error {
	if s.MoveToDeadLetterFunc != nil {
		return s.MoveToDeadLetterFunc(ctx, orgID, id, message, now)
	}
	return nil
}
func (s *StorageStub) ReclaimExpired(ctx context.Context, orgID uuid.UUID, queue string, now time.Time, limit int) ([]*domain.Task, error) {
	if s.ReclaimExpiredFunc != nil {
		return s.ReclaimExpiredFunc(ctx, orgID, queue, now, limit)
	}
	return nil, nil
}
func (s *StorageStub) ListDeadLetter(ctx context.Context, orgID uuid.UUID, queue, cursor string, limit int) (*domain.TaskPage, error) {
	if s.ListDeadLetterFunc != nil {
		return s.ListDeadLetterFunc(ctx, orgID, queue, cursor, limit)
	}
	return &domain.TaskPage{}, nil
}
func (s *StorageStub) Requeue(ctx context.Context, orgID, id uuid.UUID, now time.Time) error {
	if s.RequeueFunc != nil {
		return s.RequeueFunc(ctx, orgID, id, now)
	}
	return nil
}
func (s *StorageStub) ListTasks(ctx context.Context, orgID uuid.UUID, filter domain.TaskFilter) (*domain.TaskPage, error) {
	if s.ListTasksFunc != nil {
		return s.ListTasksFunc(ctx, orgID, filter)
	}
	return &domain.TaskPage{}, nil
}
func (s *StorageStub) Cancel(ctx context.Context, orgID, id uuid.UUID, now time.Time) (*domain.Task, error) {
	if s.CancelFunc != nil {
		return s.CancelFunc(ctx, orgID, id, now)
	}
	return nil, domain.ErrInvalidStateTransition
}
func (s *StorageStub) SoftDelete(ctx context.Context, orgID, id uuid.UUID, now time.Time) error {
	if s.SoftDeleteFunc != nil {
		return s.SoftDeleteFunc(ctx, orgID, id, now)
	}
	return nil
}
func (s *StorageStub) QueueStats(ctx context.Context, orgID uuid.UUID, queue string) (*domain.QueueStats, error) {
	if s.QueueStatsFunc != nil {
		return s.QueueStatsFunc(ctx, orgID, queue)
	}
	return &domain.QueueStats{Queue: queue}, nil
}
func (s *StorageStub) FindAPIKeyByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	if s.FindAPIKeyByHashFunc != nil {
		return s.FindAPIKeyByHashFunc(ctx, hash)
	}
	return nil, domain.ErrUnauthorized
}
func (s *StorageStub) MarkDispatched(ctx context.Context, orgID, id uuid.UUID, now time.Time) error {
	if s.MarkDispatchedFunc != nil {
		return s.MarkDispatchedFunc(ctx, orgID, id, now)
	}
	return nil
}
func (s *StorageStub) ListUndispatchedPending(ctx context.Context, orgID uuid.UUID, queue string, limit int) ([]*domain.Task, error) {
	if s.ListUndispatchedPendingFunc != nil {
		return s.ListUndispatchedPendingFunc(ctx, orgID, queue, limit)
	}
	return nil, nil
}
func (s *StorageStub) ListWorkers(ctx context.Context, orgID uuid.UUID) ([]*domain.Worker, error) {
	if s.ListWorkersFunc != nil {
		return s.ListWorkersFunc(ctx, orgID)
	}
	return nil, nil
}
func (s *StorageStub) UpsertWorkerHeartbeat(ctx context.Context, worker *domain.Worker) error {
	if s.UpsertWorkerHeartbeatFunc != nil {
		return s.UpsertWorkerHeartbeatFunc(ctx, worker)
	}
	return nil
}
func (s *StorageStub) ListActiveQueueScopes(ctx context.Context) ([]domain.QueueScope, error) {
	if s.ListActiveQueueScopesFunc != nil {
		return s.ListActiveQueueScopesFunc(ctx)
	}
	return nil, nil
}
func (s *StorageStub) Ping(ctx context.Context) error {
	if s.PingFunc != nil {
		return s.PingFunc(ctx)
	}
	return nil
}
func (s *StorageStub) Close() error { return nil }

type BrokerStub struct {
	EnqueueFunc          func(context.Context, *domain.Task) error
	EnqueueDelayedFunc   func(context.Context, *domain.Task, time.Duration) error
	AckFunc              func(context.Context, uuid.UUID, string, uuid.UUID) error
	NackFunc             func(context.Context, uuid.UUID, string, uuid.UUID, time.Duration) error
	MoveToDeadLetterFunc func(context.Context, uuid.UUID, string, uuid.UUID) error
	RemoveFunc           func(context.Context, uuid.UUID, string, uuid.UUID) error
	PingFunc             func(context.Context) error
}

func (b *BrokerStub) Enqueue(ctx context.Context, task *domain.Task) error {
	if b.EnqueueFunc != nil {
		return b.EnqueueFunc(ctx, task)
	}
	return nil
}
func (b *BrokerStub) Dequeue(context.Context, uuid.UUID, string, time.Duration) (uuid.UUID, error) {
	return uuid.Nil, domain.ErrQueueEmpty
}
func (b *BrokerStub) Ack(ctx context.Context, orgID uuid.UUID, queue string, id uuid.UUID) error {
	if b.AckFunc != nil {
		return b.AckFunc(ctx, orgID, queue, id)
	}
	return nil
}
func (b *BrokerStub) Nack(ctx context.Context, orgID uuid.UUID, queue string, id uuid.UUID, delay time.Duration) error {
	if b.NackFunc != nil {
		return b.NackFunc(ctx, orgID, queue, id, delay)
	}
	return nil
}
func (b *BrokerStub) EnqueueDelayed(ctx context.Context, task *domain.Task, delay time.Duration) error {
	if b.EnqueueDelayedFunc != nil {
		return b.EnqueueDelayedFunc(ctx, task, delay)
	}
	return nil
}
func (b *BrokerStub) MoveToDeadLetter(ctx context.Context, orgID uuid.UUID, queue string, id uuid.UUID) error {
	if b.MoveToDeadLetterFunc != nil {
		return b.MoveToDeadLetterFunc(ctx, orgID, queue, id)
	}
	return nil
}
func (b *BrokerStub) PromoteDueDelayed(context.Context, uuid.UUID, string) (int, error) {
	return 0, nil
}
func (b *BrokerStub) QueueDepth(context.Context, uuid.UUID, string) (int64, error) {
	return 0, nil
}
func (b *BrokerStub) Remove(ctx context.Context, orgID uuid.UUID, queue string, id uuid.UUID) error {
	if b.RemoveFunc != nil {
		return b.RemoveFunc(ctx, orgID, queue, id)
	}
	return nil
}
func (b *BrokerStub) Ping(ctx context.Context) error {
	if b.PingFunc != nil {
		return b.PingFunc(ctx)
	}
	return nil
}
func (b *BrokerStub) Close() error { return nil }
