package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/ports"
)

const (
	workflowNameMax        = 255
	workflowSlugMax        = 120
	workflowDescriptionMax = 2000
	workflowElementIDMax   = 255
)

var workflowSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type WorkflowService struct {
	repository ports.WorkflowRepository
}

func NewWorkflowService(repository ports.WorkflowRepository) *WorkflowService {
	return &WorkflowService{repository: repository}
}

type CreateWorkflowInput struct {
	OrgID       uuid.UUID
	Name        string
	Slug        string
	Description *string
	Definition  json.RawMessage
}

type UpdateWorkflowInput struct {
	Name           *string
	Slug           *string
	DescriptionSet bool
	Description    *string
	Definition     json.RawMessage
}

func (s *WorkflowService) CreateWorkflow(ctx context.Context, input CreateWorkflowInput) (*domain.Workflow, error) {
	name, err := normalizeWorkflowName(input.Name)
	if err != nil {
		return nil, err
	}
	slug := strings.TrimSpace(input.Slug)
	if slug == "" {
		slug = generateWorkflowSlug(name)
	}
	if err := validateWorkflowSlug(slug); err != nil {
		return nil, err
	}
	description, err := normalizeWorkflowDescription(input.Description)
	if err != nil {
		return nil, err
	}
	definition, err := parseWorkflowDefinition(input.Definition)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	workflow := &domain.Workflow{
		ID: uuid.New(), OrgID: input.OrgID, Name: name, Slug: slug,
		Description: description, Status: domain.WorkflowStatusDraft,
		Definition: definition, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repository.CreateWorkflow(ctx, workflow); err != nil {
		return nil, err
	}
	return workflow, nil
}

func (s *WorkflowService) ListWorkflows(ctx context.Context, orgID uuid.UUID, filter domain.WorkflowFilter) (*domain.WorkflowPage, error) {
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, domain.ErrInvalidInput
	}
	return s.repository.ListWorkflows(ctx, orgID, filter)
}

func (s *WorkflowService) GetWorkflow(ctx context.Context, orgID, id uuid.UUID) (*domain.Workflow, error) {
	return s.repository.GetWorkflowByID(ctx, orgID, id)
}

func (s *WorkflowService) UpdateWorkflow(ctx context.Context, orgID, id uuid.UUID, input UpdateWorkflowInput) (*domain.Workflow, error) {
	if input.Name == nil && input.Slug == nil && !input.DescriptionSet && len(input.Definition) == 0 {
		return nil, domain.ErrInvalidInput
	}
	workflow, err := s.repository.GetWorkflowByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if workflow.Status != domain.WorkflowStatusDraft {
		return nil, domain.ErrInvalidStateTransition
	}
	if input.Name != nil {
		workflow.Name, err = normalizeWorkflowName(*input.Name)
		if err != nil {
			return nil, err
		}
	}
	if input.Slug != nil {
		slug := strings.TrimSpace(*input.Slug)
		if err := validateWorkflowSlug(slug); err != nil {
			return nil, err
		}
		workflow.Slug = slug
	}
	if input.DescriptionSet {
		workflow.Description, err = normalizeWorkflowDescription(input.Description)
		if err != nil {
			return nil, err
		}
	}
	if len(input.Definition) > 0 {
		workflow.Definition, err = parseWorkflowDefinition(input.Definition)
		if err != nil {
			return nil, err
		}
	}
	workflow.UpdatedAt = time.Now().UTC()
	if err := s.repository.UpdateWorkflow(ctx, workflow); err != nil {
		return nil, err
	}
	return workflow, nil
}

func (s *WorkflowService) DeleteWorkflow(ctx context.Context, orgID, id uuid.UUID) error {
	return s.repository.SoftDeleteWorkflow(ctx, orgID, id, time.Now().UTC())
}

func normalizeWorkflowName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > workflowNameMax {
		return "", domain.ErrInvalidInput
	}
	return value, nil
}

func normalizeWorkflowDescription(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized := strings.TrimSpace(*value)
	if len(normalized) > workflowDescriptionMax {
		return nil, domain.ErrInvalidInput
	}
	return &normalized, nil
}

func validateWorkflowSlug(value string) error {
	if value == "" || len(value) > workflowSlugMax || !workflowSlugPattern.MatchString(value) {
		return domain.ErrInvalidInput
	}
	return nil
}

func generateWorkflowSlug(name string) string {
	var builder strings.Builder
	separator := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if r <= unicode.MaxASCII {
				if separator && builder.Len() > 0 {
					builder.WriteByte('-')
				}
				builder.WriteRune(r)
				separator = false
			}
		} else {
			separator = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func parseWorkflowDefinition(raw json.RawMessage) (domain.WorkflowDefinition, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return domain.WorkflowDefinition{}, domain.ErrInvalidInput
	}
	var definition domain.WorkflowDefinition
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return domain.WorkflowDefinition{}, domain.ErrInvalidInput
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.WorkflowDefinition{}, domain.ErrInvalidInput
	}
	if definition.Nodes == nil || definition.Edges == nil {
		return domain.WorkflowDefinition{}, domain.ErrInvalidInput
	}
	nodes := make(map[string]struct{}, len(definition.Nodes))
	for i := range definition.Nodes {
		node := &definition.Nodes[i]
		node.ID = strings.TrimSpace(node.ID)
		node.Name = strings.TrimSpace(node.Name)
		if node.ID == "" || len(node.ID) > workflowElementIDMax || node.Name == "" ||
			len(node.Name) > workflowNameMax || !node.Type.Valid() || node.Config == nil {
			return domain.WorkflowDefinition{}, domain.ErrInvalidInput
		}
		if _, exists := nodes[node.ID]; exists {
			return domain.WorkflowDefinition{}, domain.ErrInvalidInput
		}
		nodes[node.ID] = struct{}{}
	}
	edges := make(map[string]struct{}, len(definition.Edges))
	for i := range definition.Edges {
		edge := &definition.Edges[i]
		edge.ID = strings.TrimSpace(edge.ID)
		edge.From = strings.TrimSpace(edge.From)
		edge.To = strings.TrimSpace(edge.To)
		if edge.ID == "" || len(edge.ID) > workflowElementIDMax || edge.From == "" || edge.To == "" {
			return domain.WorkflowDefinition{}, domain.ErrInvalidInput
		}
		if _, exists := edges[edge.ID]; exists {
			return domain.WorkflowDefinition{}, domain.ErrInvalidInput
		}
		if _, exists := nodes[edge.From]; !exists {
			return domain.WorkflowDefinition{}, domain.ErrInvalidInput
		}
		if _, exists := nodes[edge.To]; !exists {
			return domain.WorkflowDefinition{}, domain.ErrInvalidInput
		}
		if len(edge.Condition) == 0 {
			edge.Condition = json.RawMessage("null")
		}
		edges[edge.ID] = struct{}{}
	}
	return definition, nil
}
