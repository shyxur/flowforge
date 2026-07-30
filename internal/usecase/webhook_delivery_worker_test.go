package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/ports"
	"github.com/shyxur/windylane/internal/testutil"
	webhookinfra "github.com/shyxur/windylane/internal/webhook"
)

type webhookHTTPClientFunc func(context.Context, ports.WebhookHTTPRequest) (*ports.WebhookHTTPResponse, error)

func (function webhookHTTPClientFunc) Send(ctx context.Context, request ports.WebhookHTTPRequest) (*ports.WebhookHTTPResponse, error) {
	return function(ctx, request)
}

func TestWebhookDeliveryWorkerMarks2xxDeliveredAndSendsSignedHeaders(t *testing.T) {
	now := time.Unix(1722326400, 0).UTC()
	orgID, endpointID := uuid.New(), uuid.New()
	delivery := claimedDelivery(orgID, endpointID, 1, 5)
	cipher := webhookinfra.NewSecretCipher("test-key")
	ciphertext, err := cipher.Encrypt("signing-secret")
	if err != nil {
		t.Fatal(err)
	}
	var updated *domain.WebhookDelivery
	deliveryRepository := workerDeliveryRepository(delivery, &updated)
	endpointRepository := &testutil.WebhookEndpointRepositoryStub{
		GetFunc: func(_ context.Context, gotOrgID, gotEndpointID uuid.UUID) (*domain.WebhookEndpoint, error) {
			if gotOrgID != orgID || gotEndpointID != endpointID {
				t.Fatalf("tenant endpoint lookup = %s/%s", gotOrgID, gotEndpointID)
			}
			return &domain.WebhookEndpoint{
				ID: endpointID, OrgID: orgID, URL: "https://example.com/hook",
				IsActive: true, SecretCiphertext: ciphertext,
			}, nil
		},
	}
	signer := webhookinfra.HMACSigner{}
	client := webhookHTTPClientFunc(func(_ context.Context, request ports.WebhookHTTPRequest) (*ports.WebhookHTTPResponse, error) {
		if request.Headers["X-Windylane-Event"] != string(delivery.EventType) ||
			request.Headers["X-Windylane-Delivery"] != delivery.ID.String() ||
			request.Headers["X-Windylane-Timestamp"] != "1722326400" {
			t.Fatalf("headers = %#v", request.Headers)
		}
		if !signer.Verify(
			"signing-secret",
			request.Headers["X-Windylane-Timestamp"],
			request.Body,
			request.Headers["X-Windylane-Signature"],
		) {
			t.Fatal("delivery signature is invalid")
		}
		return &ports.WebhookHTTPResponse{StatusCode: 204, Body: "ok"}, nil
	})

	metrics := &metricRecorderSpy{}
	worker := NewWebhookDeliveryWorker(
		endpointRepository, deliveryRepository, cipher, signer, client,
		10, time.Second, time.Minute,
	).WithMetricRecorder(metrics)
	count, err := worker.ProcessDue(context.Background(), now)
	if err != nil || count != 1 {
		t.Fatalf("process count=%d err=%v", count, err)
	}
	if updated == nil || updated.Status != domain.WebhookDeliveryDelivered ||
		updated.ResponseStatus == nil || *updated.ResponseStatus != 204 ||
		updated.NextAttemptAt != nil || updated.LastError != nil {
		t.Fatalf("updated delivery = %+v", updated)
	}
	if !metrics.has(domain.MetricDeliveryStarted) || !metrics.has(domain.MetricDeliverySucceeded) {
		t.Fatal("delivery success lifecycle metrics were not recorded")
	}
}

