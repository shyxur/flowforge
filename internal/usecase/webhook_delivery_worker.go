package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/shyxur/windylane/internal/domain"
	metricspkg "github.com/shyxur/windylane/internal/metrics"
	"github.com/shyxur/windylane/internal/ports"
)

type WebhookDeliveryWorker struct {
	endpoints      ports.WebhookEndpointRepository
	deliveries     ports.WebhookDeliveryRepository
	secretCipher   ports.WebhookSecretCipher
	signer         ports.WebhookSigner
	client         ports.WebhookHTTPClient
	batchSize      int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	metrics        ports.MetricRecorder
}

func (worker *WebhookDeliveryWorker) WithMetricRecorder(recorder ports.MetricRecorder) *WebhookDeliveryWorker {
	worker.metrics = recorder
	return worker
}

func NewWebhookDeliveryWorker(
	endpoints ports.WebhookEndpointRepository,
	deliveries ports.WebhookDeliveryRepository,
	secretCipher ports.WebhookSecretCipher,
	signer ports.WebhookSigner,
	client ports.WebhookHTTPClient,
	batchSize int,
	initialBackoff, maxBackoff time.Duration,
) *WebhookDeliveryWorker {
	if batchSize <= 0 {
		batchSize = 50
	}
	if initialBackoff <= 0 {
		initialBackoff = 5 * time.Second
	}
	if maxBackoff <= 0 {
		maxBackoff = time.Hour
	}
	return &WebhookDeliveryWorker{
		endpoints: endpoints, deliveries: deliveries, secretCipher: secretCipher,
		signer: signer, client: client, batchSize: batchSize,
		initialBackoff: initialBackoff, maxBackoff: maxBackoff,
	}
}

func (worker *WebhookDeliveryWorker) ProcessDue(ctx context.Context, now time.Time) (int, error) {
	deliveries, err := worker.deliveries.ClaimDueWebhookDeliveries(ctx, now, worker.batchSize)
	if err != nil {
		return 0, err
	}
	var processErrors []error
	for _, delivery := range deliveries {
		worker.recordMetric(delivery, domain.MetricDeliveryStarted, now, "")
		if err := worker.process(ctx, delivery, now); err != nil {
			processErrors = append(processErrors, err)
		}
	}
	return len(deliveries), errors.Join(processErrors...)
}

func (worker *WebhookDeliveryWorker) process(ctx context.Context, delivery *domain.WebhookDelivery, now time.Time) error {
	endpoint, err := worker.endpoints.GetWebhookEndpoint(ctx, delivery.OrgID, delivery.EndpointID)
	if err != nil {
		return worker.recordFailure(ctx, delivery, nil, err, now)
	}
	if !endpoint.IsActive {
		return worker.recordFailure(ctx, delivery, nil, errors.New("webhook endpoint is inactive"), now)
	}
	secret, err := worker.secretCipher.Decrypt(endpoint.SecretCiphertext)
	if err != nil {
		return worker.recordFailure(ctx, delivery, nil, err, now)
	}

	timestamp := strconv.FormatInt(now.Unix(), 10)
	response, sendErr := worker.client.Send(ctx, ports.WebhookHTTPRequest{
		URL: endpoint.URL,
		Headers: map[string]string{
			"X-Windylane-Event":     string(delivery.EventType),
			"X-Windylane-Delivery":  delivery.ID.String(),
			"X-Windylane-Timestamp": timestamp,
			"X-Windylane-Signature": worker.signer.Sign(secret, timestamp, delivery.Payload),
		},
		Body: delivery.Payload,
	})
	if sendErr != nil {
		return worker.recordFailure(ctx, delivery, nil, sendErr, now)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return worker.recordFailure(
			ctx, delivery, response,
			fmt.Errorf("webhook returned HTTP %d", response.StatusCode),
			now,
		)
	}

	delivery.Status = domain.WebhookDeliveryDelivered
	delivery.NextAttemptAt = nil
	delivery.ResponseStatus = &response.StatusCode
	delivery.ResponseBody = &response.Body
	delivery.LastError = nil
	delivery.UpdatedAt = now
	if err := worker.deliveries.UpdateWebhookDelivery(ctx, delivery); err != nil {
		return err
	}
	worker.recordMetric(delivery, domain.MetricDeliverySucceeded, now, "")
	return nil
}

func (worker *WebhookDeliveryWorker) recordFailure(
	ctx context.Context,
	delivery *domain.WebhookDelivery,
	response *ports.WebhookHTTPResponse,
	deliveryErr error,
	now time.Time,
) error {
	message := deliveryErr.Error()
	delivery.LastError = &message
	delivery.UpdatedAt = now
	if response != nil {
		delivery.ResponseStatus = &response.StatusCode
		delivery.ResponseBody = &response.Body
	} else {
		delivery.ResponseStatus = nil
		delivery.ResponseBody = nil
	}
	if delivery.AttemptCount >= delivery.MaxAttempts {
		delivery.Status = domain.WebhookDeliveryFailed
		delivery.NextAttemptAt = nil
	} else {
		delivery.Status = domain.WebhookDeliveryRetrying
		nextAttemptAt := now.Add(worker.backoff(delivery.AttemptCount))
		delivery.NextAttemptAt = &nextAttemptAt
	}
	if err := worker.deliveries.UpdateWebhookDelivery(ctx, delivery); err != nil {
		return errors.Join(deliveryErr, err)
	}
	worker.recordMetric(delivery, domain.MetricDeliveryFailed, now, "delivery_error")
	if delivery.Status == domain.WebhookDeliveryFailed {
		worker.recordMetric(delivery, domain.MetricDeliveryExhausted, now, "delivery_error")
	} else {
		worker.recordMetric(delivery, domain.MetricDeliveryRetryScheduled, now, "delivery_error")
	}
	return nil
}

func (worker *WebhookDeliveryWorker) recordMetric(
	delivery *domain.WebhookDelivery,
	eventType domain.MetricEventType,
	now time.Time,
	errorCode string,
) {
	attempt, maxAttempts := delivery.AttemptCount, delivery.MaxAttempts
	metricspkg.Record(worker.metrics, domain.NewMetricEventInput{
		OrganizationID: delivery.OrgID, Source: domain.MetricSourceEventForge,
		EventType: eventType, ResourceType: domain.MetricResourceWebhookDelivery,
		ResourceID: delivery.ID.String(), Status: string(delivery.Status), OccurredAt: now,
		Metadata: domain.MetricMetadata{
			Attempt: &attempt, MaxAttempts: &maxAttempts, ErrorCode: errorCode,
		},
		TransitionKey: domain.MetricTransitionKey(delivery.AttemptCount, eventType),
	})
}

func (worker *WebhookDeliveryWorker) backoff(attempt int) time.Duration {
	backoff := worker.initialBackoff
	for current := 1; current < attempt && backoff < worker.maxBackoff; current++ {
		if backoff > worker.maxBackoff/2 {
			return worker.maxBackoff
		}
		backoff *= 2
	}
	if backoff > worker.maxBackoff {
		return worker.maxBackoff
	}
	return backoff
}
