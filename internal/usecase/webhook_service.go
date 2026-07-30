package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/ports"
)

type WebhookService struct {
	repository         ports.WebhookEndpointRepository
	allowInsecureLocal bool
}

func NewWebhookService(repository ports.WebhookEndpointRepository, allowInsecureLocal bool) *WebhookService {
	return &WebhookService{repository: repository, allowInsecureLocal: allowInsecureLocal}
}

type CreateWebhookEndpointInput struct {
	OrgID      uuid.UUID
	Name       string
	URL        string
	Secret     string
	EventTypes []domain.WebhookEventType
	IsActive   bool
}

type UpdateWebhookEndpointInput struct {
	Name       *string
	URL        *string
	Secret     *string
	EventTypes *[]domain.WebhookEventType
	IsActive   *bool
}

func (s *WebhookService) CreateEndpoint(ctx context.Context, input CreateWebhookEndpointInput) (*domain.WebhookEndpoint, error) {
	name := strings.TrimSpace(input.Name)
	endpointURL := strings.TrimSpace(input.URL)
	if name == "" || len(name) > 255 || strings.TrimSpace(input.Secret) == "" {
		return nil, domain.ErrInvalidInput
	}
	if err := s.validateURL(endpointURL); err != nil {
		return nil, err
	}
	eventTypes, err := normalizeWebhookEventTypes(input.EventTypes)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	endpoint := &domain.WebhookEndpoint{
		ID:         uuid.New(),
		OrgID:      input.OrgID,
		Name:       name,
		URL:        endpointURL,
		SecretHash: hashWebhookSecret(input.Secret),
		EventTypes: eventTypes,
		IsActive:   input.IsActive,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repository.CreateWebhookEndpoint(ctx, endpoint); err != nil {
		return nil, err
	}
	return endpoint, nil
}

func (s *WebhookService) ListEndpoints(ctx context.Context, orgID uuid.UUID) ([]*domain.WebhookEndpoint, error) {
	return s.repository.ListWebhookEndpoints(ctx, orgID)
}

func (s *WebhookService) GetEndpoint(ctx context.Context, orgID, id uuid.UUID) (*domain.WebhookEndpoint, error) {
	return s.repository.GetWebhookEndpoint(ctx, orgID, id)
}

func (s *WebhookService) UpdateEndpoint(ctx context.Context, orgID, id uuid.UUID, input UpdateWebhookEndpointInput) (*domain.WebhookEndpoint, error) {
	if input.Name == nil && input.URL == nil && input.Secret == nil && input.EventTypes == nil && input.IsActive == nil {
		return nil, domain.ErrInvalidInput
	}
	endpoint, err := s.repository.GetWebhookEndpoint(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" || len(name) > 255 {
			return nil, domain.ErrInvalidInput
		}
		endpoint.Name = name
	}
	if input.URL != nil {
		endpointURL := strings.TrimSpace(*input.URL)
		if err := s.validateURL(endpointURL); err != nil {
			return nil, err
		}
		endpoint.URL = endpointURL
	}
	if input.Secret != nil {
		if strings.TrimSpace(*input.Secret) == "" {
			return nil, domain.ErrInvalidInput
		}
		endpoint.SecretHash = hashWebhookSecret(*input.Secret)
	}
	if input.EventTypes != nil {
		eventTypes, normalizeErr := normalizeWebhookEventTypes(*input.EventTypes)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		endpoint.EventTypes = eventTypes
	}
	if input.IsActive != nil {
		endpoint.IsActive = *input.IsActive
	}
	endpoint.UpdatedAt = time.Now().UTC()
	if err := s.repository.UpdateWebhookEndpoint(ctx, endpoint); err != nil {
		return nil, err
	}
	return endpoint, nil
}

func (s *WebhookService) DeleteEndpoint(ctx context.Context, orgID, id uuid.UUID) error {
	return s.repository.SoftDeleteWebhookEndpoint(ctx, orgID, id, time.Now().UTC())
}

func (s *WebhookService) validateURL(raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" {
		return domain.ErrInvalidInput
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		if s.allowInsecureLocal && isLocalWebhookHost(parsed.Hostname()) {
			return nil
		}
		return domain.ErrInvalidInput
	default:
		return domain.ErrInvalidInput
	}
}

func isLocalWebhookHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeWebhookEventTypes(values []domain.WebhookEventType) ([]domain.WebhookEventType, error) {
	if len(values) == 0 {
		return nil, domain.ErrInvalidInput
	}
	seen := make(map[domain.WebhookEventType]struct{}, len(values))
	result := make([]domain.WebhookEventType, 0, len(values))
	for _, eventType := range values {
		if !eventType.Valid() {
			return nil, domain.ErrInvalidInput
		}
		if _, exists := seen[eventType]; exists {
			continue
		}
		seen[eventType] = struct{}{}
		result = append(result, eventType)
	}
	return result, nil
}

func hashWebhookSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
