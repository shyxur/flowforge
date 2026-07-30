//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shyxur/windylane/internal/api"
	redisbroker "github.com/shyxur/windylane/internal/broker/redis"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/storage/postgres"
	"github.com/shyxur/windylane/internal/usecase"
	webhookinfra "github.com/shyxur/windylane/internal/webhook"
	"go.uber.org/zap"
)

const integrationAPIKey = "queueflow-dev-key"

type capturedWebhookRequest struct {
	Header http.Header
	Body   []byte
}

type webhookEndpointAPIResponse struct {
	ID     uuid.UUID `json:"id"`
	Secret string    `json:"secret"`
}

type webhookDeliveryAPIResponse struct {
	ID             uuid.UUID                    `json:"id"`
	EndpointID     uuid.UUID                    `json:"endpoint_id"`
	EventType      domain.WebhookEventType      `json:"event_type"`
	Status         domain.WebhookDeliveryStatus `json:"status"`
	AttemptCount   int                          `json:"attempt_count"`
	MaxAttempts    int                          `json:"max_attempts"`
	NextAttemptAt  *time.Time                   `json:"next_attempt_at"`
	ResponseStatus *int                         `json:"response_status"`
	Payload        json.RawMessage              `json:"payload"`
}

type webhookDeliveryListAPIResponse struct {
	Items []webhookDeliveryAPIResponse `json:"items"`
}

