package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/usecase"
	webhookinfra "github.com/shyxur/flowforge/internal/webhook"
	"go.uber.org/zap"
)

type webhookEndpointRepositoryStub struct {
	endpoints map[uuid.UUID]*domain.WebhookEndpoint
}

func newWebhookEndpointRepositoryStub() *webhookEndpointRepositoryStub {
	return &webhookEndpointRepositoryStub{endpoints: make(map[uuid.UUID]*domain.WebhookEndpoint)}
}

func (repository *webhookEndpointRepositoryStub) CreateWebhookEndpoint(_ context.Context, endpoint *domain.WebhookEndpoint) error {
	repository.endpoints[endpoint.ID] = cloneWebhookEndpoint(endpoint)
	return nil
}

func (repository *webhookEndpointRepositoryStub) ListWebhookEndpoints(_ context.Context, orgID uuid.UUID) ([]*domain.WebhookEndpoint, error) {
	items := make([]*domain.WebhookEndpoint, 0)
	for _, endpoint := range repository.endpoints {
		if endpoint.OrgID == orgID && endpoint.DeletedAt == nil {
			items = append(items, cloneWebhookEndpoint(endpoint))
		}
	}
	return items, nil
}

func (repository *webhookEndpointRepositoryStub) GetWebhookEndpoint(_ context.Context, orgID, id uuid.UUID) (*domain.WebhookEndpoint, error) {
	endpoint, exists := repository.endpoints[id]
	if !exists || endpoint.OrgID != orgID || endpoint.DeletedAt != nil {
		return nil, domain.ErrWebhookEndpointNotFound
	}
	return cloneWebhookEndpoint(endpoint), nil
}

func (repository *webhookEndpointRepositoryStub) UpdateWebhookEndpoint(_ context.Context, endpoint *domain.WebhookEndpoint) error {
	stored, exists := repository.endpoints[endpoint.ID]
	if !exists || stored.OrgID != endpoint.OrgID || stored.DeletedAt != nil {
		return domain.ErrWebhookEndpointNotFound
	}
	repository.endpoints[endpoint.ID] = cloneWebhookEndpoint(endpoint)
	return nil
}

func (repository *webhookEndpointRepositoryStub) SoftDeleteWebhookEndpoint(_ context.Context, orgID, id uuid.UUID, now time.Time) error {
	endpoint, exists := repository.endpoints[id]
	if !exists || endpoint.OrgID != orgID || endpoint.DeletedAt != nil {
		return domain.ErrWebhookEndpointNotFound
	}
	endpoint.DeletedAt = &now
	endpoint.IsActive = false
	endpoint.UpdatedAt = now
	return nil
}

func cloneWebhookEndpoint(endpoint *domain.WebhookEndpoint) *domain.WebhookEndpoint {
	cloned := *endpoint
	cloned.EventTypes = append([]domain.WebhookEventType(nil), endpoint.EventTypes...)
	return &cloned
}

