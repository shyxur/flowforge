package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/usecase"
)

type WorkflowExecutionService interface {
	StartExecution(
		context.Context,
		usecase.StartWorkflowExecutionInput,
	) (*domain.WorkflowExecutionStartResult, bool, error)
	ListExecutions(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		domain.WorkflowExecutionFilter,
	) (*domain.WorkflowExecutionPage, error)
	GetExecution(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		uuid.UUID,
	) (*domain.WorkflowExecutionDetail, error)
	CancelExecution(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		uuid.UUID,
	) (*domain.WorkflowExecution, error)
}

type startWorkflowExecutionRequest struct {
	Version *int            `json:"version"`
	Input   json.RawMessage `json:"input"`
}

func (handler *Handler) StartWorkflowExecution(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := parseWorkflowID(w, r.PathValue("id"))
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required", nil)
		return
	}
	if len(idempotencyKey) > 255 {
		writeAPIError(w, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must not exceed 255 characters", nil)
		return
	}
	var request startWorkflowExecutionRequest
	if !decodeWorkflowRequest(w, r, &request) {
		return
	}
	if len(request.Input) > maxPayloadBytes {
		writeAPIError(w, http.StatusRequestEntityTooLarge, "payload_too_large",
			"workflow execution input must not exceed 262144 bytes", nil)
		return
	}
	result, _, err := handler.workflowExecutionService.StartExecution(
		r.Context(),
		usecase.StartWorkflowExecutionInput{
			OrgID:          MustPrincipal(r.Context()).OrgID,
			WorkflowID:     workflowID,
			Version:        request.Version,
			Input:          request.Input,
			IdempotencyKey: idempotencyKey,
		},
	)
	if err != nil {
		handler.writeWorkflowExecutionError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (handler *Handler) ListWorkflowExecutions(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := parseWorkflowID(w, r.PathValue("id"))
	if !ok {
		return
	}
	limit, ok := parseLimit(w, r.URL.Query().Get("limit"))
	if !ok {
		return
	}
	status := domain.WorkflowExecutionStatus(r.URL.Query().Get("status"))
	if status != "" && !status.Valid() {
		writeAPIError(w, http.StatusBadRequest, "invalid_status",
			"status must be pending, running, succeeded, failed, or cancelled", nil)
		return
	}
	page, err := handler.workflowExecutionService.ListExecutions(
		r.Context(),
		MustPrincipal(r.Context()).OrgID,
		workflowID,
		domain.WorkflowExecutionFilter{
			Status: status,
			Cursor: r.URL.Query().Get("cursor"),
			Limit:  limit,
		},
	)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidInput) {
			writeAPIError(w, http.StatusBadRequest, "invalid_cursor", "cursor is invalid", nil)
			return
		}
		handler.writeWorkflowExecutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (handler *Handler) GetWorkflowExecution(w http.ResponseWriter, r *http.Request) {
	workflowID, executionID, ok := parseWorkflowExecutionIDs(w, r)
	if !ok {
		return
	}
	result, err := handler.workflowExecutionService.GetExecution(
		r.Context(), MustPrincipal(r.Context()).OrgID, workflowID, executionID,
	)
	if err != nil {
		handler.writeWorkflowExecutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) CancelWorkflowExecution(w http.ResponseWriter, r *http.Request) {
	workflowID, executionID, ok := parseWorkflowExecutionIDs(w, r)
	if !ok {
		return
	}
	result, err := handler.workflowExecutionService.CancelExecution(
		r.Context(), MustPrincipal(r.Context()).OrgID, workflowID, executionID,
	)
	if err != nil {
		handler.writeWorkflowExecutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseWorkflowExecutionIDs(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, uuid.UUID, bool) {
	workflowID, ok := parseWorkflowID(w, r.PathValue("id"))
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	executionID, err := uuid.Parse(r.PathValue("execution_id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_workflow_execution_id",
			"workflow execution id must be a UUID", nil)
		return uuid.Nil, uuid.Nil, false
	}
	return workflowID, executionID, true
}

func (handler *Handler) writeWorkflowExecutionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrWorkflowExecutionNotFound):
		writeAPIError(w, http.StatusNotFound, "workflow_execution_not_found", "workflow execution not found", nil)
	case errors.Is(err, domain.ErrWorkflowNotPublished):
		writeAPIError(w, http.StatusConflict, "workflow_not_published", "workflow must be published before execution", nil)
	case errors.Is(err, domain.ErrWorkflowVersionNotFound):
		writeAPIError(w, http.StatusNotFound, "workflow_version_not_found", "workflow version not found", nil)
	case errors.Is(err, domain.ErrWorkflowExecutionTerminal):
		writeAPIError(w, http.StatusConflict, "workflow_execution_terminal", "terminal workflow executions cannot be cancelled", nil)
	case errors.Is(err, domain.ErrWorkflowExecutionIdempotencyConflict):
		writeAPIError(w, http.StatusConflict, "idempotency_conflict",
			"Idempotency-Key was already used for a different workflow execution request", nil)
	case errors.Is(err, domain.ErrWorkflowNotFound):
		handler.writeWorkflowError(w, err)
	case errors.Is(err, domain.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "validation_failed", "request validation failed", nil)
	default:
		handler.internalError(w, "workflow execution operation", err)
	}
}
