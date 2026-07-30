package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/ports"
)

type Service struct {
	storage        ports.Storage
	broker         ports.Broker
	eventPublisher ports.TaskEventPublisher
}

func NewService(storage ports.Storage, broker ports.Broker, eventPublishers ...ports.TaskEventPublisher) *Service {
	service := &Service{storage: storage, broker: broker}
	if len(eventPublishers) > 0 {
		service.eventPublisher = eventPublishers[0]
	}
	return service
}

type CreateTaskInput struct {
	OrgID             uuid.UUID
	IdempotencyKey    string
	Queue             string
	Payload           json.RawMessage
	Priority          int
	MaxRetries        int
	Timeout           time.Duration
	VisibilityTimeout time.Duration
	ScheduledAt       *time.Time
	BackoffStrategy   string
	TraceID           string
}

func (s *Service) CreateTask(ctx context.Context, input CreateTaskInput) (*domain.Task, bool, error) {
	fingerprint, normalizedPayload, err := requestFingerprint(input)
	if err != nil {
		return nil, false, domain.ErrInvalidPayload
	}
	existing, err := s.storage.FindByIdempotencyKey(ctx, input.OrgID, input.Queue, input.IdempotencyKey)
	if err == nil {
		if existing.RequestFingerprint != fingerprint {
			return nil, false, domain.ErrIdempotencyConflict
		}
		return existing, true, nil
	}
	if !errors.Is(err, domain.ErrTaskNotFound) {
		return nil, false, err
	}

	task := domain.NewTask(
		input.OrgID,
		input.Queue,
		normalizedPayload,
		input.IdempotencyKey,
		fingerprint,
		input.MaxRetries+1,
		input.VisibilityTimeout,
	)
	task.Priority = input.Priority
	task.BackoffStrategy = input.BackoffStrategy
	task.TaskTimeout = input.Timeout
	task.TraceID = input.TraceID
	if input.ScheduledAt != nil {
		task.ScheduledAt = input.ScheduledAt.UTC()
		task.VisibleAt = task.ScheduledAt
	}

	if err := s.storage.Create(ctx, task); err != nil {
		if errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
			existing, findErr := s.storage.FindByIdempotencyKey(ctx, input.OrgID, input.Queue, input.IdempotencyKey)
			if findErr == nil {
				if existing.RequestFingerprint != fingerprint {
					return nil, false, domain.ErrIdempotencyConflict
				}
				return existing, true, nil
			}
		}
		return nil, false, err
	}
	s.publishTaskEvent(ctx, domain.WebhookEventTaskCreated, task)

	now := time.Now().UTC()
	if task.ScheduledAt.After(now) {
		err = s.broker.EnqueueDelayed(ctx, task, time.Until(task.ScheduledAt))
	} else {
		err = s.broker.Enqueue(ctx, task)
	}
	if err != nil {
		return task, false, domain.ErrDispatchUnavailable
	}
	if err := s.storage.MarkDispatched(ctx, task.OrgID, task.ID, now); err != nil {
		return task, false, err
	}
	return task, false, nil
}

func (s *Service) GetTask(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error) {
	return s.storage.GetByID(ctx, orgID, id)
}

func (s *Service) ListTasks(ctx context.Context, orgID uuid.UUID, filter domain.TaskFilter) (*domain.TaskPage, error) {
	return s.storage.ListTasks(ctx, orgID, filter)
}

func (s *Service) RetryTask(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error) {
	task, err := s.storage.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := s.storage.Requeue(ctx, orgID, id, now); err != nil {
		return nil, err
	}
	task.Status = domain.StatusPending
	task.Attempts = 0
	task.VisibleAt = now
	if err := s.broker.Enqueue(ctx, task); err != nil {
		return task, domain.ErrDispatchUnavailable
	}
	if err := s.storage.MarkDispatched(ctx, orgID, id, now); err != nil {
		return task, err
	}
	return s.storage.GetByID(ctx, orgID, id)
}

