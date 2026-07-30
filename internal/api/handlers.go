package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/usecase"
	"go.uber.org/zap"
)

const (
	maxPayloadBytes     = 256 * 1024
	maxRequestBodyBytes = maxPayloadBytes + 16*1024
)

type TaskService interface {
	CreateTask(ctx context.Context, input usecase.CreateTaskInput) (*domain.Task, bool, error)
	GetTask(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error)
	ListTasks(ctx context.Context, orgID uuid.UUID, filter domain.TaskFilter) (*domain.TaskPage, error)
	RetryTask(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error)
	CancelTask(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error)
	SoftDeleteTask(ctx context.Context, orgID, id uuid.UUID) error
	QueueStats(ctx context.Context, orgID uuid.UUID, queue string) (*domain.QueueStats, error)
	ListDLQ(ctx context.Context, orgID uuid.UUID, queue, cursor string, limit int) (*domain.TaskPage, error)
	RequeueDLQ(ctx context.Context, orgID, id uuid.UUID) (*domain.Task, error)
	ListWorkers(ctx context.Context, orgID uuid.UUID) ([]*domain.Worker, error)
	WorkerHeartbeat(ctx context.Context, worker *domain.Worker) error
	Ready(ctx context.Context) error
}

type Handler struct {
	service                TaskService
	webhookService         WebhookService
	webhookDeliveryService WebhookDeliveryService
	workflowService        WorkflowService
	logger                 *zap.Logger
}

func (handler *Handler) WithWebhookDeliveryService(service WebhookDeliveryService) *Handler {
	handler.webhookDeliveryService = service
	return handler
}

func (handler *Handler) WithWorkflowService(service WorkflowService) *Handler {
	handler.workflowService = service
	return handler
}

func NewHandler(service TaskService, logger *zap.Logger, webhookServices ...WebhookService) *Handler {
	handler := &Handler{service: service, logger: logger}
	if len(webhookServices) > 0 {
		handler.webhookService = webhookServices[0]
	}
	return handler
}

type createTaskRequest struct {
	Queue                    string          `json:"queue"`
	Payload                  json.RawMessage `json:"payload"`
	Priority                 int             `json:"priority"`
	MaxRetries               *int            `json:"max_retries"`
	TimeoutSeconds           int             `json:"timeout_seconds"`
	VisibilityTimeoutSeconds int             `json:"visibility_timeout_seconds"`
	ScheduledAt              *time.Time      `json:"scheduled_at"`
	BackoffStrategy          string          `json:"backoff_strategy"`
}

type taskResponse struct {
	ID        uuid.UUID         `json:"id"`
	Status    domain.TaskStatus `json:"status"`
	Queue     string            `json:"queue"`
	CreatedAt time.Time         `json:"created_at"`
}

