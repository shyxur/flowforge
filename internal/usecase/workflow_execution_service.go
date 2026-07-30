package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	metricspkg "github.com/shyxur/windylane/internal/metrics"
	"github.com/shyxur/windylane/internal/ports"
)

type WorkflowExecutionService struct {
	executions ports.WorkflowExecutionRepository
	workflows  ports.WorkflowRepository
	tasks      ports.WorkflowTaskDispatcher
	webhooks   ports.WorkflowWebhookDispatcher
	now        func() time.Time
	metrics    ports.MetricRecorder
}

func (service *WorkflowExecutionService) WithMetricRecorder(
	recorder ports.MetricRecorder,
) *WorkflowExecutionService {
	service.metrics = recorder
	return service
}

const (
	maxWorkflowExecutionInputBytes    = 256 * 1024
	workflowNodeDispatchRecoveryAfter = 30 * time.Second
)

func NewWorkflowExecutionService(
	executions ports.WorkflowExecutionRepository,
	workflows ports.WorkflowRepository,
	tasks ports.WorkflowTaskDispatcher,
	webhooks ports.WorkflowWebhookDispatcher,
) *WorkflowExecutionService {
	return &WorkflowExecutionService{
		executions: executions,
		workflows:  workflows,
		tasks:      tasks,
		webhooks:   webhooks,
		now:        func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	}
}

type StartWorkflowExecutionInput struct {
	OrgID          uuid.UUID
	WorkflowID     uuid.UUID
	Version        *int
	Input          json.RawMessage
	IdempotencyKey string
}

func (service *WorkflowExecutionService) StartExecution(
	ctx context.Context,
	input StartWorkflowExecutionInput,
) (*domain.WorkflowExecutionStartResult, bool, error) {
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		return nil, false, domain.ErrInvalidInput
	}
	workflow, err := service.workflows.GetWorkflowByID(ctx, input.OrgID, input.WorkflowID)
	if err != nil {
		return nil, false, err
	}
	if workflow.Status != domain.WorkflowStatusPublished {
		return nil, false, domain.ErrWorkflowNotPublished
	}
	version, err := service.selectVersion(ctx, input.OrgID, input.WorkflowID, input.Version)
	if err != nil {
		return nil, false, err
	}
	normalizedInput, err := normalizeWorkflowExecutionInput(input.Input)
	if err != nil {
		return nil, false, domain.ErrInvalidInput
	}
	if len(normalizedInput) > maxWorkflowExecutionInputBytes {
		return nil, false, domain.ErrInvalidInput
	}
	fingerprint, err := workflowExecutionFingerprint(
		input.WorkflowID, version.Version, normalizedInput,
	)
	if err != nil {
		return nil, false, err
	}
	now := service.now()
	execution := &domain.WorkflowExecution{
		ID:                 uuid.New(),
		OrgID:              input.OrgID,
		WorkflowID:         input.WorkflowID,
		WorkflowVersionID:  version.ID,
		WorkflowVersion:    version.Version,
		Status:             domain.WorkflowExecutionPending,
		Input:              normalizedInput,
		IdempotencyKey:     idempotencyKey,
		RequestFingerprint: fingerprint,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	nodes := make([]*domain.WorkflowNodeExecution, 0, len(version.Definition.Nodes))
	for _, definitionNode := range version.Definition.Nodes {
		nodes = append(nodes, &domain.WorkflowNodeExecution{
			ID:                  uuid.New(),
			OrgID:               input.OrgID,
			WorkflowExecutionID: execution.ID,
			NodeID:              definitionNode.ID,
			NodeType:            definitionNode.Type,
			Status:              domain.WorkflowNodePending,
			CreatedAt:           now,
			UpdatedAt:           now,
		})
	}
	execution, reused, err := service.executions.CreateWorkflowExecution(ctx, execution, nodes)
	if err != nil {
		return nil, false, err
	}
	if execution.WorkflowID != input.WorkflowID {
		return nil, false, domain.ErrWorkflowExecutionIdempotencyConflict
	}
	if !reused {
		service.recordExecutionMetric(execution, domain.MetricWorkflowExecutionCreated, "")
	}
	if err := service.AdvanceExecution(ctx, execution.OrgID, execution.WorkflowID, execution.ID); err != nil {
		return nil, reused, err
	}
	current, err := service.executions.GetWorkflowExecution(
		ctx, execution.OrgID, execution.WorkflowID, execution.ID,
	)
	if err != nil {
		return nil, reused, err
	}
	return &domain.WorkflowExecutionStartResult{
		ExecutionID:     current.ID,
		WorkflowID:      current.WorkflowID,
		WorkflowVersion: current.WorkflowVersion,
		Status:          current.Status,
		CreatedAt:       current.CreatedAt,
	}, reused, nil
}