func (s *Service) CancelTask(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error) {
	task, err := s.storage.Cancel(ctx, orgID, id, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	_ = s.broker.Remove(ctx, orgID, task.Queue, task.ID)
	s.publishTaskEvent(ctx, domain.WebhookEventTaskCancelled, task)
	return task, nil
}

func (s *Service) SoftDeleteTask(ctx context.Context, orgID, id uuid.UUID) error {
	task, err := s.storage.GetByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	if !task.IsTerminal() {
		return domain.ErrInvalidStateTransition
	}
	if err := s.storage.SoftDelete(ctx, orgID, id, time.Now().UTC()); err != nil {
		return err
	}
	_ = s.broker.Remove(ctx, orgID, task.Queue, id)
	return nil
}

func (s *Service) QueueStats(ctx context.Context, orgID uuid.UUID, queue string) (*domain.QueueStats, error) {
	return s.storage.QueueStats(ctx, orgID, queue)
}

func (s *Service) ListDLQ(ctx context.Context, orgID uuid.UUID, queue, cursor string, limit int) (*domain.TaskPage, error) {
	return s.storage.ListDeadLetter(ctx, orgID, queue, cursor, limit)
}

func (s *Service) RequeueDLQ(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error) {
	return s.RetryTask(ctx, orgID, id)
}

func (s *Service) ListWorkers(ctx context.Context, orgID uuid.UUID) ([]*domain.Worker, error) {
	return s.storage.ListWorkers(ctx, orgID)
}

func (s *Service) WorkerHeartbeat(ctx context.Context, worker *domain.Worker) error {
	now := time.Now().UTC()
	worker.LastHeartbeatAt = now
	if worker.CreatedAt.IsZero() {
		worker.CreatedAt = now
	}
	if worker.Status == "" {
		worker.Status = "online"
	}
	return s.storage.UpsertWorkerHeartbeat(ctx, worker)
}

func (s *Service) Ready(ctx context.Context) error {
	if err := s.storage.Ping(ctx); err != nil {
		return err
	}
	return s.broker.Ping(ctx)
}

func (s *Service) publishTaskEvent(ctx context.Context, eventType domain.WebhookEventType, task *domain.Task) {
	if s.eventPublisher != nil {
		_ = s.eventPublisher.PublishTaskEvent(ctx, eventType, task)
	}
}

func requestFingerprint(input CreateTaskInput) (string, json.RawMessage, error) {
	var payload any
	if err := json.Unmarshal(input.Payload, &payload); err != nil {
		return "", nil, err
	}
	normalizedPayload, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	var scheduledAt *time.Time
	if input.ScheduledAt != nil {
		utc := input.ScheduledAt.UTC()
		scheduledAt = &utc
	}
	canonical := struct {
		Queue           string          `json:"queue"`
		Payload         json.RawMessage `json:"payload"`
		Priority        int             `json:"priority"`
		MaxRetries      int             `json:"max_retries"`
		TimeoutMS       int64           `json:"timeout_ms"`
		VisibilityMS    int64           `json:"visibility_timeout_ms"`
		ScheduledAt     *time.Time      `json:"scheduled_at"`
		BackoffStrategy string          `json:"backoff_strategy"`
	}{
		Queue: input.Queue, Payload: normalizedPayload, Priority: input.Priority,
		MaxRetries: input.MaxRetries, TimeoutMS: input.Timeout.Milliseconds(),
		VisibilityMS: input.VisibilityTimeout.Milliseconds(),
		ScheduledAt:  scheduledAt, BackoffStrategy: input.BackoffStrategy,
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), normalizedPayload, nil
}

type AuthService struct {
	storage ports.Storage
}

func NewAuthService(storage ports.Storage) *AuthService {
	return &AuthService{storage: storage}
}

func (s *AuthService) Authenticate(ctx context.Context, rawKey string) (*domain.Principal, error) {
	sum := sha256.Sum256([]byte(rawKey))
	key, err := s.storage.FindAPIKeyByHash(ctx, hex.EncodeToString(sum[:]))
	if err != nil || key.RevokedAt != nil {
		return nil, domain.ErrUnauthorized
	}
	return &domain.Principal{OrgID: key.OrgID, Scopes: key.Scopes}, nil
}
