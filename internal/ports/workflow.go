package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
)

type WorkflowRepository interface {
	CreateWorkflow(context.Context, *domain.Workflow) error
	GetWorkflowByID(context.Context, uuid.UUID, uuid.UUID) (*domain.Workflow, error)
	GetWorkflowBySlug(context.Context, uuid.UUID, string) (*domain.Workflow, error)
	ListWorkflows(context.Context, uuid.UUID, domain.WorkflowFilter) (*domain.WorkflowPage, error)
	UpdateWorkflow(context.Context, *domain.Workflow) error
	SoftDeleteWorkflow(context.Context, uuid.UUID, uuid.UUID, time.Time) error
}