func TestWebhookDeliveryWorkerSchedulesRetryForHTTPAndNetworkFailures(t *testing.T) {
	tests := []struct {
		name     string
		response *ports.WebhookHTTPResponse
		err      error
	}{
		{name: "HTTP 500", response: &ports.WebhookHTTPResponse{StatusCode: 500, Body: "temporary"}},
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "network", err: errors.New("connection refused")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			orgID, endpointID := uuid.New(), uuid.New()
			delivery := claimedDelivery(orgID, endpointID, 1, 5)
			cipher := webhookinfra.NewSecretCipher("test-key")
			ciphertext, err := cipher.Encrypt("secret")
			if err != nil {
				t.Fatal(err)
			}
			var updated *domain.WebhookDelivery
			worker := NewWebhookDeliveryWorker(
				&testutil.WebhookEndpointRepositoryStub{
					GetFunc: func(context.Context, uuid.UUID, uuid.UUID) (*domain.WebhookEndpoint, error) {
						return &domain.WebhookEndpoint{
							ID: endpointID, OrgID: orgID, URL: "https://example.com",
							IsActive: true, SecretCiphertext: ciphertext,
						}, nil
					},
				},
				workerDeliveryRepository(delivery, &updated),
				cipher,
				webhookinfra.HMACSigner{},
				webhookHTTPClientFunc(func(context.Context, ports.WebhookHTTPRequest) (*ports.WebhookHTTPResponse, error) {
					return test.response, test.err
				}),
				10, 2*time.Second, time.Minute,
			)
			if count, err := worker.ProcessDue(context.Background(), now); err != nil || count != 1 {
				t.Fatalf("process count=%d err=%v", count, err)
			}
			if updated.Status != domain.WebhookDeliveryRetrying ||
				updated.NextAttemptAt == nil ||
				!updated.NextAttemptAt.Equal(now.Add(2*time.Second)) ||
				updated.LastError == nil {
				t.Fatalf("updated delivery = %+v", updated)
			}
		})
	}
}

func TestWebhookDeliveryWorkerMarksFailedAtMaxAttempts(t *testing.T) {
	now := time.Now().UTC()
	orgID, endpointID := uuid.New(), uuid.New()
	delivery := claimedDelivery(orgID, endpointID, 5, 5)
	cipher := webhookinfra.NewSecretCipher("test-key")
	ciphertext, err := cipher.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	var updated *domain.WebhookDelivery
	metrics := &metricRecorderSpy{}
	worker := NewWebhookDeliveryWorker(
		&testutil.WebhookEndpointRepositoryStub{
			GetFunc: func(context.Context, uuid.UUID, uuid.UUID) (*domain.WebhookEndpoint, error) {
				return &domain.WebhookEndpoint{
					ID: endpointID, OrgID: orgID, URL: "https://example.com",
					IsActive: true, SecretCiphertext: ciphertext,
				}, nil
			},
		},
		workerDeliveryRepository(delivery, &updated),
		cipher,
		webhookinfra.HMACSigner{},
		webhookHTTPClientFunc(func(context.Context, ports.WebhookHTTPRequest) (*ports.WebhookHTTPResponse, error) {
			return &ports.WebhookHTTPResponse{StatusCode: 503, Body: "unavailable"}, nil
		}),
		10, time.Second, time.Minute,
	).WithMetricRecorder(metrics)
	if _, err := worker.ProcessDue(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.WebhookDeliveryFailed || updated.NextAttemptAt != nil ||
		updated.AttemptCount != updated.MaxAttempts {
		t.Fatalf("updated delivery = %+v", updated)
	}
	if !metrics.has(domain.MetricDeliveryFailed) || !metrics.has(domain.MetricDeliveryExhausted) {
		t.Fatal("delivery exhausted lifecycle metrics were not recorded")
	}
}

func claimedDelivery(orgID, endpointID uuid.UUID, attemptCount, maxAttempts int) *domain.WebhookDelivery {
	now := time.Now().UTC()
	return &domain.WebhookDelivery{
		ID: uuid.New(), OrgID: orgID, EndpointID: endpointID,
		EventType:    domain.WebhookEventTaskCreated,
		Payload:      []byte(`{"event_id":"test"}`),
		Status:       domain.WebhookDeliveryDelivering,
		AttemptCount: attemptCount, MaxAttempts: maxAttempts,
		LastAttemptAt: &now, CreatedAt: now, UpdatedAt: now,
	}
}

func workerDeliveryRepository(
	delivery *domain.WebhookDelivery,
	updated **domain.WebhookDelivery,
) *testutil.WebhookDeliveryRepositoryStub {
	return &testutil.WebhookDeliveryRepositoryStub{
		ClaimDueFunc: func(context.Context, time.Time, int) ([]*domain.WebhookDelivery, error) {
			return []*domain.WebhookDelivery{delivery}, nil
		},
		UpdateFunc: func(_ context.Context, value *domain.WebhookDelivery) error {
			cloned := *value
			*updated = &cloned
			return nil
		},
	}
}
