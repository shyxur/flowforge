package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
)

type WebhookEndpointRepository interface {
	CreateWebhookEndpoint(ctx context.Context, endpoint *domain.WebhookEndpoint) error
	ListWebhookEndpoints(ctx context.Context, orgID uuid.UUID) ([]*domain.WebhookEndpoint, error)
	GetWebhookEndpoint(ctx context.Context, orgID, id uuid.UUID) (*domain.WebhookEndpoint, error)
	UpdateWebhookEndpoint(ctx context.Context, endpoint *domain.WebhookEndpoint) error
	SoftDeleteWebhookEndpoint(ctx context.Context, orgID, id uuid.UUID, now time.Time) error
	ListActiveWebhookEndpointsForEvent(ctx context.Context, orgID uuid.UUID, eventType domain.WebhookEventType) ([]*domain.WebhookEndpoint, error)
}

type WebhookDeliveryRepository interface {
	CreateWebhookDelivery(ctx context.Context, delivery *domain.WebhookDelivery) error
	GetWebhookDelivery(ctx context.Context, orgID, id uuid.UUID) (*domain.WebhookDelivery, error)
	ListWebhookDeliveries(ctx context.Context, orgID uuid.UUID, filter domain.WebhookDeliveryFilter) (*domain.WebhookDeliveryPage, error)
	ListDueWebhookDeliveries(ctx context.Context, now time.Time, limit int) ([]*domain.WebhookDelivery, error)
	ClaimDueWebhookDeliveries(ctx context.Context, now time.Time, limit int) ([]*domain.WebhookDelivery, error)
	UpdateWebhookDelivery(ctx context.Context, delivery *domain.WebhookDelivery) error
	RetryWebhookDelivery(ctx context.Context, orgID, id uuid.UUID, now time.Time) (*domain.WebhookDelivery, error)
}

type TaskEventPublisher interface {
	PublishTaskEvent(ctx context.Context, eventType domain.WebhookEventType, task *domain.Task) error
}

type WebhookHTTPRequest struct {
	URL     string
	Headers map[string]string
	Body    []byte
}

type WebhookHTTPResponse struct {
	StatusCode int
	Body       string
}

type WebhookHTTPClient interface {
	Send(ctx context.Context, request WebhookHTTPRequest) (*WebhookHTTPResponse, error)
}

type WebhookSecretCipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type WebhookSigner interface {
	Sign(secret, timestamp string, payload []byte) string
	Verify(secret, timestamp string, payload []byte, signature string) bool
}
