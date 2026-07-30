package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/usecase"
)

type WorkflowService interface {
	CreateWorkflow(context.Context, usecase.CreateWorkflowInput) (*domain.Workflow, error)
	ListWorkflows(context.Context, uuid.UUID, domain.WorkflowFilter) (*domain.WorkflowPage, error)
	GetWorkflow(context.Context, uuid.UUID, uuid.UUID) (*domain.Workflow, error)
	UpdateWorkflow(context.Context, uuid.UUID, uuid.UUID, usecase.UpdateWorkflowInput) (*domain.Workflow, error)
	DeleteWorkflow(context.Context, uuid.UUID, uuid.UUID) error
}

type createWorkflowRequest struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description *string         `json:"description"`
	Definition  json.RawMessage `json:"definition"`
}

type updateWorkflowRequest struct {
	Name        *string         `json:"name"`
	Slug        *string         `json:"slug"`
	Description json.RawMessage `json:"description"`
	Definition  json.RawMessage `json:"definition"`
}

func (h *Handler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var request createWorkflowRequest
	if !decodeWorkflowRequest(w, r, &request) {
		return
	}
	workflow, err := h.workflowService.CreateWorkflow(r.Context(), usecase.CreateWorkflowInput{
		OrgID: MustPrincipal(r.Context()).OrgID, Name: request.Name, Slug: request.Slug,
		Description: request.Description, Definition: request.Definition,
	})
	if err != nil {
		h.writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, workflow)
}

func (h *Handler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	limit, ok := parseLimit(w, r.URL.Query().Get("limit"))
	if !ok {
		return
	}
	status := domain.WorkflowStatus(r.URL.Query().Get("status"))
	if status != "" && !status.Valid() {
		writeAPIError(w, http.StatusBadRequest, "invalid_status", "status must be draft, published, or archived", nil)
		return
	}
	page, err := h.workflowService.ListWorkflows(r.Context(), MustPrincipal(r.Context()).OrgID, domain.WorkflowFilter{
		Status: status, Cursor: r.URL.Query().Get("cursor"), Limit: limit,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidInput) {
			writeAPIError(w, http.StatusBadRequest, "invalid_cursor", "cursor is invalid", nil)
			return
		}
		h.writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	id, ok := parseWorkflowID(w, r.PathValue("id"))
	if !ok {
		return
	}
	workflow, err := h.workflowService.GetWorkflow(r.Context(), MustPrincipal(r.Context()).OrgID, id)
	if err != nil {
		h.writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workflow)
}

func (h *Handler) UpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	id, ok := parseWorkflowID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var request updateWorkflowRequest
	if !decodeWorkflowRequest(w, r, &request) {
		return
	}
	descriptionSet := len(request.Description) > 0
	var description *string
	if descriptionSet && !bytes.Equal(bytes.TrimSpace(request.Description), []byte("null")) {
		var value string
		if err := json.Unmarshal(request.Description, &value); err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_failed", "request validation failed", map[string]any{"description": "must be a string or null"})
			return
		}
		description = &value
	}
	workflow, err := h.workflowService.UpdateWorkflow(r.Context(), MustPrincipal(r.Context()).OrgID, id, usecase.UpdateWorkflowInput{
		Name: request.Name, Slug: request.Slug, DescriptionSet: descriptionSet,
		Description: description, Definition: request.Definition,
	})
	if err != nil {
		h.writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workflow)
}

func (h *Handler) DeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	id, ok := parseWorkflowID(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := h.workflowService.DeleteWorkflow(r.Context(), MustPrincipal(r.Context()).OrgID, id); err != nil {
		h.writeWorkflowError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeWorkflowRequest(w http.ResponseWriter, r *http.Request, destination any) bool {
	if err := decodeJSON(w, r, destination); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large", nil)
			return false
		}
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be a single valid JSON object", nil)
		return false
	}
	return true
}

func parseWorkflowID(w http.ResponseWriter, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_workflow_id", "workflow id must be a UUID", nil)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) writeWorkflowError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrWorkflowNotFound):
		writeAPIError(w, http.StatusNotFound, "workflow_not_found", "workflow not found", nil)
	case errors.Is(err, domain.ErrWorkflowSlugConflict):
		writeAPIError(w, http.StatusConflict, "workflow_slug_conflict", "workflow slug already exists", nil)
	case errors.Is(err, domain.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "validation_failed", "request validation failed", nil)
	case errors.Is(err, domain.ErrInvalidStateTransition):
		writeAPIError(w, http.StatusConflict, "workflow_not_editable", "only draft workflows can be updated", nil)
	default:
		h.internalError(w, "workflow operation", err)
	}
}
