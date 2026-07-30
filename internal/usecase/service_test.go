package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/distributed-task-queue/internal/domain"
	"github.com/shyxur/distributed-task-queue/internal/testutil"
)

func TestCreateTaskIdempotencyFingerprint(t *testing.T) {
	orgID := uuid.New()
	var stored *domain.Task
	storage := &testutil.StorageStub{
		FindByIdempotencyKeyFunc: func(_ context.Context, gotOrg uuid.UUID, queue, key string) (*domain.Task, error) {
			if gotOrg != orgID {
				t.Fatalf("lookup org = %s, want %s", gotOrg, orgID)
			}
			if stored == nil {
				return nil, domain.ErrTaskNotFound
			}
			return stored, nil
		},
		CreateFunc: func(_ context.Context, task *domain.Task) error {
			stored = task
			return nil
		},
	}
	service := NewService(storage, &testutil.BrokerStub{})
	input := CreateTaskInput{
		OrgID: orgID, IdempotencyKey: "charge-1", Queue: "billing",
		Payload: json.RawMessage(`{"amount":42}`), MaxRetries: 2,
		Timeout: time.Minute, VisibilityTimeout: 30 * time.Second,
		BackoffStrategy: "exponential",
	}

	first, replay, err := service.CreateTask(context.Background(), input)
	if err != nil || replay {
		t.Fatalf("first create: replay=%v err=%v", replay, err)
	}
	second, replay, err := service.CreateTask(context.Background(), input)
	if err != nil || !replay || second.ID != first.ID {
		t.Fatalf("same request: task=%v replay=%v err=%v", second, replay, err)
	}

	input.Payload = json.RawMessage(`{"amount":43}`)
	_, _, err = service.CreateTask(context.Background(), input)
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("different payload error = %v, want idempotency conflict", err)
	}
}

func TestAuthServiceRejectsRevokedKey(t *testing.T) {
	now := time.Now()
	storage := &testutil.StorageStub{
		FindAPIKeyByHashFunc: func(context.Context, string) (*domain.APIKey, error) {
			return &domain.APIKey{OrgID: uuid.New(), RevokedAt: &now}, nil
		},
	}
	_, err := NewAuthService(storage).Authenticate(context.Background(), "revoked")
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("error = %v, want unauthorized", err)
	}
}

func TestServicePassesTenantToReads(t *testing.T) {
	orgID, taskID := uuid.New(), uuid.New()
	storage := &testutil.StorageStub{
		GetByIDFunc: func(_ context.Context, gotOrg, gotID uuid.UUID) (*domain.Task, error) {
			if gotOrg != orgID || gotID != taskID {
				t.Fatalf("got org/task %s/%s", gotOrg, gotID)
			}
			return &domain.Task{ID: taskID, OrgID: orgID}, nil
		},
	}
	_, err := NewService(storage, &testutil.BrokerStub{}).GetTask(context.Background(), orgID, taskID)
	if err != nil {
		t.Fatal(err)
	}

	storage.ListTasksFunc = func(_ context.Context, gotOrg uuid.UUID, _ domain.TaskFilter) (*domain.TaskPage, error) {
		if gotOrg != orgID {
			t.Fatalf("list org = %s, want %s", gotOrg, orgID)
		}
		return &domain.TaskPage{}, nil
	}
	if _, err := NewService(storage, &testutil.BrokerStub{}).ListTasks(context.Background(), orgID, domain.TaskFilter{}); err != nil {
		t.Fatal(err)
	}
}

func TestSoftDeleteRequiresTerminalState(t *testing.T) {
	orgID, taskID := uuid.New(), uuid.New()
	status := domain.StatusPending
	deleted := false
	storage := &testutil.StorageStub{
		GetByIDFunc: func(context.Context, uuid.UUID, uuid.UUID) (*domain.Task, error) {
			return &domain.Task{ID: taskID, OrgID: orgID, Status: status}, nil
		},
		SoftDeleteFunc: func(context.Context, uuid.UUID, uuid.UUID, time.Time) error {
			deleted = true
			return nil
		},
	}
	service := NewService(storage, &testutil.BrokerStub{})
	if err := service.SoftDeleteTask(context.Background(), orgID, taskID); !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("pending delete error = %v", err)
	}
	if deleted {
		t.Fatal("pending task was deleted")
	}
	status = domain.StatusCompleted
	if err := service.SoftDeleteTask(context.Background(), orgID, taskID); err != nil {
		t.Fatalf("terminal delete: %v", err)
	}
	if !deleted {
		t.Fatal("terminal task was not deleted")
	}
}