func (service *WorkflowExecutionService) selectVersion(
	ctx context.Context,
	orgID, workflowID uuid.UUID,
	requested *int,
) (*domain.WorkflowVersion, error) {
	if requested != nil {
		if *requested <= 0 {
			return nil, domain.ErrInvalidInput
		}
		version, err := service.workflows.GetWorkflowVersion(ctx, orgID, workflowID, *requested)
		if err != nil {
			return nil, err
		}
		if version.Status != domain.WorkflowVersionStatusPublished {
			return nil, domain.ErrWorkflowVersionNotFound
		}
		return version, nil
	}
	version, err := service.workflows.GetLatestWorkflowVersion(ctx, orgID, workflowID)
	if err != nil {
		return nil, err
	}
	if version.Status != domain.WorkflowVersionStatusPublished {
		return nil, domain.ErrWorkflowVersionNotFound
	}
	return version, nil
}

func (service *WorkflowExecutionService) ListExecutions(
	ctx context.Context,
	orgID, workflowID uuid.UUID,
	filter domain.WorkflowExecutionFilter,
) (*domain.WorkflowExecutionPage, error) {
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, domain.ErrInvalidInput
	}
	if _, err := service.workflows.GetWorkflowByID(ctx, orgID, workflowID); err != nil {
		return nil, err
	}
	return service.executions.ListWorkflowExecutions(ctx, orgID, workflowID, filter)
}

func (service *WorkflowExecutionService) GetExecution(
	ctx context.Context,
	orgID, workflowID, executionID uuid.UUID,
) (*domain.WorkflowExecutionDetail, error) {
	execution, err := service.executions.GetWorkflowExecution(
		ctx, orgID, workflowID, executionID,
	)
	if err != nil {
		return nil, err
	}
	nodes, err := service.executions.GetWorkflowNodeExecutions(ctx, orgID, executionID)
	if err != nil {
		return nil, err
	}
	sort.Slice(nodes, func(left, right int) bool {
		if nodes[left].CreatedAt.Equal(nodes[right].CreatedAt) {
			return nodes[left].NodeID < nodes[right].NodeID
		}
		return nodes[left].CreatedAt.Before(nodes[right].CreatedAt)
	})
	return &domain.WorkflowExecutionDetail{
		WorkflowExecution: execution,
		Nodes:             nodes,
	}, nil
}

func (service *WorkflowExecutionService) CancelExecution(
	ctx context.Context,
	orgID, workflowID, executionID uuid.UUID,
) (*domain.WorkflowExecution, error) {
	execution, err := service.executions.CancelWorkflowExecution(
		ctx, orgID, workflowID, executionID, service.now(),
	)
	if err != nil {
		return nil, err
	}
	service.recordExecutionMetric(execution, domain.MetricWorkflowExecutionCancelled, "")
	nodes, _ := service.executions.GetWorkflowNodeExecutions(ctx, orgID, executionID)
	for _, node := range nodes {
		if node.Status == domain.WorkflowNodeCancelled {
			service.recordNodeMetric(node, domain.MetricNodeExecutionCancelled, "")
		}
	}
	return execution, nil
}