func TestEventForgeLifecycle(t *testing.T) {
	if os.Getenv("QUEUEFLOW_INTEGRATION") != "1" {
		t.Skip("run with make test-integration")
	}

	ctx := context.Background()
	dsn := os.Getenv("INTEGRATION_DB_DSN")
	redisAddr := os.Getenv("INTEGRATION_REDIS_ADDR")
	if dsn == "" || redisAddr == "" {
		t.Fatal("integration database and Redis addresses are required")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "TRUNCATE workflow_node_executions, workflow_executions, workflow_versions, workflows, webhook_deliveries, webhook_endpoints, tasks, workers"); err != nil {
		t.Fatal(err)
	}
	assertMigrationsAndSeed(t, ctx, pool)

	storage, err := postgres.NewPostgresStorage(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	broker := redisbroker.NewRedisBroker(redisAddr, "", 0)
	defer broker.Close()
	if err := broker.Client().FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	var responseStatus atomic.Int32
	responseStatus.Store(http.StatusNoContent)
	requests := make(chan capturedWebhookRequest, 8)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)
			return
		}
		requests <- capturedWebhookRequest{Header: r.Header.Clone(), Body: body}
		w.WriteHeader(int(responseStatus.Load()))
	}))
	defer target.Close()

	cipher := webhookinfra.NewSecretCipher("eventforge-integration-encryption-key")
	endpointService := usecase.NewWebhookService(storage, cipher, true)
	deliveryLogService := usecase.NewWebhookDeliveryLogService(storage)
	eventService := usecase.NewWebhookEventService(storage, storage, 5)
	taskService := usecase.NewService(storage, broker, eventService)
	handler := api.NewHandler(taskService, zap.NewNop(), endpointService).
		WithWebhookDeliveryService(deliveryLogService)
	router := api.NewRouter(handler, usecase.NewAuthService(storage), nil, zap.NewNop())

	activeEndpoint, originalSecret := createWebhookEndpoint(
		t, router, integrationAPIKey, target.URL, "active", true,
		[]domain.WebhookEventType{domain.WebhookEventTaskCreated},
	)
	if originalSecret == "" {
		t.Fatal("create did not return the endpoint secret")
	}
	assertEndpointSecretHidden(t, router, integrationAPIKey, activeEndpoint)

	rotate := eventForgeRequest(
		t, router, integrationAPIKey, http.MethodPost,
		"/v1/webhooks/endpoints/"+activeEndpoint.String()+"/rotate-secret", nil,
	)
	assertStatus(t, rotate, http.StatusOK)
	var rotatedSecretResponse struct {
		Secret string `json:"secret"`
	}
	decodeResponse(t, rotate, &rotatedSecretResponse)
	if rotatedSecretResponse.Secret == "" || rotatedSecretResponse.Secret == originalSecret {
		t.Fatal("secret rotation did not return a new secret")
	}
	assertEndpointSecretHidden(t, router, integrationAPIKey, activeEndpoint)

	createWebhookEndpoint(
		t, router, integrationAPIKey, target.URL, "inactive", false,
		[]domain.WebhookEventType{domain.WebhookEventTaskCreated},
	)
	createWebhookEndpoint(
		t, router, integrationAPIKey, target.URL, "filtered", true,
		[]domain.WebhookEventType{domain.WebhookEventTaskFailed},
	)

	orgA := uuid.MustParse(devOrgID)
	_, replay, err := taskService.CreateTask(ctx, usecase.CreateTaskInput{
		OrgID: orgA, IdempotencyKey: "eventforge-task-created", Queue: "eventforge-integration",
		Payload: json.RawMessage(`{"source":"eventforge-integration"}`), MaxRetries: 4,
		Timeout: time.Second, VisibilityTimeout: 30 * time.Second, BackoffStrategy: "exponential",
	})
	if err != nil || replay {
		t.Fatalf("create task lifecycle event: replay=%v err=%v", replay, err)
	}

	pending := listWebhookDeliveries(
		t, router, integrationAPIKey,
		"?status=pending&event_type=task.created&endpoint_id="+activeEndpoint.String(),
	)
	if len(pending.Items) != 1 || pending.Items[0].EndpointID != activeEndpoint {
		t.Fatalf("inactive or filtered endpoint received delivery: %+v", pending.Items)
	}
	successDeliveryID := pending.Items[0].ID

	signer := webhookinfra.HMACSigner{}
	worker := usecase.NewWebhookDeliveryWorker(
		storage, storage, cipher, signer,
		webhookinfra.NewHTTPClient(2*time.Second, webhookinfra.DefaultResponseBodyLimit),
		10, time.Second, time.Minute,
	)
	successAttemptAt := time.Now().UTC().Add(time.Second)
	processed, err := worker.ProcessDue(ctx, successAttemptAt)
	if err != nil || processed != 1 {
		t.Fatalf("process successful delivery: processed=%d err=%v", processed, err)
	}
	successRequest := receiveWebhookRequest(t, requests)
	assertSignedWebhookRequest(
		t, signer, rotatedSecretResponse.Secret, successDeliveryID,
		domain.WebhookEventTaskCreated, successRequest,
	)
	successDelivery := getWebhookDelivery(t, router, integrationAPIKey, successDeliveryID)
	if successDelivery.Status != domain.WebhookDeliveryDelivered ||
		successDelivery.ResponseStatus == nil || *successDelivery.ResponseStatus != http.StatusNoContent {
		t.Fatalf("successful delivery state: %+v", successDelivery)
	}
	var sentPayload, persistedPayload any
	if err := json.Unmarshal(successRequest.Body, &sentPayload); err != nil {
		t.Fatalf("decode sent webhook payload: %v", err)
	}
	if err := json.Unmarshal(successDelivery.Payload, &persistedPayload); err != nil {
		t.Fatalf("decode persisted webhook payload: %v", err)
	}
	if !reflect.DeepEqual(sentPayload, persistedPayload) {
		t.Fatal("worker request body differs from persisted raw JSON payload")
	}

	responseStatus.Store(http.StatusInternalServerError)
	if err := eventService.PublishTaskEvent(ctx, domain.WebhookEventTaskCreated, integrationWebhookTask(orgA)); err != nil {
		t.Fatalf("publish retrying task.created: %v", err)
	}
	retryAttemptAt := successAttemptAt.Add(time.Second)
	processed, err = worker.ProcessDue(ctx, retryAttemptAt)
	if err != nil || processed != 1 {
		t.Fatalf("process retrying delivery: processed=%d err=%v", processed, err)
	}
	_ = receiveWebhookRequest(t, requests)
	retrying := listWebhookDeliveries(
		t, router, integrationAPIKey,
		"?status=retrying&endpoint_id="+activeEndpoint.String(),
	)
	if len(retrying.Items) != 1 {
		t.Fatalf("retrying deliveries = %+v", retrying.Items)
	}
	if retrying.Items[0].AttemptCount != 1 || retrying.Items[0].NextAttemptAt == nil ||
		retrying.Items[0].ResponseStatus == nil ||
		*retrying.Items[0].ResponseStatus != http.StatusInternalServerError {
		t.Fatalf("500 response did not schedule retry: %+v", retrying.Items[0])
	}

	maxAttemptEventService := usecase.NewWebhookEventService(storage, storage, 1)
	if err := maxAttemptEventService.PublishTaskEvent(
		ctx, domain.WebhookEventTaskCreated, integrationWebhookTask(orgA),
	); err != nil {
		t.Fatalf("publish max-attempt task.created: %v", err)
	}
	failedAttemptAt := retryAttemptAt.Add(500 * time.Millisecond)
	processed, err = worker.ProcessDue(ctx, failedAttemptAt)
	if err != nil || processed != 1 {
		t.Fatalf("process max-attempt delivery: processed=%d err=%v", processed, err)
	}
	_ = receiveWebhookRequest(t, requests)
	failed := listWebhookDeliveries(
		t, router, integrationAPIKey,
		"?status=failed&endpoint_id="+activeEndpoint.String(),
	)
	if len(failed.Items) != 1 || failed.Items[0].AttemptCount != 1 ||
		failed.Items[0].MaxAttempts != 1 || failed.Items[0].NextAttemptAt != nil {
		t.Fatalf("max-attempt delivery state = %+v", failed.Items)
	}
	failedDeliveryID := failed.Items[0].ID

	manualRetry := eventForgeRequest(
		t, router, integrationAPIKey, http.MethodPost,
		"/v1/webhooks/deliveries/"+failedDeliveryID.String()+"/retry", nil,
	)
	assertStatus(t, manualRetry, http.StatusOK)
	var retried webhookDeliveryAPIResponse
	decodeResponse(t, manualRetry, &retried)
	if retried.Status != domain.WebhookDeliveryPending || retried.AttemptCount != 0 {
		t.Fatalf("manual retry state = %+v", retried)
	}

	orgB, apiKeyB := createIntegrationTenant(t, ctx, pool)
	endpointB, secretB := createWebhookEndpoint(
		t, router, apiKeyB, target.URL, "tenant-b", true,
		[]domain.WebhookEventType{domain.WebhookEventTaskCreated},
	)
	if secretB == "" {
		t.Fatal("tenant B create did not return a secret")
	}
	assertNotFound(
		t, eventForgeRequest(t, router, integrationAPIKey, http.MethodGet,
			"/v1/webhooks/endpoints/"+endpointB.String(), nil),
	)
	assertNotFound(
		t, eventForgeRequest(t, router, apiKeyB, http.MethodGet,
			"/v1/webhooks/endpoints/"+activeEndpoint.String(), nil),
	)

	if err := eventService.PublishTaskEvent(ctx, domain.WebhookEventTaskCreated, integrationWebhookTask(orgB)); err != nil {
		t.Fatalf("publish tenant B task.created: %v", err)
	}
	deliveriesB := listWebhookDeliveries(t, router, apiKeyB, "")
	if len(deliveriesB.Items) != 1 || deliveriesB.Items[0].EndpointID != endpointB {
		t.Fatalf("tenant B delivery list = %+v", deliveriesB.Items)
	}
	deliveryB := deliveriesB.Items[0].ID
	assertNotFound(
		t, eventForgeRequest(t, router, integrationAPIKey, http.MethodGet,
			"/v1/webhooks/deliveries/"+deliveryB.String(), nil),
	)
	assertNotFound(
		t, eventForgeRequest(t, router, apiKeyB, http.MethodGet,
			"/v1/webhooks/deliveries/"+successDeliveryID.String(), nil),
	)
	for _, delivery := range listWebhookDeliveries(t, router, integrationAPIKey, "").Items {
		if delivery.ID == deliveryB {
			t.Fatal("tenant A delivery list exposed tenant B delivery")
		}
	}
}