func TestWebhookEndpointCRUDIsTenantScopedAndSecretSafe(t *testing.T) {
	repository := newWebhookEndpointRepositoryStub()
	webhookService := usecase.NewWebhookService(repository, webhookinfra.NewSecretCipher("test-key"), false)
	orgOne, orgTwo := uuid.New(), uuid.New()
	orgOneRouter := testWebhookRouter(webhookService, orgOne)
	orgTwoRouter := testWebhookRouter(webhookService, orgTwo)

	createResponse := performWebhookRequest(t, orgOneRouter, http.MethodPost, "/v1/webhooks/endpoints", `{
		"name":"Build events",
		"url":"https://example.com/hooks/tasks",
		"secret":"top-secret",
		"event_types":["task.created","task.completed"]
	}`)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", createResponse.Code, createResponse.Body.String())
	}
	var created map[string]any
	decodeWebhookResponse(t, createResponse, &created)
	if created["secret"] != "top-secret" {
		t.Fatalf("create response did not return secret once: %#v", created)
	}
	if _, exposed := created["secret_hash"]; exposed {
		t.Fatal("create response exposed secret_hash")
	}
	endpointID := created["id"].(string)
	storedID := uuid.MustParse(endpointID)
	if repository.endpoints[storedID].SecretHash == "" ||
		repository.endpoints[storedID].SecretHash == "top-secret" {
		t.Fatal("secret was not stored as a one-way hash")
	}

	otherCreate := performWebhookRequest(t, orgTwoRouter, http.MethodPost, "/v1/webhooks/endpoints", `{
		"name":"Other org",
		"url":"https://other.example/hooks",
		"secret":"other-secret",
		"event_types":["task.failed"]
	}`)
	if otherCreate.Code != http.StatusCreated {
		t.Fatalf("other create status = %d: %s", otherCreate.Code, otherCreate.Body.String())
	}

	listResponse := performWebhookRequest(t, orgOneRouter, http.MethodGet, "/v1/webhooks/endpoints", "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listResponse.Code, listResponse.Body.String())
	}
	var listBody struct {
		Items []map[string]any `json:"items"`
	}
	decodeWebhookResponse(t, listResponse, &listBody)
	if len(listBody.Items) != 1 || listBody.Items[0]["id"] != endpointID {
		t.Fatalf("tenant-scoped list = %#v", listBody.Items)
	}
	if _, exposed := listBody.Items[0]["secret"]; exposed {
		t.Fatal("list response exposed secret")
	}

	getResponse := performWebhookRequest(t, orgOneRouter, http.MethodGet, "/v1/webhooks/endpoints/"+endpointID, "")
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d: %s", getResponse.Code, getResponse.Body.String())
	}
	var got map[string]any
	decodeWebhookResponse(t, getResponse, &got)
	if _, exposed := got["secret"]; exposed {
		t.Fatal("get response exposed secret")
	}

	crossOrgResponse := performWebhookRequest(t, orgTwoRouter, http.MethodGet, "/v1/webhooks/endpoints/"+endpointID, "")
	if crossOrgResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d, want 404: %s", crossOrgResponse.Code, crossOrgResponse.Body.String())
	}

	updateResponse := performWebhookRequest(t, orgOneRouter, http.MethodPatch, "/v1/webhooks/endpoints/"+endpointID, `{
		"name":"Updated events",
		"is_active":false,
		"event_types":["task.failed","task.cancelled"]
	}`)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", updateResponse.Code, updateResponse.Body.String())
	}
	var updated map[string]any
	decodeWebhookResponse(t, updateResponse, &updated)
	if updated["name"] != "Updated events" || updated["is_active"] != false {
		t.Fatalf("updated response = %#v", updated)
	}

	rotateResponse := performWebhookRequest(t, orgOneRouter, http.MethodPost, "/v1/webhooks/endpoints/"+endpointID+"/rotate-secret", "")
	if rotateResponse.Code != http.StatusOK {
		t.Fatalf("rotate status = %d: %s", rotateResponse.Code, rotateResponse.Body.String())
	}
	var rotated map[string]string
	decodeWebhookResponse(t, rotateResponse, &rotated)
	newSecret := rotated["secret"]
	if newSecret == "" || newSecret == "top-secret" {
		t.Fatalf("rotation secret = %q", newSecret)
	}
	signer := webhookinfra.HMACSigner{}
	oldSignature := signer.Sign("top-secret", "1722326400", []byte(`{"task":"one"}`))
	if signer.Verify(newSecret, "1722326400", []byte(`{"task":"one"}`), oldSignature) {
		t.Fatal("old signature remained valid after secret rotation")
	}

	postRotationGet := performWebhookRequest(t, orgOneRouter, http.MethodGet, "/v1/webhooks/endpoints/"+endpointID, "")
	var postRotationBody map[string]any
	decodeWebhookResponse(t, postRotationGet, &postRotationBody)
	if _, exposed := postRotationBody["secret"]; exposed {
		t.Fatal("get response exposed rotated secret")
	}

	deleteResponse := performWebhookRequest(t, orgOneRouter, http.MethodDelete, "/v1/webhooks/endpoints/"+endpointID, "")
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if repository.endpoints[storedID].DeletedAt == nil {
		t.Fatal("endpoint was not soft deleted")
	}
	deletedGet := performWebhookRequest(t, orgOneRouter, http.MethodGet, "/v1/webhooks/endpoints/"+endpointID, "")
	if deletedGet.Code != http.StatusNotFound {
		t.Fatalf("deleted get status = %d, want 404", deletedGet.Code)
	}
}

func TestWebhookEndpointRejectsInvalidURLsAndEvents(t *testing.T) {
	router := testWebhookRouter(newTestWebhookService(newWebhookEndpointRepositoryStub(), false), uuid.New())
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing URL",
			body: `{"name":"x","secret":"s","event_types":["task.created"]}`,
		},
		{
			name: "unsupported scheme",
			body: `{"name":"x","url":"ftp://example.com/hook","secret":"s","event_types":["task.created"]}`,
		},
		{
			name: "insecure remote URL",
			body: `{"name":"x","url":"http://example.com/hook","secret":"s","event_types":["task.created"]}`,
		},
		{
			name: "unknown event",
			body: `{"name":"x","url":"https://example.com/hook","secret":"s","event_types":["task.unknown"]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performWebhookRequest(t, router, http.MethodPost, "/v1/webhooks/endpoints", test.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestWebhookEndpointAllowsInsecureLocalURLOnlyWhenEnabled(t *testing.T) {
	body := `{"name":"local","url":"http://127.0.0.1:9000/hook","secret":"s","event_types":["task.created"]}`
	disabled := testWebhookRouter(newTestWebhookService(newWebhookEndpointRepositoryStub(), false), uuid.New())
	if response := performWebhookRequest(t, disabled, http.MethodPost, "/v1/webhooks/endpoints", body); response.Code != http.StatusBadRequest {
		t.Fatalf("disabled status = %d, want 400", response.Code)
	}

	enabled := testWebhookRouter(newTestWebhookService(newWebhookEndpointRepositoryStub(), true), uuid.New())
	if response := performWebhookRequest(t, enabled, http.MethodPost, "/v1/webhooks/endpoints", body); response.Code != http.StatusCreated {
		t.Fatalf("enabled status = %d, want 201: %s", response.Code, response.Body.String())
	}
}

func newTestWebhookService(repository *webhookEndpointRepositoryStub, allowInsecure bool) *usecase.WebhookService {
	return usecase.NewWebhookService(repository, webhookinfra.NewSecretCipher("test-key"), allowInsecure)
}

func testWebhookRouter(webhookService WebhookService, orgID uuid.UUID) http.Handler {
	return NewRouter(
		NewHandler(&serviceStub{}, zap.NewNop(), webhookService),
		allowAuth(orgID),
		nil,
		zap.NewNop(),
	)
}

func performWebhookRequest(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody *bytes.Reader
	if body == "" {
		requestBody = bytes.NewReader(nil)
	} else {
		requestBody = bytes.NewReader([]byte(body))
	}
	request := httptest.NewRequest(method, path, requestBody)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeWebhookResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(response.Body.String()))
	if err := decoder.Decode(destination); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
}
