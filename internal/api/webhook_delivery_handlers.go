package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/flowforge/internal/domain"
)

type WebhookDeliveryService interface {
	ListDeliveries(ctx context.Context, orgID uuid.UUID, filter domain.WebhookDeliveryFilter) (*domain.WebhookDeliveryPage, error)
	GetDelivery(ctx context.Context, orgID, id uuid.UUID) (*domain.WebhookDelivery, error)
	RetryDelivery(ctx context.Context, orgID, id uuid.UUID) (*domain.WebhookDelivery, error)
}

type webhookDeliveryResponse struct {
	ID             uuid.UUID                    `json:"id"`
	EndpointID     uuid.UUID                    `json:"endpoint_id"`
	EventType      domain.WebhookEventType      `json:"event_type"`
	Status         domain.WebhookDeliveryStatus `json:"status"`
	AttemptCount   int                          `json:"attempt_count"`
	MaxAttempts    int                          `json:"max_attempts"`
	NextAttemptAt  *time.Time                   `json:"next_attempt_at,omitempty"`
	LastAttemptAt  *time.Time                   `json:"last_attempt_at,omitempty"`
	ResponseStatus *int                         `json:"response_status,omitempty"`
	ResponseBody   *string                      `json:"response_body,omitempty"`
	LastError      *string                      `json:"last_error,omitempty"`
	CreatedAt      time.Time                    `json:"created_at"`
	UpdatedAt      time.Time                    `json:"updated_at"`
}

type webhookDeliveryDetailResponse struct {
	webhookDeliveryResponse
	Payload json.RawMessage `json:"payload"`
}

func toWebhookDeliveryResponse(delivery *domain.WebhookDelivery) webhookDeliveryResponse {
	responseBody := delivery.ResponseBody
	if responseBody != nil && len(*responseBody) > 4096 {
		truncated := (*responseBody)[:4096]
		responseBody = &truncated
	}
	return webhookDeliveryResponse{
		ID: delivery.ID, EndpointID: delivery.EndpointID,
		EventType: delivery.EventType, Status: delivery.Status,
		AttemptCount: delivery.AttemptCount, MaxAttempts: delivery.MaxAttempts,
		NextAttemptAt: delivery.NextAttemptAt, LastAttemptAt: delivery.LastAttemptAt,
		ResponseStatus: delivery.ResponseStatus, ResponseBody: responseBody,
		LastError: delivery.LastError, CreatedAt: delivery.CreatedAt, UpdatedAt: delivery.UpdatedAt,
	}
}

func (h *Handler) ListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	limit, ok := parseLimit(w, r.URL.Query().Get("limit"))
	if !ok {
		return
	}
	var endpointID *uuid.UUID
	if rawEndpointID := r.URL.Query().Get("endpoint_id"); rawEndpointID != "" {
		parsed, err := uuid.Parse(rawEndpointID)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_webhook_endpoint_id", "endpoint_id must be a UUID", nil)
			return
		}
		endpointID = &parsed
	}
	status := domain.WebhookDeliveryStatus(r.URL.Query().Get("status"))
	eventType := domain.WebhookEventType(r.URL.Query().Get("event_type"))
	page, err := h.webhookDeliveryService.ListDeliveries(
		r.Context(),
		MustPrincipal(r.Context()).OrgID,
		domain.WebhookDeliveryFilter{
			EndpointID: endpointID, Status: status, EventType: eventType,
			Cursor: r.URL.Query().Get("cursor"), Limit: limit,
		},
	)
	if err != nil {
		h.writeWebhookDeliveryError(w, "list webhook deliveries", err)
		return
	}
	items := make([]webhookDeliveryResponse, len(page.Deliveries))
	for index, delivery := range page.Deliveries {
		items[index] = toWebhookDeliveryResponse(delivery)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "next_cursor": page.NextCursor,
	})
}

func (h *Handler) GetWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	id, ok := parseWebhookDeliveryID(w, r.PathValue("id"))
	if !ok {
		return
	}
	delivery, err := h.webhookDeliveryService.GetDelivery(
		r.Context(), MustPrincipal(r.Context()).OrgID, id,
	)
	if err != nil {
		h.writeWebhookDeliveryError(w, "get webhook delivery", err)
		return
	}
	writeJSON(w, http.StatusOK, webhookDeliveryDetailResponse{
		webhookDeliveryResponse: toWebhookDeliveryResponse(delivery),
		Payload:                 delivery.Payload,
	})
}

func (h *Handler) RetryWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	id, ok := parseWebhookDeliveryID(w, r.PathValue("id"))
	if !ok {
		return
	}
	delivery, err := h.webhookDeliveryService.RetryDelivery(
		r.Context(), MustPrincipal(r.Context()).OrgID, id,
	)
	if err != nil {
		h.writeWebhookDeliveryError(w, "retry webhook delivery", err)
		return
	}
	writeJSON(w, http.StatusOK, toWebhookDeliveryResponse(delivery))
}

func parseWebhookDeliveryID(w http.ResponseWriter, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_webhook_delivery_id", "webhook delivery id must be a UUID", nil)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) writeWebhookDeliveryError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, domain.ErrWebhookDeliveryNotFound):
		writeAPIError(w, http.StatusNotFound, "webhook_delivery_not_found", "webhook delivery not found", nil)
	case errors.Is(err, domain.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "validation_failed", "webhook delivery filter is invalid", nil)
	case errors.Is(err, domain.ErrInvalidStateTransition):
		writeAPIError(w, http.StatusConflict, "invalid_state_transition", "webhook delivery cannot be retried in its current state", nil)
	default:
		h.internalError(w, operation, err)
	}
}