func toTaskResponse(task *domain.Task) taskResponse {
	return taskResponse{ID: task.ID, Status: task.Status, Queue: task.Queue, CreatedAt: task.CreatedAt}
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required", nil)
		return
	}
	if len(idempotencyKey) > 255 {
		writeAPIError(w, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must not exceed 255 characters", nil)
		return
	}

	var req createTaskRequest
	if err := decodeJSON(w, r, &req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large", nil)
			return
		}
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be a single valid JSON object", nil)
		return
	}
	if len(req.Payload) > maxPayloadBytes {
		writeAPIError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "payload must not exceed 262144 bytes", nil)
		return
	}
	if details := validateCreateTask(req); len(details) > 0 {
		writeAPIError(w, http.StatusBadRequest, "validation_failed", "request validation failed", details)
		return
	}

	maxRetries := 4
	if req.MaxRetries != nil {
		maxRetries = *req.MaxRetries
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 60
	}
	if req.VisibilityTimeoutSeconds == 0 {
		req.VisibilityTimeoutSeconds = 30
	}
	if req.BackoffStrategy == "" {
		req.BackoffStrategy = "exponential"
	}
	principal := MustPrincipal(r.Context())
	task, replay, err := h.service.CreateTask(r.Context(), usecase.CreateTaskInput{
		OrgID: principal.OrgID, IdempotencyKey: idempotencyKey, Queue: req.Queue,
		Payload: req.Payload, Priority: req.Priority, MaxRetries: maxRetries,
		Timeout:           time.Duration(req.TimeoutSeconds) * time.Second,
		VisibilityTimeout: time.Duration(req.VisibilityTimeoutSeconds) * time.Second,
		ScheduledAt:       req.ScheduledAt, BackoffStrategy: req.BackoffStrategy,
		TraceID: r.Header.Get("Trace-Id"),
	})
	if err != nil {
		if errors.Is(err, domain.ErrIdempotencyConflict) {
			writeAPIError(w, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was already used for a different request", nil)
			return
		}
		if errors.Is(err, domain.ErrDispatchUnavailable) {
			writeAPIError(w, http.StatusServiceUnavailable, "dispatch_unavailable", "task was persisted and will be dispatched by reconciliation", map[string]any{"task_id": task.ID})
			return
		}
		h.internalError(w, "create task", err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	writeJSON(w, status, toTaskResponse(task))
}

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTaskID(w, r.PathValue("id"))
	if !ok {
		return
	}
	task, err := h.service.GetTask(r.Context(), MustPrincipal(r.Context()).OrgID, id)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	limit, ok := parseLimit(w, r.URL.Query().Get("limit"))
	if !ok {
		return
	}
	status := domain.TaskStatus(r.URL.Query().Get("status"))
	if status != "" && !validStatus(status) {
		writeAPIError(w, http.StatusBadRequest, "invalid_status", "unsupported task status", nil)
		return
	}
	page, err := h.service.ListTasks(r.Context(), MustPrincipal(r.Context()).OrgID, domain.TaskFilter{
		Queue: r.URL.Query().Get("queue"), Status: status,
		Cursor: r.URL.Query().Get("cursor"), Limit: limit,
	})
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) RetryTask(w http.ResponseWriter, r *http.Request) {
	h.taskMutation(w, r, h.service.RetryTask)
}

func (h *Handler) CancelTask(w http.ResponseWriter, r *http.Request) {
	h.taskMutation(w, r, h.service.CancelTask)
}

func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTaskID(w, r.PathValue("id"))
	if !ok {
		return
	}
	err := h.service.SoftDeleteTask(r.Context(), MustPrincipal(r.Context()).OrgID, id)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) QueueStats(w http.ResponseWriter, r *http.Request) {
	queue := strings.TrimSpace(r.PathValue("name"))
	if queue == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_queue", "queue is required", nil)
		return
	}
	stats, err := h.service.QueueStats(r.Context(), MustPrincipal(r.Context()).OrgID, queue)
	if err != nil {
		h.internalError(w, "queue stats", err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) ListDLQ(w http.ResponseWriter, r *http.Request) {
	limit, ok := parseLimit(w, r.URL.Query().Get("limit"))
	if !ok {
		return
	}
	page, err := h.service.ListDLQ(r.Context(), MustPrincipal(r.Context()).OrgID,
		r.URL.Query().Get("queue"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) RequeueDLQ(w http.ResponseWriter, r *http.Request) {
	h.taskMutation(w, r, h.service.RequeueDLQ)
}

func (h *Handler) ListWorkers(w http.ResponseWriter, r *http.Request) {
	workers, err := h.service.ListWorkers(r.Context(), MustPrincipal(r.Context()).OrgID)
	if err != nil {
		h.internalError(w, "list workers", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": workers})
}

func (h *Handler) WorkerHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Queue  string `json:"queue"`
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
		return
	}
	if strings.TrimSpace(req.Queue) == "" {
		writeAPIError(w, http.StatusBadRequest, "validation_failed", "queue is required", nil)
		return
	}
	worker := &domain.Worker{
		ID: r.PathValue("id"), OrgID: MustPrincipal(r.Context()).OrgID,
		Queue: req.Queue, Status: req.Status,
	}
	if err := h.service.WorkerHeartbeat(r.Context(), worker); err != nil {
		h.internalError(w, "worker heartbeat", err)
		return
	}
	writeJSON(w, http.StatusOK, worker)
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Ready(r.Context()); err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "not_ready", "a required dependency is unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) StreamTaskEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "stream_unsupported", "streaming is unavailable", nil)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "retry: 3000\n\n")
	flusher.Flush()

	orgID := MustPrincipal(r.Context()).OrgID
	seen := make(map[uuid.UUID]string)
	poll := func() bool {
		page, err := h.service.ListTasks(r.Context(), orgID, domain.TaskFilter{Limit: 100})
		if err != nil {
			h.logger.Warn("task event poll failed", zap.Error(err))
			_, _ = io.WriteString(w, ": task poll temporarily unavailable\n\n")
			flusher.Flush()
			return true
		}
		for _, task := range page.Tasks {
			version := string(task.Status) + "|" + task.UpdatedAt.UTC().Format(time.RFC3339Nano)
			if seen[task.ID] == version {
				continue
			}
			seen[task.ID] = version
			data, err := json.Marshal(task)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: task\ndata: %s\n\n", data); err != nil {
				return false
			}
		}
		_, _ = io.WriteString(w, ": keepalive\n\n")
		flusher.Flush()
		return true
	}

	if !poll() {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !poll() {
				return
			}
		}
	}
}

func (h *Handler) taskMutation(w http.ResponseWriter, r *http.Request, fn func(context.Context, uuid.UUID, uuid.UUID) (*domain.Task, error)) {
	id, ok := parseTaskID(w, r.PathValue("id"))
	if !ok {
		return
	}
	task, err := fn(r.Context(), MustPrincipal(r.Context()).OrgID, id)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTaskResponse(task))
}

func (h *Handler) writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrTaskNotFound):
		writeAPIError(w, http.StatusNotFound, "task_not_found", "task not found", nil)
	case errors.Is(err, domain.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_cursor", "cursor is invalid", nil)
	case errors.Is(err, domain.ErrInvalidStateTransition):
		writeAPIError(w, http.StatusConflict, "invalid_state_transition", "operation is not valid for the current task state", nil)
	case errors.Is(err, domain.ErrDispatchUnavailable):
		writeAPIError(w, http.StatusServiceUnavailable, "dispatch_unavailable", "task state was persisted and reconciliation will retry dispatch", nil)
	default:
		h.internalError(w, "task operation", err)
	}
}

func (h *Handler) internalError(w http.ResponseWriter, operation string, err error) {
	h.logger.Error(operation, zap.Error(err))
	writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error", nil)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func parseTaskID(w http.ResponseWriter, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_task_id", "task id must be a UUID", nil)
		return uuid.Nil, false
	}
	return id, true
}

func parseLimit(w http.ResponseWriter, raw string) (int, bool) {
	if raw == "" {
		return 50, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		writeAPIError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100", nil)
		return 0, false
	}
	return limit, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{
		"code": code, "message": message, "details": details,
	}})
}