func (service *WorkflowExecutionService) AdvanceExecution(
	ctx context.Context,
	orgID, workflowID, executionID uuid.UUID,
) error {
	execution, err := service.executions.GetWorkflowExecution(
		ctx, orgID, workflowID, executionID,
	)
	if err != nil {
		return err
	}
	if execution.Status.Terminal() {
		return nil
	}
	version, err := service.executions.GetWorkflowExecutionVersion(
		ctx, orgID, workflowID, execution.WorkflowVersionID,
	)
	if err != nil {
		return err
	}
	if version.Version != execution.WorkflowVersion {
		return domain.ErrWorkflowVersionNotFound
	}
	if execution.Status == domain.WorkflowExecutionPending {
		now := service.now()
		execution.Status = domain.WorkflowExecutionRunning
		execution.StartedAt = &now
		execution.UpdatedAt = now
		updated, err := service.executions.UpdateWorkflowExecution(
			ctx, execution, []domain.WorkflowExecutionStatus{domain.WorkflowExecutionPending},
		)
		if err != nil {
			return err
		}
		if updated {
			service.recordExecutionMetric(execution, domain.MetricWorkflowExecutionStarted, "")
		}
	}

	for iteration := 0; iteration <= len(version.Definition.Nodes)+1; iteration++ {
		nodes, err := service.executions.GetWorkflowNodeExecutions(ctx, orgID, executionID)
		if err != nil {
			return err
		}
		changed, err := service.reconcileExternalNodes(
			ctx, execution, version.Definition, nodes,
		)
		if err != nil {
			return err
		}
		if changed {
			continue
		}
		nodes, err = service.executions.GetWorkflowNodeExecutions(ctx, orgID, executionID)
		if err != nil {
			return err
		}
		if failed := firstFailedWorkflowNode(nodes); failed != nil {
			return service.failExecution(ctx, execution, failed.ErrorCode, failed.ErrorMessage)
		}
		progressed, err := service.scheduleRunnableNodes(
			ctx, execution, version.Definition, nodes,
		)
		if err != nil {
			return err
		}
		if progressed {
			continue
		}
		return service.finalizeExecutionIfComplete(ctx, execution, version.Definition, nodes)
	}
	return nil
}

