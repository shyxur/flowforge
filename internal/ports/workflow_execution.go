package ports

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
)

type WorkflowExecutionRepository interface {
	GetWorkflowExecutionVersion(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		uuid.UUID,
	) (*domain.WorkflowVersion, error)
	CreateWorkflowExecution(
		context.Context,
		*domain.WorkflowExecution,
		[]*domain.WorkflowNodeExecution,
	) (*domain.WorkflowExecution, bool, error)
	GetWorkflowExecution(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.WorkflowExecution, error)
	ListWorkflowExecutions(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		domain.WorkflowExecutionFilter,
	) (*domain.WorkflowExecutionPage, error)
	GetWorkflowNodeExecutions(context.Context, uuid.UUID, uuid.UUID) ([]*domain.WorkflowNodeExecution, error)
	ClaimWorkflowNode(context.Context, uuid.UUID, uuid.UUID, string, time.Time) (*domain.WorkflowNodeExecution, bool, error)
	UpdateWorkflowNodeExecution(
		context.Context,
		*domain.WorkflowNodeExecution,
		domain.WorkflowNodeExecutionStatus,
	) (bool, error)
	UpdateWorkflowExecution(
		context.Context,
		*domain.WorkflowExecution,
		[]domain.WorkflowExecutionStatus,
	) (bool, error)
	FinalizeWorkflowExecution(
		context.Context,
		*domain.WorkflowExecution,
		[]domain.WorkflowExecutionStatus,
		bool,
	) (bool, error)
	CancelWorkflowExecution(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) (*domain.WorkflowExecution, error)
	ListWorkflowExecutionsForReconciliation(context.Context, int) ([]*domain.WorkflowExecution, error)
}

type WorkflowTaskDispatcher interface {
	DispatchWorkflowTask(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		string,
		map[string]any,
		json.RawMessage,
	) (*domain.Task, error)
	GetWorkflowTask(context.Context, uuid.UUID, uuid.UUID) (*domain.Task, error)
}

type WorkflowWebhookDispatcher interface {
	DispatchWorkflowWebhook(
		context.Context,
		*domain.WorkflowExecution,
		domain.WorkflowNode,
		json.RawMessage,
	) (*domain.WebhookDelivery, error)
	GetWorkflowWebhook(context.Context, uuid.UUID, uuid.UUID) (*domain.WebhookDelivery, error)
}
