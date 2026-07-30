package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/testutil"
	"github.com/shyxur/flowforge/internal/usecase"
	"go.uber.org/zap"
)

func TestWebhookDeliveryLogsAreFilteredTenantScopedAndRetryable(t *testing.T) {
	orgID, otherOrgID := uuid.New(), uuid.New()
	endpointID, otherEndpointID := uuid.New(), uuid.New()
	now := time.Now().UTC()
	failedID, deliveredID, otherOrgDeliveryID := uuid.New(), uuid.New(), uuid.New()
	deliveries := map[uuid.UUID]*domain.WebhookDelivery{
		failedID: {
			ID: failedID, OrgID: orgID, EndpointID: endpointID,
			EventType: domain.WebhookEventTaskFailed, Status: domain.WebhookDeliveryFailed,
			AttemptCount: 5, MaxAttempts: 5, CreatedAt: now, UpdatedAt: now,
			Payload: []byte(`{"event_id":"failed-event"}`),
		},
		deliveredID: {
			ID: deliveredID, OrgID: orgID, EndpointID: otherEndpointID,
			EventType: domain.WebhookEventTaskCreated, Status: domain.WebhookDeliveryDelivered,
			AttemptCount: 1, MaxAttempts: 5, CreatedAt: now.Add(-time.Minute), UpdatedAt: now,
		},
		otherOrgDeliveryID: {
			ID: otherOrgDeliveryID, OrgID: otherOrgID, EndpointID: endpointID,
			EventType: domain.WebhookEventTaskFailed, Status: domain.WebhookDeliveryFailed,
			AttemptCount: 5, MaxAttempts: 5, CreatedAt: now, UpdatedAt: now,
		},
	}
	repository := &testutil.WebhookDeliveryRepositoryStub{
		GetFunc: func(_ context.Context, gotOrgID, id uuid.UUID) (*domain.WebhookDelivery, error) {
			delivery, exists := deliveries[id]
			if !exists || delivery.OrgID != gotOrgID {
				return nil, domain.ErrWebhookDeliveryNotFound
			}
			cloned := *delivery
			return &cloned, nil
		},
		ListFunc: func(_ context.Context, gotOrgID uuid.UUID, filter domain.WebhookDeliveryFilter) (*domain.WebhookDeliveryPage, error) {
			page := &domain.WebhookDeliveryPage{}
			for _, delivery := range deliveries {
				if delivery.OrgID != gotOrgID ||
					filter.EndpointID != nil && delivery.EndpointID != *filter.EndpointID ||
					filter.Status != "" && delivery.Status != filter.Status ||
					filter.EventType != "" && delivery.EventType != filter.EventType {
					continue
				}
				cloned := *delivery
				page.Deliveries = append(page.Deliveries, &cloned)
			}
			return page, nil
		},
		RetryFunc: func(_ context.Context, gotOrgID, id uuid.UUID, retryAt time.Time) (*domain.WebhookDelivery, error) {
			delivery, exists := deliveries[id]
			if !exists || delivery.OrgID != gotOrgID {
				return nil, domain.ErrWebhookDeliveryNotFound
			}
			delivery.Status = domain.WebhookDeliveryPending
			delivery.AttemptCount = 0
			delivery.NextAttemptAt = &retryAt
			delivery.LastAttemptAt = nil
			delivery.LastError = nil
			delivery.ResponseStatus = nil
			delivery.ResponseBody = nil
			delivery.UpdatedAt = retryAt
			cloned := *delivery
			return &cloned, nil
		},
	}
	service := usecase.NewWebhookDeliveryLogService(repository)
	orgRouter := testWebhookDeliveryRouter(service, orgID)
	otherOrgRouter := testWebhookDeliveryRouter(service, otherOrgID)

	listPath := "/v1/webhooks/deliveries?endpoint_id=" + endpointID.String() +
		"&status=failed&event_type=task.failed&limit=10"
	listResponse := performWebhookRequest(t, orgRouter, http.MethodGet, listPath, "")
	if listResponse.Code != http.StatusOK ||
		!strings.Contains(listResponse.Body.String(), failedID.String()) ||
		strings.Contains(listResponse.Body.String(), deliveredID.String()) ||
		strings.Contains(listResponse.Body.String(), otherOrgDeliveryID.String()) {
		t.Fatalf("list status/body = %d %s", listResponse.Code, listResponse.Body.String())
	}

	getResponse := performWebhookRequest(t, orgRouter, http.MethodGet, "/v1/webhooks/deliveries/"+failedID.String(), "")
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d: %s", getResponse.Code, getResponse.Body.String())
	}
	if !strings.Contains(getResponse.Body.String(), `"event_id":"failed-event"`) {
		t.Fatalf("get response omitted payload: %s", getResponse.Body.String())
	}
	crossOrgGet := performWebhookRequest(t, otherOrgRouter, http.MethodGet, "/v1/webhooks/deliveries/"+failedID.String(), "")
	if crossOrgGet.Code != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d, want 404", crossOrgGet.Code)
	}

	retryResponse := performWebhookRequest(t, orgRouter, http.MethodPost, "/v1/webhooks/deliveries/"+failedID.String()+"/retry", "")
	if retryResponse.Code != http.StatusOK ||
		!strings.Contains(retryResponse.Body.String(), `"status":"pending"`) ||
		!strings.Contains(retryResponse.Body.String(), `"attempt_count":0`) {
		t.Fatalf("retry status/body = %d %s", retryResponse.Code, retryResponse.Body.String())
	}

	deliveredRetry := performWebhookRequest(t, orgRouter, http.MethodPost, "/v1/webhooks/deliveries/"+deliveredID.String()+"/retry", "")
	if deliveredRetry.Code != http.StatusConflict {
		t.Fatalf("delivered retry status = %d, want 409", deliveredRetry.Code)
	}
}

func TestWebhookDeliveryLogsRejectInvalidFilters(t *testing.T) {
	service := usecase.NewWebhookDeliveryLogService(&testutil.WebhookDeliveryRepositoryStub{})
	router := testWebhookDeliveryRouter(service, uuid.New())
	for _, path := range []string{
		"/v1/webhooks/deliveries?endpoint_id=bad",
		"/v1/webhooks/deliveries?status=unknown",
		"/v1/webhooks/deliveries?event_type=task.unknown",
	} {
		response := performWebhookRequest(t, router, http.MethodGet, path, "")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", path, response.Code)
		}
	}
}

func testWebhookDeliveryRouter(service WebhookDeliveryService, orgID uuid.UUID) http.Handler {
	handler := NewHandler(&serviceStub{}, zap.NewNop()).
		WithWebhookDeliveryService(service)
	return NewRouter(handler, allowAuth(orgID), nil, zap.NewNop())
}