func (service *WorkflowExecutionService) reconcileExternalNodes(
	ctx context.Context,
	execution *domain.WorkflowExecution,
	definition domain.WorkflowDefinition,
	nodes []*domain.WorkflowNodeExecution,
) (bool, error) {
	changed := false
	definitionNodes := make(map[string]domain.WorkflowNode, len(definition.Nodes))
	for _, definitionNode := range definition.Nodes {
		definitionNodes[definitionNode.ID] = definitionNode
	}
	for _, node := range nodes {
		if node.Status != domain.WorkflowNodeQueued && node.Status != domain.WorkflowNodeRunning {
			continue
		}
		expected := node.Status
		previousAttempt := node.Attempt
		now := service.now()
		switch {
		case node.QueueTaskID != nil:
			task, err := service.tasks.GetWorkflowTask(ctx, execution.OrgID, *node.QueueTaskID)
			if err != nil {
				if errors.Is(err, domain.ErrTaskNotFound) {
					continue
				}
				return changed, err
			}
			node.Attempt = task.Attempts
			switch task.Status {
			case domain.StatusPending:
				node.Status = domain.WorkflowNodeQueued
			case domain.StatusProcessing:
				node.Status = domain.WorkflowNodeRunning
			case domain.StatusCompleted:
				node.Status = domain.WorkflowNodeSucceeded
				node.Output = workflowNodeOutput(map[string]any{"task_id": task.ID})
				node.CompletedAt = &now
			case domain.StatusFailed, domain.StatusDeadLetter, domain.StatusCancelled:
				node.Status = domain.WorkflowNodeFailed
				node.ErrorCode = "node_execution_failed"
				node.ErrorMessage = task.LastError
				if node.ErrorMessage == "" {
					node.ErrorMessage = "QueueFlow task did not complete successfully"
				}
				node.CompletedAt = &now
			}
		case node.WebhookDeliveryID != nil:
			delivery, err := service.webhooks.GetWorkflowWebhook(
				ctx, execution.OrgID, *node.WebhookDeliveryID,
			)
			if err != nil {
				if errors.Is(err, domain.ErrWebhookDeliveryNotFound) {
					continue
				}
				return changed, err
			}
			node.Attempt = delivery.AttemptCount
			switch delivery.Status {
			case domain.WebhookDeliveryPending, domain.WebhookDeliveryRetrying:
				node.Status = domain.WorkflowNodeQueued
			case domain.WebhookDeliveryDelivering:
				node.Status = domain.WorkflowNodeRunning
			case domain.WebhookDeliveryDelivered:
				node.Status = domain.WorkflowNodeSucceeded
				node.Output = workflowNodeOutput(map[string]any{"webhook_delivery_id": delivery.ID})
				node.CompletedAt = &now
			case domain.WebhookDeliveryFailed:
				node.Status = domain.WorkflowNodeFailed
				node.ErrorCode = "node_execution_failed"
				if delivery.LastError != nil {
					node.ErrorMessage = *delivery.LastError
				} else {
					node.ErrorMessage = "EventForge delivery failed"
				}
				node.CompletedAt = &now
			}
		case node.NodeType == domain.WorkflowNodeDelay && node.AvailableAt != nil:
			if !node.AvailableAt.After(now) {
				node.Status = domain.WorkflowNodeSucceeded
				node.Output = workflowNodeOutput(map[string]any{"resumed_at": now})
				node.CompletedAt = &now
			}
		case node.Status == domain.WorkflowNodeRunning:
			if now.Sub(node.UpdatedAt) < workflowNodeDispatchRecoveryAfter {
				continue
			}
			definitionNode, exists := definitionNodes[node.NodeID]
			if !exists {
				return changed, domain.ErrInvalidInput
			}
			if err := service.executeClaimedNode(
				ctx, execution, definitionNode, node,
			); err != nil {
				return true, err
			}
			changed = true
			continue
		}
		if node.Status != expected || node.Attempt != previousAttempt || node.CompletedAt != nil {
			node.UpdatedAt = now
			updated, err := service.updateNode(ctx, node, expected)
			if err != nil {
				return changed, err
			}
			changed = changed || updated
		}
	}
	return changed, nil
}

