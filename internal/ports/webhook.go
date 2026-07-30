package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/flowforge/internal/domain"
)

type WebhookEndpointRepository interface {
	CreateWebhookEndpoint(ctx context.Context, endpoint *domain.WebhookEndpoint) error
	ListWebhookEndpoints(ctx context.Context, orgID uuid.UUID) ([]*domain.WebhookEndpoint, error)
	GetWebhookEndpoint(ctx context.Context, orgID, id uuid.UUID) (*domain.WebhookEndpoint, error)
	UpdateWebhookEndpoint(ctx context.Context, endpoint *domain.WebhookEndpoint) error
	SoftDeleteWebhookEndpoint(ctx context.Context, orgID, id uuid.UUID, now time.Time) error
}

type WebhookDeliveryRepository interface {
	CreateWebhookDelivery(ctx context.Context, delivery *domain.WebhookDelivery) error
	GetWebhookDelivery(ctx context.Context, orgID, id uuid.UUID) (*domain.WebhookDelivery, error)
	ListDueWebhookDeliveries(ctx context.Context, now time.Time, limit int) ([]*domain.WebhookDelivery, error)
	UpdateWebhookDelivery(ctx context.Context, delivery *domain.WebhookDelivery) error
}

type WebhookSecretCipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type WebhookSigner interface {
	Sign(secret, timestamp string, payload []byte) string
	Verify(secret, timestamp string, payload []byte, signature string) bool
}