func createWebhookEndpoint(
	t *testing.T,
	router http.Handler,
	apiKey, targetURL, name string,
	active bool,
	eventTypes []domain.WebhookEventType,
) (uuid.UUID, string) {
	t.Helper()
	body := map[string]any{
		"name": name, "url": targetURL, "event_types": eventTypes, "is_active": active,
	}
	response := eventForgeRequest(
		t, router, apiKey, http.MethodPost, "/v1/webhooks/endpoints", body,
	)
	assertStatus(t, response, http.StatusCreated)
	var endpoint webhookEndpointAPIResponse
	decodeResponse(t, response, &endpoint)
	if endpoint.ID == uuid.Nil {
		t.Fatal("create endpoint returned an empty id")
	}
	return endpoint.ID, endpoint.Secret
}

func assertEndpointSecretHidden(t *testing.T, router http.Handler, apiKey string, endpointID uuid.UUID) {
	t.Helper()
	get := eventForgeRequest(
		t, router, apiKey, http.MethodGet,
		"/v1/webhooks/endpoints/"+endpointID.String(), nil,
	)
	assertStatus(t, get, http.StatusOK)
	var endpoint map[string]json.RawMessage
	decodeResponse(t, get, &endpoint)
	assertNoSecretFields(t, endpoint)

	list := eventForgeRequest(t, router, apiKey, http.MethodGet, "/v1/webhooks/endpoints", nil)
	assertStatus(t, list, http.StatusOK)
	var page struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	decodeResponse(t, list, &page)
	if len(page.Items) == 0 {
		t.Fatal("endpoint list is empty")
	}
	for _, item := range page.Items {
		assertNoSecretFields(t, item)
	}
}

