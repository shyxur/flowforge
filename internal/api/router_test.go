package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/testutil"
	"github.com/shyxur/flowforge/internal/usecase"
	"go.uber.org/zap"
)

type authFunc func(context.Context, string) (*domain.Principal, error)

func (fn authFunc) Authenticate(ctx context.Context, key string) (*domain.Principal, error) {
	return fn(ctx, key)
}

type serviceStub struct {
	TaskService
	getTask   func(context.Context, uuid.UUID, uuid.UUID) (*domain.Task, error)
	listTasks func(context.Context, uuid.UUID, domain.TaskFilter) (*domain.TaskPage, error)
}

func (s *serviceStub) GetTask(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error) {
	return s.getTask(ctx, orgID, id)
}

func (s *serviceStub) ListTasks(ctx context.Context, orgID uuid.UUID, filter domain.TaskFilter) (*domain.TaskPage, error) {
	return s.listTasks(ctx, orgID, filter)
}

func TestInvalidAndRevokedAPIKeyReturn401(t *testing.T) {
	for _, name := range []string{"invalid", "revoked"} {
		t.Run(name, func(t *testing.T) {
			router := testRouter(&serviceStub{}, authFunc(func(context.Context, string) (*domain.Principal, error) {
				return nil, domain.ErrUnauthorized
			}))
			req := httptest.NewRequest(http.MethodGet, "/v1/tasks", nil)
			req.Header.Set("Authorization", "Bearer "+name)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestCreateRequiresIdempotencyKey(t *testing.T) {
	router := testRouter(&serviceStub{}, allowAuth(uuid.New()))
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks",
		strings.NewReader(`{"queue":"default","payload":{"x":1}}`))
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing_idempotency_key") {
		t.Fatalf("status/body = %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreateRejectsPayloadOver256KiB(t *testing.T) {
	router := testRouter(&serviceStub{}, allowAuth(uuid.New()))
	body := `{"queue":"default","payload":"` + strings.Repeat("a", maxPayloadBytes+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid")
	req.Header.Set("Idempotency-Key", "large")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateIdempotencyReplayAndConflict(t *testing.T) {
	orgID := uuid.New()
	var stored *domain.Task
	storage := &testutil.StorageStub{
		FindByIdempotencyKeyFunc: func(context.Context, uuid.UUID, string, string) (*domain.Task, error) {
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
	service := usecase.NewService(storage, &testutil.BrokerStub{})
	router := testRouter(service, allowAuth(orgID))

	request := func(payload string) *httptest.ResponseRecorder {
		body := []byte(`{"queue":"default","payload":` + payload + `}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/tasks", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid")
		req.Header.Set("Idempotency-Key", "same-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := request(`{"x":1}`); rec.Code != http.StatusCreated {
		t.Fatalf("first status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request(`{"x":1}`); rec.Code != http.StatusOK {
		t.Fatalf("replay status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request(`{"x":2}`); rec.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetTaskUsesAuthenticatedOrg(t *testing.T) {
	orgID, taskID := uuid.New(), uuid.New()
	service := &serviceStub{getTask: func(_ context.Context, gotOrg, gotID uuid.UUID) (*domain.Task, error) {
		if gotOrg != orgID || gotID != taskID {
			t.Fatalf("got %s/%s, want %s/%s", gotOrg, gotID, orgID, taskID)
		}
		return &domain.Task{ID: taskID, OrgID: orgID}, nil
	}}
	router := testRouter(service, allowAuth(orgID))
	req := httptest.NewRequest(http.MethodGet, "/v1/tasks/"+taskID.String(), nil)
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTaskEventsUseAuthenticatedOrg(t *testing.T) {
	orgID, taskID := uuid.New(), uuid.New()
	called := make(chan struct{})
	service := &serviceStub{
		listTasks: func(_ context.Context, gotOrg uuid.UUID, _ domain.TaskFilter) (*domain.TaskPage, error) {
			if gotOrg != orgID {
				t.Errorf("stream org = %s, want %s", gotOrg, orgID)
			}
			select {
			case <-called:
			default:
				close(called)
			}
			return &domain.TaskPage{Tasks: []*domain.Task{{
				ID: taskID, OrgID: orgID, Queue: "default",
				Status: domain.StatusPending, UpdatedAt: time.Now().UTC(),
			}}}, nil
		},
	}
	router := testRouter(service, allowAuth(orgID))
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/events/tasks", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()
	<-called
	cancel()
	<-done
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"`+taskID.String()) {
		t.Fatalf("status/body = %d %s", rec.Code, rec.Body.String())
	}
}

func TestTaskEventsRequireAuthentication(t *testing.T) {
	router := testRouter(&serviceStub{}, allowAuth(uuid.New()))
	req := httptest.NewRequest(http.MethodGet, "/v1/events/tasks", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func allowAuth(orgID uuid.UUID) Authenticator {
	return authFunc(func(_ context.Context, key string) (*domain.Principal, error) {
		if key != "valid" {
			return nil, errors.New("invalid")
		}
		return &domain.Principal{OrgID: orgID}, nil
	})
}

func testRouter(service TaskService, auth Authenticator) http.Handler {
	return NewRouter(NewHandler(service, zap.NewNop()), auth, nil, zap.NewNop())
}
