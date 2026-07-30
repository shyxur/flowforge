package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/usecase"
)

type WebhookService interface {
	CreateEndpoint(ctx context.Context, input usecase.CreateWebhookEndpointInput) (*usecase.CreateWebhookEndpointResult, error)
	ListEndpoints(ctx context.Context, orgID uuid.UUID) ([]*domain.WebhookEndpoint, error)
	GetEndpoint(ctx context.Context, orgID, id uuid.UUID) (*domain.WebhookEndpoint, error)
	UpdateEndpoint(ctx context.Context, orgID, id uuid.UUID, input usecase.UpdateWebhookEndpointInput) (*domain.WebhookEndpoint, error)
	DeleteEndpoint(ctx context.Context, orgID, id uuid.UUID) error
	RotateSecret(ctx context.Context, orgID, id uuid.UUID) (string, error)
}

type createWebhookEndpointRequest struct {
	Name       string                    `json:"name"`
	URL        string                    `json:"url"`
	Secret     string                    `json:"secret"`
	EventTypes []domain.WebhookEventType `json:"event_types"`
	IsActive   *bool                     `json:"is_active"`
}

type updateWebhookEndpointRequest struct {
	Name       *string                    `json:"name"`
	URL        *string                    `json:"url"`
	Secret     *string                    `json:"secret"`
	EventTypes *[]domain.WebhookEventType `json:"event_types"`
	IsActive   *bool                      `json:"is_active"`
}

type webhookEndpointResponse struct {
	ID         uuid.UUID                 `json:"id"`
	OrgID      uuid.UUID                 `json:"org_id"`
	Name       string                    `json:"name"`
	URL        string                    `json:"url"`
	EventTypes []domain.WebhookEventType `json:"event_types"`
	IsActive   bool                      `json:"is_active"`
	CreatedAt  time.Time                 `json:"created_at"`
	UpdatedAt  time.Time                 `json:"updated_at"`
}

type createWebhookEndpointResponse struct {
	webhookEndpointResponse
	Secret string `json:"secret"`
}

func toWebhookEndpointResponse(endpoint *domain.WebhookEndpoint) webhookEndpointResponse {
	return webhookEndpointResponse{
		ID: endpoint.ID, OrgID: endpoint.OrgID, Name: endpoint.Name, URL: endpoint.URL,
		EventTypes: endpoint.EventTypes, IsActive: endpoint.IsActive,
		CreatedAt: endpoint.CreatedAt, UpdatedAt: endpoint.UpdatedAt,
	}
}

func (h *Handler) CreateWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	var request createWebhookEndpointRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be a single valid JSON object", nil)
		return
	}
	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}
	result, err := h.webhookService.CreateEndpoint(r.Context(), usecase.CreateWebhookEndpointInput{
		OrgID: MustPrincipal(r.Context()).OrgID, Name: request.Name, URL: request.URL,
		Secret: request.Secret, EventTypes: request.EventTypes, IsActive: isActive,
	})
	if err != nil {
		h.writeWebhookError(w, "create webhook endpoint", err)
		return
	}
	writeJSON(w, http.StatusCreated, createWebhookEndpointResponse{
		webhookEndpointResponse: toWebhookEndpointResponse(result.Endpoint),
		Secret:                  result.Secret,
	})
}

func (h *Handler) ListWebhookEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints, err := h.webhookService.ListEndpoints(r.Context(), MustPrincipal(r.Context()).OrgID)
	if err != nil {
		h.writeWebhookError(w, "list webhook endpoints", err)
		return
	}
	items := make([]webhookEndpointResponse, len(endpoints))
	for i, endpoint := range endpoints {
		items[i] = toWebhookEndpointResponse(endpoint)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	id, ok := parseWebhookEndpointID(w, r.PathValue("id"))
	if !ok {
		return
	}
	endpoint, err := h.webhookService.GetEndpoint(r.Context(), MustPrincipal(r.Context()).OrgID, id)
	if err != nil {
		h.writeWebhookError(w, "get webhook endpoint", err)
		return
	}
	writeJSON(w, http.StatusOK, toWebhookEndpointResponse(endpoint))
}

func (h *Handler) UpdateWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	id, ok := parseWebhookEndpointID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var request updateWebhookEndpointRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be a single valid JSON object", nil)
		return
	}
	endpoint, err := h.webhookService.UpdateEndpoint(
		r.Context(), MustPrincipal(r.Context()).OrgID, id,
		usecase.UpdateWebhookEndpointInput{
			Name: request.Name, URL: request.URL, Secret: request.Secret,
			EventTypes: request.EventTypes, IsActive: request.IsActive,
		},
	)
	if err != nil {
		h.writeWebhookError(w, "update webhook endpoint", err)
		return
	}
	writeJSON(w, http.StatusOK, toWebhookEndpointResponse(endpoint))
}

func (h *Handler) DeleteWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	id, ok := parseWebhookEndpointID(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := h.webhookService.DeleteEndpoint(r.Context(), MustPrincipal(r.Context()).OrgID, id); err != nil {
		h.writeWebhookError(w, "delete webhook endpoint", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RotateWebhookEndpointSecret(w http.ResponseWriter, r *http.Request) {
	id, ok := parseWebhookEndpointID(w, r.PathValue("id"))
	if !ok {
		return
	}
	secret, err := h.webhookService.RotateSecret(r.Context(), MustPrincipal(r.Context()).OrgID, id)
	if err != nil {
		h.writeWebhookError(w, "rotate webhook endpoint secret", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret})
}

func parseWebhookEndpointID(w http.ResponseWriter, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_webhook_endpoint_id", "webhook endpoint id must be a UUID", nil)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) writeWebhookError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, domain.ErrWebhookEndpointNotFound):
		writeAPIError(w, http.StatusNotFound, "webhook_endpoint_not_found", "webhook endpoint not found", nil)
	case errors.Is(err, domain.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "validation_failed", "webhook endpoint validation failed", nil)
	default:
		h.internalError(w, operation, err)
	}
}