func assertNoSecretFields(t *testing.T, response map[string]json.RawMessage) {
	t.Helper()
	for _, field := range []string{"secret", "secret_hash", "secret_ciphertext"} {
		if _, ok := response[field]; ok {
			t.Fatalf("API exposed %s", field)
		}
	}
}

func integrationWebhookTask(orgID uuid.UUID) *domain.Task {
	now := time.Now().UTC()
	return &domain.Task{
		ID: uuid.New(), OrgID: orgID, Queue: "eventforge-integration",
		Status: domain.StatusPending, MaxAttempts: 5, CreatedAt: now, UpdatedAt: now,
	}
}

func eventForgeRequest(
	t *testing.T,
	router http.Handler,
	apiKey, method, path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, requestBody)
	request.Header.Set("Authorization", "Bearer "+apiKey)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body.String())
	}
}

func assertNotFound(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	assertStatus(t, response, http.StatusNotFound)
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
}

func listWebhookDeliveries(
	t *testing.T,
	router http.Handler,
	apiKey, query string,
) webhookDeliveryListAPIResponse {
	t.Helper()
	response := eventForgeRequest(
		t, router, apiKey, http.MethodGet, "/v1/webhooks/deliveries"+query, nil,
	)
	assertStatus(t, response, http.StatusOK)
	var page webhookDeliveryListAPIResponse
	decodeResponse(t, response, &page)
	return page
}

func getWebhookDelivery(
	t *testing.T,
	router http.Handler,
	apiKey string,
	id uuid.UUID,
) webhookDeliveryAPIResponse {
	t.Helper()
	response := eventForgeRequest(
		t, router, apiKey, http.MethodGet, "/v1/webhooks/deliveries/"+id.String(), nil,
	)
	assertStatus(t, response, http.StatusOK)
	var delivery webhookDeliveryAPIResponse
	decodeResponse(t, response, &delivery)
	return delivery
}

func receiveWebhookRequest(
	t *testing.T,
	requests <-chan capturedWebhookRequest,
) capturedWebhookRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(2 * time.Second):
		t.Fatal("webhook request was not received")
		return capturedWebhookRequest{}
	}
}

func assertSignedWebhookRequest(
	t *testing.T,
	signer webhookinfra.HMACSigner,
	secret string,
	deliveryID uuid.UUID,
	eventType domain.WebhookEventType,
	request capturedWebhookRequest,
) {
	t.Helper()
	eventHeader := request.Header.Get("X-Windylane-Event")
	deliveryHeader := request.Header.Get("X-Windylane-Delivery")
	timestampHeader := request.Header.Get("X-Windylane-Timestamp")
	signatureHeader := request.Header.Get("X-Windylane-Signature")
	if eventHeader != string(eventType) {
		t.Fatalf("event header=%q want=%q", eventHeader, eventType)
	}
	if deliveryHeader != deliveryID.String() {
		t.Fatalf("delivery header=%q want=%q", deliveryHeader, deliveryID)
	}
	if timestampHeader == "" || signatureHeader == "" {
		t.Fatalf("missing webhook headers: timestamp=%q signature=%q", timestampHeader, signatureHeader)
	}
	if _, err := strconv.ParseInt(timestampHeader, 10, 64); err != nil {
		t.Fatalf("invalid timestamp header %q: %v", timestampHeader, err)
	}
	if !signer.Verify(secret, timestampHeader, request.Body, signatureHeader) {
		t.Fatal("webhook HMAC signature is invalid")
	}
}

func createIntegrationTenant(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) (uuid.UUID, string) {
	t.Helper()
	orgID := uuid.New()
	apiKey := "eventforge-integration-" + uuid.NewString()
	keyHash := sha256.Sum256([]byte(apiKey))
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO organizations (id, name) VALUES ($1, $2)",
		orgID, "EventForge integration tenant",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO api_keys (id, org_id, name, key_hash, key_prefix, scopes)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		uuid.New(), orgID, "EventForge integration key",
		hex.EncodeToString(keyHash[:]), apiKey[:8], []string{"webhooks:read", "webhooks:write"},
	); err != nil {
		t.Fatal(err)
	}
	return orgID, apiKey
}