func (service *WorkflowExecutionService) scheduleRunnableNodes(
	ctx context.Context,
	execution *domain.WorkflowExecution,
	definition domain.WorkflowDefinition,
	nodes []*domain.WorkflowNodeExecution,
) (bool, error) {
	nodeStates := make(map[string]*domain.WorkflowNodeExecution, len(nodes))
	definitionNodes := make(map[string]domain.WorkflowNode, len(definition.Nodes))
	incoming := make(map[string][]domain.WorkflowEdge, len(definition.Nodes))
	for _, node := range nodes {
		nodeStates[node.NodeID] = node
	}
	for _, node := range definition.Nodes {
		definitionNodes[node.ID] = node
	}
	for _, edge := range definition.Edges {
		incoming[edge.To] = append(incoming[edge.To], edge)
	}

	for _, definitionNode := range definition.Nodes {
		state := nodeStates[definitionNode.ID]
		if state == nil || state.Status != domain.WorkflowNodePending {
			continue
		}
		runnable, shouldSkip, err := workflowNodeReadiness(
			definitionNode, incoming[definitionNode.ID], definitionNodes, nodeStates,
		)
		if err != nil {
			return false, err
		}
		if !runnable && !shouldSkip {
			continue
		}
		claimed, won, err := service.executions.ClaimWorkflowNode(
			ctx, execution.OrgID, execution.ID, definitionNode.ID, service.now(),
		)
		if err != nil {
			return false, err
		}
		if !won {
			continue
		}
		service.recordNodeMetric(claimed, domain.MetricNodeExecutionStarted, "")
		if shouldSkip {
			now := service.now()
			claimed.Status = domain.WorkflowNodeSkipped
			claimed.CompletedAt = &now
			claimed.UpdatedAt = now
			_, err = service.updateNode(ctx, claimed, domain.WorkflowNodeRunning)
			return true, err
		}
		if err := service.executeClaimedNode(ctx, execution, definitionNode, claimed); err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

func workflowNodeReadiness(
	node domain.WorkflowNode,
	incoming []domain.WorkflowEdge,
	definitionNodes map[string]domain.WorkflowNode,
	states map[string]*domain.WorkflowNodeExecution,
) (bool, bool, error) {
	if node.Type == domain.WorkflowNodeTrigger {
		return true, false, nil
	}
	if len(incoming) == 0 {
		return false, false, nil
	}
	active := 0
	for _, edge := range incoming {
		sourceState := states[edge.From]
		if sourceState == nil || !sourceState.Status.Terminal() {
			return false, false, nil
		}
		if sourceState.Status == domain.WorkflowNodeFailed ||
			sourceState.Status == domain.WorkflowNodeCancelled {
			return false, false, nil
		}
		if sourceState.Status == domain.WorkflowNodeSkipped {
			continue
		}
		source := definitionNodes[edge.From]
		if source.Type == domain.WorkflowNodeCondition {
			branch, err := parseWorkflowBranch(edge.Condition)
			if err != nil {
				return false, false, err
			}
			if branch != nil {
				result, err := workflowConditionResult(sourceState.Output)
				if err != nil {
					return false, false, err
				}
				if result != *branch {
					continue
				}
			}
		}
		active++
	}
	if active == 0 {
		return false, true, nil
	}
	return true, false, nil
}

func (service *WorkflowExecutionService) executeClaimedNode(
	ctx context.Context,
	execution *domain.WorkflowExecution,
	definitionNode domain.WorkflowNode,
	node *domain.WorkflowNodeExecution,
) error {
	node.Input = execution.Input
	now := service.now()
	switch definitionNode.Type {
	case domain.WorkflowNodeTrigger:
		node.Status = domain.WorkflowNodeSucceeded
		node.Output = execution.Input
		node.CompletedAt = &now
	case domain.WorkflowNodeCondition:
		config, err := parseWorkflowConditionConfig(definitionNode.Config)
		if err != nil {
			return service.failClaimedNode(ctx, execution, node, "invalid_node_config", err.Error())
		}
		result, err := evaluateWorkflowCondition(config, execution.Input)
		if err != nil {
			return service.failClaimedNode(ctx, execution, node, "node_execution_failed", err.Error())
		}
		node.Status = domain.WorkflowNodeSucceeded
		node.Output = json.RawMessage(fmt.Sprintf(`{"result":%t}`, result))
		node.CompletedAt = &now
	case domain.WorkflowNodeDelay:
		config, err := parseWorkflowDelayConfig(definitionNode.Config)
		if err != nil {
			return service.failClaimedNode(ctx, execution, node, "invalid_node_config", err.Error())
		}
		availableAt := now.Add(time.Duration(config.DurationSeconds) * time.Second)
		node.Status = domain.WorkflowNodeQueued
		node.AvailableAt = &availableAt
	case domain.WorkflowNodeTask:
		task, err := service.tasks.DispatchWorkflowTask(
			ctx, execution.OrgID, execution.ID, definitionNode.ID,
			definitionNode.Config, execution.Input,
		)
		if task == nil {
			message := "QueueFlow task dispatch failed"
			if err != nil {
				message = err.Error()
			}
			return service.failClaimedNode(ctx, execution, node, "node_dispatch_failed", message)
		}
		node.Status = domain.WorkflowNodeQueued
		node.QueueTaskID = &task.ID
	case domain.WorkflowNodeWebhook:
		delivery, err := service.webhooks.DispatchWorkflowWebhook(
			ctx, execution, definitionNode, execution.Input,
		)
		if delivery == nil {
			message := "EventForge delivery dispatch failed"
			if err != nil {
				message = err.Error()
			}
			return service.failClaimedNode(ctx, execution, node, "node_dispatch_failed", message)
		}
		node.Status = domain.WorkflowNodeQueued
		node.WebhookDeliveryID = &delivery.ID
	default:
		return service.failClaimedNode(
			ctx, execution, node, "invalid_node_config", "unsupported node type",
		)
	}
	node.UpdatedAt = now
	_, err := service.updateNode(ctx, node, domain.WorkflowNodeRunning)
	return err
}

func (service *WorkflowExecutionService) failClaimedNode(
	ctx context.Context,
	execution *domain.WorkflowExecution,
	node *domain.WorkflowNodeExecution,
	code, message string,
) error {
	now := service.now()
	node.Status = domain.WorkflowNodeFailed
	node.ErrorCode = code
	node.ErrorMessage = message
	node.CompletedAt = &now
	node.UpdatedAt = now
	if _, err := service.updateNode(ctx, node, domain.WorkflowNodeRunning); err != nil {
		return err
	}
	return service.failExecution(ctx, execution, code, message)
}

func (service *WorkflowExecutionService) failExecution(
	ctx context.Context,
	execution *domain.WorkflowExecution,
	code, message string,
) error {
	now := service.now()
	execution.Status = domain.WorkflowExecutionFailed
	execution.ErrorCode = code
	execution.ErrorMessage = message
	execution.CompletedAt = &now
	execution.UpdatedAt = now
	updated, err := service.executions.FinalizeWorkflowExecution(
		ctx, execution,
		[]domain.WorkflowExecutionStatus{
			domain.WorkflowExecutionPending,
			domain.WorkflowExecutionRunning,
		},
		true,
	)
	if updated {
		service.recordExecutionMetric(execution, domain.MetricWorkflowExecutionFailed, code)
		nodes, listErr := service.executions.GetWorkflowNodeExecutions(
			ctx, execution.OrgID, execution.ID,
		)
		if listErr == nil {
			for _, node := range nodes {
				if node.Status == domain.WorkflowNodeCancelled {
					service.recordNodeMetric(node, domain.MetricNodeExecutionCancelled, "")
				}
			}
		}
	}
	return err
}

func (service *WorkflowExecutionService) finalizeExecutionIfComplete(
	ctx context.Context,
	execution *domain.WorkflowExecution,
	definition domain.WorkflowDefinition,
	nodes []*domain.WorkflowNodeExecution,
) error {
	if len(nodes) != len(definition.Nodes) {
		return service.touchRunningExecution(ctx, execution)
	}
	outputs := make(map[string]json.RawMessage)
	for _, node := range nodes {
		if node.Status != domain.WorkflowNodeSucceeded &&
			node.Status != domain.WorkflowNodeSkipped {
			return service.touchRunningExecution(ctx, execution)
		}
		if len(node.Output) > 0 {
			outputs[node.NodeID] = node.Output
		}
	}
	output, err := json.Marshal(map[string]any{"nodes": outputs})
	if err != nil {
		return err
	}
	now := service.now()
	execution.Status = domain.WorkflowExecutionSucceeded
	execution.Output = output
	execution.CompletedAt = &now
	execution.UpdatedAt = now
	updated, err := service.executions.FinalizeWorkflowExecution(
		ctx, execution,
		[]domain.WorkflowExecutionStatus{
			domain.WorkflowExecutionPending,
			domain.WorkflowExecutionRunning,
		},
		false,
	)
	if updated {
		service.recordExecutionMetric(execution, domain.MetricWorkflowExecutionSucceeded, "")
	}
	return err
}

func (service *WorkflowExecutionService) updateNode(
	ctx context.Context,
	node *domain.WorkflowNodeExecution,
	expected domain.WorkflowNodeExecutionStatus,
) (bool, error) {
	updated, err := service.executions.UpdateWorkflowNodeExecution(ctx, node, expected)
	if err != nil || !updated {
		return updated, err
	}
	switch node.Status {
	case domain.WorkflowNodeSucceeded:
		service.recordNodeMetric(node, domain.MetricNodeExecutionSucceeded, "")
	case domain.WorkflowNodeFailed:
		service.recordNodeMetric(node, domain.MetricNodeExecutionFailed, node.ErrorCode)
	case domain.WorkflowNodeSkipped:
		service.recordNodeMetric(node, domain.MetricNodeExecutionSkipped, "")
	case domain.WorkflowNodeCancelled:
		service.recordNodeMetric(node, domain.MetricNodeExecutionCancelled, "")
	}
	return true, nil
}

func (service *WorkflowExecutionService) recordExecutionMetric(
	execution *domain.WorkflowExecution,
	eventType domain.MetricEventType,
	errorCode string,
) {
	if execution == nil {
		return
	}
	metricspkg.Record(service.metrics, domain.NewMetricEventInput{
		OrganizationID: execution.OrgID, Source: domain.MetricSourceTaskCanvas,
		EventType: eventType, ResourceType: domain.MetricResourceWorkflowExecution,
		ResourceID: execution.ID.String(), Status: string(execution.Status),
		OccurredAt: execution.UpdatedAt, Metadata: domain.MetricMetadata{ErrorCode: errorCode},
		TransitionKey: string(eventType),
	})
}

func (service *WorkflowExecutionService) recordNodeMetric(
	node *domain.WorkflowNodeExecution,
	eventType domain.MetricEventType,
	errorCode string,
) {
	if node == nil {
		return
	}
	attempt := node.Attempt
	metricspkg.Record(service.metrics, domain.NewMetricEventInput{
		OrganizationID: node.OrgID, Source: domain.MetricSourceTaskCanvas,
		EventType: eventType, ResourceType: domain.MetricResourceNodeExecution,
		ResourceID: node.ID.String(), Status: string(node.Status), OccurredAt: node.UpdatedAt,
		Metadata:      domain.MetricMetadata{Attempt: &attempt, ErrorCode: errorCode},
		TransitionKey: string(eventType),
	})
}

func (service *WorkflowExecutionService) touchRunningExecution(
	ctx context.Context,
	execution *domain.WorkflowExecution,
) error {
	execution.Status = domain.WorkflowExecutionRunning
	execution.UpdatedAt = service.now()
	_, err := service.executions.UpdateWorkflowExecution(
		ctx, execution,
		[]domain.WorkflowExecutionStatus{domain.WorkflowExecutionRunning},
	)
	return err
}

func (service *WorkflowExecutionService) ReconcileActive(
	ctx context.Context,
	limit int,
) (int, error) {
	executions, err := service.executions.ListWorkflowExecutionsForReconciliation(ctx, limit)
	if err != nil {
		return 0, err
	}
	var reconcileErrors []error
	for _, execution := range executions {
		if err := service.AdvanceExecution(
			ctx, execution.OrgID, execution.WorkflowID, execution.ID,
		); err != nil {
			reconcileErrors = append(reconcileErrors, err)
		}
	}
	return len(executions), errors.Join(reconcileErrors...)
}

func (service *WorkflowExecutionService) ReconciliationLoop(
	ctx context.Context,
	interval time.Duration,
	limit int,
) {
	if interval <= 0 {
		interval = time.Second
	}
	process := func() {
		_, _ = service.ReconcileActive(ctx, limit)
	}
	process()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			process()
		}
	}
}

func normalizeWorkflowExecutionInput(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func workflowExecutionFingerprint(
	workflowID uuid.UUID,
	version int,
	input json.RawMessage,
) (string, error) {
	canonical, err := json.Marshal(struct {
		WorkflowID uuid.UUID       `json:"workflow_id"`
		Version    int             `json:"version"`
		Input      json.RawMessage `json:"input"`
	}{WorkflowID: workflowID, Version: version, Input: input})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func firstFailedWorkflowNode(nodes []*domain.WorkflowNodeExecution) *domain.WorkflowNodeExecution {
	for _, node := range nodes {
		if node.Status == domain.WorkflowNodeFailed {
			return node
		}
	}
	return nil
}

func workflowConditionResult(output json.RawMessage) (bool, error) {
	var value struct {
		Result bool `json:"result"`
	}
	if err := json.Unmarshal(output, &value); err != nil {
		return false, err
	}
	return value.Result, nil
}

func workflowNodeOutput(value map[string]any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}
