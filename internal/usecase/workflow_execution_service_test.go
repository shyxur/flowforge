package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
)

type workflowExecutionMemory struct {
	mu         sync.Mutex
	workflows  map[uuid.UUID]*domain.Workflow
	versions   map[uuid.UUID][]*domain.WorkflowVersion
	executions map[uuid.UUID]*domain.WorkflowExecution
	nodes      map[uuid.UUID]map[string]*domain.WorkflowNodeExecution
}

func newWorkflowExecutionMemory(
	workflow *domain.Workflow,
	versions ...*domain.WorkflowVersion,
) *workflowExecutionMemory {
	return &workflowExecutionMemory{
		workflows:  map[uuid.UUID]*domain.Workflow{workflow.ID: cloneExecutionTestValue(workflow)},
		versions:   map[uuid.UUID][]*domain.WorkflowVersion{workflow.ID: versions},
		executions: make(map[uuid.UUID]*domain.WorkflowExecution),
		nodes:      make(map[uuid.UUID]map[string]*domain.WorkflowNodeExecution),
	}
}

func (memory *workflowExecutionMemory) CreateWorkflow(_ context.Context, workflow *domain.Workflow) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	memory.workflows[workflow.ID] = cloneExecutionTestValue(workflow)
	return nil
}
func (memory *workflowExecutionMemory) GetWorkflowByID(_ context.Context, orgID, id uuid.UUID) (*domain.Workflow, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	workflow := memory.workflows[id]
	if workflow == nil || workflow.OrgID != orgID || workflow.DeletedAt != nil {
		return nil, domain.ErrWorkflowNotFound
	}
	return cloneExecutionTestValue(workflow), nil
}
func (memory *workflowExecutionMemory) GetWorkflowBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*domain.Workflow, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, workflow := range memory.workflows {
		if workflow.OrgID == orgID && workflow.Slug == slug && workflow.DeletedAt == nil {
			return cloneExecutionTestValue(workflow), nil
		}
	}
	return nil, domain.ErrWorkflowNotFound
}
func (memory *workflowExecutionMemory) ListWorkflows(context.Context, uuid.UUID, domain.WorkflowFilter) (*domain.WorkflowPage, error) {
	return &domain.WorkflowPage{}, nil
}
func (memory *workflowExecutionMemory) UpdateWorkflow(_ context.Context, workflow *domain.Workflow) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	memory.workflows[workflow.ID] = cloneExecutionTestValue(workflow)
	return nil
}
func (memory *workflowExecutionMemory) SoftDeleteWorkflow(_ context.Context, orgID, id uuid.UUID, now time.Time) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	workflow := memory.workflows[id]
	if workflow == nil || workflow.OrgID != orgID {
		return domain.ErrWorkflowNotFound
	}
	workflow.DeletedAt = &now
	return nil
}
func (memory *workflowExecutionMemory) PublishWorkflow(context.Context, *domain.Workflow, time.Time) (*domain.WorkflowVersion, error) {
	return nil, errors.New("not implemented")
}
func (memory *workflowExecutionMemory) CreateWorkflowVersion(_ context.Context, version *domain.WorkflowVersion) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	memory.versions[version.WorkflowID] = append(memory.versions[version.WorkflowID], cloneExecutionTestValue(version))
	return nil
}
func (memory *workflowExecutionMemory) GetWorkflowVersion(_ context.Context, orgID, workflowID uuid.UUID, number int) (*domain.WorkflowVersion, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, version := range memory.versions[workflowID] {
		if version.OrgID == orgID && version.Version == number {
			return cloneExecutionTestValue(version), nil
		}
	}
	return nil, domain.ErrWorkflowVersionNotFound
}
func (memory *workflowExecutionMemory) ListWorkflowVersions(context.Context, uuid.UUID, uuid.UUID) (*domain.WorkflowVersionPage, error) {
	return &domain.WorkflowVersionPage{}, nil
}
func (memory *workflowExecutionMemory) GetLatestWorkflowVersion(_ context.Context, orgID, workflowID uuid.UUID) (*domain.WorkflowVersion, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	versions := memory.versions[workflowID]
	for index := len(versions) - 1; index >= 0; index-- {
		if versions[index].OrgID == orgID {
			return cloneExecutionTestValue(versions[index]), nil
		}
	}
	return nil, domain.ErrWorkflowVersionNotFound
}

func (memory *workflowExecutionMemory) CreateWorkflowExecution(
	_ context.Context,
	execution *domain.WorkflowExecution,
	nodes []*domain.WorkflowNodeExecution,
) (*domain.WorkflowExecution, bool, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, existing := range memory.executions {
		if existing.OrgID == execution.OrgID && existing.IdempotencyKey == execution.IdempotencyKey {
			if existing.RequestFingerprint != execution.RequestFingerprint {
				return nil, false, domain.ErrWorkflowExecutionIdempotencyConflict
			}
			return cloneWorkflowExecutionTest(existing), true, nil
		}
	}
	memory.executions[execution.ID] = cloneWorkflowExecutionTest(execution)
	memory.nodes[execution.ID] = make(map[string]*domain.WorkflowNodeExecution)
	for _, node := range nodes {
		memory.nodes[execution.ID][node.NodeID] = cloneExecutionTestValue(node)
	}
	return cloneWorkflowExecutionTest(execution), false, nil
}
func (memory *workflowExecutionMemory) GetWorkflowExecutionVersion(
	_ context.Context,
	orgID, workflowID, versionID uuid.UUID,
) (*domain.WorkflowVersion, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, version := range memory.versions[workflowID] {
		if version.OrgID == orgID && version.ID == versionID {
			return cloneExecutionTestValue(version), nil
		}
	}
	return nil, domain.ErrWorkflowVersionNotFound
}
func (memory *workflowExecutionMemory) GetWorkflowExecution(_ context.Context, orgID, workflowID, executionID uuid.UUID) (*domain.WorkflowExecution, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	execution := memory.executions[executionID]
	if execution == nil || execution.OrgID != orgID || execution.WorkflowID != workflowID {
		return nil, domain.ErrWorkflowExecutionNotFound
	}
	return cloneWorkflowExecutionTest(execution), nil
}
func (memory *workflowExecutionMemory) ListWorkflowExecutions(_ context.Context, orgID, workflowID uuid.UUID, filter domain.WorkflowExecutionFilter) (*domain.WorkflowExecutionPage, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	page := &domain.WorkflowExecutionPage{}
	for _, execution := range memory.executions {
		if execution.OrgID == orgID && execution.WorkflowID == workflowID &&
			(filter.Status == "" || execution.Status == filter.Status) {
			page.Executions = append(page.Executions, cloneWorkflowExecutionTest(execution))
		}
	}
	sort.Slice(page.Executions, func(left, right int) bool {
		return page.Executions[left].CreatedAt.After(page.Executions[right].CreatedAt)
	})
	return page, nil
}
func (memory *workflowExecutionMemory) GetWorkflowNodeExecutions(_ context.Context, orgID, executionID uuid.UUID) ([]*domain.WorkflowNodeExecution, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	execution := memory.executions[executionID]
	if execution == nil || execution.OrgID != orgID {
		return nil, domain.ErrWorkflowExecutionNotFound
	}
	result := make([]*domain.WorkflowNodeExecution, 0, len(memory.nodes[executionID]))
	for _, node := range memory.nodes[executionID] {
		result = append(result, cloneExecutionTestValue(node))
	}
	sort.Slice(result, func(left, right int) bool { return result[left].NodeID < result[right].NodeID })
	return result, nil
}
func (memory *workflowExecutionMemory) ClaimWorkflowNode(_ context.Context, orgID, executionID uuid.UUID, nodeID string, now time.Time) (*domain.WorkflowNodeExecution, bool, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	node := memory.nodes[executionID][nodeID]
	if node == nil || node.OrgID != orgID || node.Status != domain.WorkflowNodePending {
		return nil, false, nil
	}
	node.Status = domain.WorkflowNodeRunning
	node.Attempt++
	node.StartedAt = &now
	node.UpdatedAt = now
	return cloneExecutionTestValue(node), true, nil
}
func (memory *workflowExecutionMemory) UpdateWorkflowNodeExecution(_ context.Context, node *domain.WorkflowNodeExecution, expected domain.WorkflowNodeExecutionStatus) (bool, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	current := memory.nodes[node.WorkflowExecutionID][node.NodeID]
	if current == nil || current.Status != expected {
		return false, nil
	}
	memory.nodes[node.WorkflowExecutionID][node.NodeID] = cloneExecutionTestValue(node)
	return true, nil
}
func (memory *workflowExecutionMemory) UpdateWorkflowExecution(_ context.Context, execution *domain.WorkflowExecution, expected []domain.WorkflowExecutionStatus) (bool, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	current := memory.executions[execution.ID]
	if current == nil || !executionStatusContains(expected, current.Status) {
		return false, nil
	}
	memory.executions[execution.ID] = cloneWorkflowExecutionTest(execution)
	return true, nil
}
func (memory *workflowExecutionMemory) FinalizeWorkflowExecution(ctx context.Context, execution *domain.WorkflowExecution, expected []domain.WorkflowExecutionStatus, cancelOpen bool) (bool, error) {
	updated, err := memory.UpdateWorkflowExecution(ctx, execution, expected)
	if !updated || err != nil || !cancelOpen {
		return updated, err
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, node := range memory.nodes[execution.ID] {
		if !node.Status.Terminal() {
			node.Status = domain.WorkflowNodeCancelled
			node.CompletedAt = execution.CompletedAt
			node.UpdatedAt = execution.UpdatedAt
		}
	}
	return true, nil
}
func (memory *workflowExecutionMemory) CancelWorkflowExecution(_ context.Context, orgID, workflowID, executionID uuid.UUID, now time.Time) (*domain.WorkflowExecution, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	execution := memory.executions[executionID]
	if execution == nil || execution.OrgID != orgID || execution.WorkflowID != workflowID {
		return nil, domain.ErrWorkflowExecutionNotFound
	}
	if execution.Status.Terminal() {
		return nil, domain.ErrWorkflowExecutionTerminal
	}
	execution.Status = domain.WorkflowExecutionCancelled
	execution.CompletedAt = &now
	execution.UpdatedAt = now
	for _, node := range memory.nodes[executionID] {
		if !node.Status.Terminal() {
			node.Status = domain.WorkflowNodeCancelled
			node.CompletedAt = &now
		}
	}
	return cloneWorkflowExecutionTest(execution), nil
}
func (memory *workflowExecutionMemory) ListWorkflowExecutionsForReconciliation(context.Context, int) ([]*domain.WorkflowExecution, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	var result []*domain.WorkflowExecution
	for _, execution := range memory.executions {
		if !execution.Status.Terminal() {
			result = append(result, cloneWorkflowExecutionTest(execution))
		}
	}
	return result, nil
}

type workflowTaskDispatcherFake struct {
	mu            sync.Mutex
	tasks         map[uuid.UUID]*domain.Task
	dispatchCount int
}

func newWorkflowTaskDispatcherFake() *workflowTaskDispatcherFake {
	return &workflowTaskDispatcherFake{tasks: make(map[uuid.UUID]*domain.Task)}
}
func (dispatcher *workflowTaskDispatcherFake) DispatchWorkflowTask(
	_ context.Context,
	orgID, executionID uuid.UUID,
	nodeID string,
	_ map[string]any,
	_ json.RawMessage,
) (*domain.Task, error) {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	dispatcher.dispatchCount++
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(executionID.String()+":"+nodeID))
	if task := dispatcher.tasks[id]; task != nil {
		return cloneExecutionTestValue(task), nil
	}
	now := time.Now().UTC()
	task := &domain.Task{
		ID: id, OrgID: orgID, Queue: "default", Status: domain.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	}
	dispatcher.tasks[id] = task
	return cloneExecutionTestValue(task), nil
}
func (dispatcher *workflowTaskDispatcherFake) GetWorkflowTask(_ context.Context, orgID, taskID uuid.UUID) (*domain.Task, error) {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	task := dispatcher.tasks[taskID]
	if task == nil || task.OrgID != orgID {
		return nil, domain.ErrTaskNotFound
	}
	return cloneExecutionTestValue(task), nil
}
func (dispatcher *workflowTaskDispatcherFake) complete(taskID uuid.UUID) {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	dispatcher.tasks[taskID].Status = domain.StatusCompleted
}

func (dispatcher *workflowTaskDispatcherFake) fail(taskID uuid.UUID, message string) {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	dispatcher.tasks[taskID].Status = domain.StatusDeadLetter
	dispatcher.tasks[taskID].LastError = message
}

type workflowWebhookDispatcherFake struct{}

func (workflowWebhookDispatcherFake) DispatchWorkflowWebhook(context.Context, *domain.WorkflowExecution, domain.WorkflowNode, json.RawMessage) (*domain.WebhookDelivery, error) {
	return nil, errors.New("unexpected webhook dispatch")
}
func (workflowWebhookDispatcherFake) GetWorkflowWebhook(context.Context, uuid.UUID, uuid.UUID) (*domain.WebhookDelivery, error) {
	return nil, domain.ErrWebhookDeliveryNotFound
}

type workflowWebhookDispatcherState struct {
	mu         sync.Mutex
	deliveries map[uuid.UUID]*domain.WebhookDelivery
	dispatches int
}

func newWorkflowWebhookDispatcherState() *workflowWebhookDispatcherState {
	return &workflowWebhookDispatcherState{deliveries: make(map[uuid.UUID]*domain.WebhookDelivery)}
}

func (dispatcher *workflowWebhookDispatcherState) DispatchWorkflowWebhook(
	_ context.Context,
	execution *domain.WorkflowExecution,
	node domain.WorkflowNode,
	_ json.RawMessage,
) (*domain.WebhookDelivery, error) {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	dispatcher.dispatches++
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(execution.ID.String()+":"+node.ID))
	if delivery := dispatcher.deliveries[id]; delivery != nil {
		return cloneExecutionTestValue(delivery), nil
	}
	delivery := &domain.WebhookDelivery{
		ID: id, OrgID: execution.OrgID, Status: domain.WebhookDeliveryPending,
	}
	dispatcher.deliveries[id] = delivery
	return cloneExecutionTestValue(delivery), nil
}

func (dispatcher *workflowWebhookDispatcherState) GetWorkflowWebhook(
	_ context.Context,
	orgID, deliveryID uuid.UUID,
) (*domain.WebhookDelivery, error) {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	delivery := dispatcher.deliveries[deliveryID]
	if delivery == nil || delivery.OrgID != orgID {
		return nil, domain.ErrWebhookDeliveryNotFound
	}
	return cloneExecutionTestValue(delivery), nil
}

func (dispatcher *workflowWebhookDispatcherState) complete(deliveryID uuid.UUID) {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	dispatcher.deliveries[deliveryID].Status = domain.WebhookDeliveryDelivered
	dispatcher.deliveries[deliveryID].AttemptCount = 1
}

func TestWorkflowExecutionLinearTaskAndIdempotency(t *testing.T) {
	orgID := uuid.New()
	workflowID := uuid.New()
	definition := domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{"queue": "default"}},
		},
		Edges: []domain.WorkflowEdge{{ID: "edge", From: "start", To: "task"}},
	}
	workflow, version := publishedExecutionTestWorkflow(orgID, workflowID, definition)
	memory := newWorkflowExecutionMemory(workflow, version)
	tasks := newWorkflowTaskDispatcherFake()
	metrics := &metricRecorderSpy{}
	service := NewWorkflowExecutionService(memory, memory, tasks, workflowWebhookDispatcherFake{}).
		WithMetricRecorder(metrics)

	result, reused, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, Input: json.RawMessage(`{"customer":"one"}`),
		IdempotencyKey: "linear-one",
	})
	if err != nil || reused || result.Status != domain.WorkflowExecutionRunning {
		t.Fatalf("start = %+v reused=%v err=%v", result, reused, err)
	}
	detail, err := service.GetExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if statusForExecutionNode(detail.Nodes, "start") != domain.WorkflowNodeSucceeded ||
		statusForExecutionNode(detail.Nodes, "task") != domain.WorkflowNodeQueued {
		t.Fatalf("initial nodes = %+v", detail.Nodes)
	}
	taskID := taskIDForExecutionNode(detail.Nodes, "task")
	tasks.complete(taskID)
	if err := service.AdvanceExecution(context.Background(), orgID, workflowID, result.ExecutionID); err != nil {
		t.Fatal(err)
	}
	detail, _ = service.GetExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	if detail.Status != domain.WorkflowExecutionSucceeded ||
		statusForExecutionNode(detail.Nodes, "task") != domain.WorkflowNodeSucceeded {
		t.Fatalf("completed detail = %+v", detail)
	}
	if !metrics.has(domain.MetricWorkflowExecutionCreated) ||
		!metrics.has(domain.MetricWorkflowExecutionStarted) ||
		!metrics.has(domain.MetricWorkflowExecutionSucceeded) ||
		!metrics.has(domain.MetricNodeExecutionStarted) ||
		!metrics.has(domain.MetricNodeExecutionSucceeded) {
		t.Fatal("workflow success lifecycle metrics were not recorded")
	}

	duplicate, reused, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, Input: json.RawMessage(`{"customer":"one"}`),
		IdempotencyKey: "linear-one",
	})
	if err != nil || !reused || duplicate.ExecutionID != result.ExecutionID || tasks.dispatchCount != 1 {
		t.Fatalf("duplicate = %+v reused=%v dispatches=%d err=%v", duplicate, reused, tasks.dispatchCount, err)
	}
	_, _, err = service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, Input: json.RawMessage(`{"customer":"different"}`),
		IdempotencyKey: "linear-one",
	})
	if !errors.Is(err, domain.ErrWorkflowExecutionIdempotencyConflict) {
		t.Fatalf("conflicting idempotency error = %v", err)
	}
}

func TestWorkflowExecutionConditionSkipsInactiveBranch(t *testing.T) {
	orgID, workflowID := uuid.New(), uuid.New()
	trueBranch := true
	falseBranch := false
	definition := domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{ID: "paid", Type: domain.WorkflowNodeCondition, Name: "Paid?", Config: map[string]any{
				"field": "input.status", "operator": "equals", "value": "paid",
			}},
			{ID: "yes", Type: domain.WorkflowNodeTask, Name: "Yes", Config: map[string]any{"queue": "yes"}},
			{ID: "no", Type: domain.WorkflowNodeTask, Name: "No", Config: map[string]any{"queue": "no"}},
		},
		Edges: []domain.WorkflowEdge{
			{ID: "a", From: "start", To: "paid"},
			{ID: "b", From: "paid", To: "yes", Condition: mustJSONRaw(map[string]any{"branch": trueBranch})},
			{ID: "c", From: "paid", To: "no", Condition: mustJSONRaw(map[string]any{"branch": falseBranch})},
		},
	}
	workflow, version := publishedExecutionTestWorkflow(orgID, workflowID, definition)
	memory := newWorkflowExecutionMemory(workflow, version)
	tasks := newWorkflowTaskDispatcherFake()
	metrics := &metricRecorderSpy{}
	service := NewWorkflowExecutionService(memory, memory, tasks, workflowWebhookDispatcherFake{}).
		WithMetricRecorder(metrics)
	result, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, Input: json.RawMessage(`{"status":"paid"}`),
		IdempotencyKey: "condition",
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, _ := service.GetExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	if statusForExecutionNode(detail.Nodes, "yes") != domain.WorkflowNodeQueued ||
		statusForExecutionNode(detail.Nodes, "no") != domain.WorkflowNodeSkipped ||
		tasks.dispatchCount != 1 {
		t.Fatalf("condition detail = %+v dispatches=%d", detail, tasks.dispatchCount)
	}
	if !metrics.has(domain.MetricNodeExecutionSkipped) {
		t.Fatal("node skipped metric was not recorded")
	}
}

func TestWorkflowExecutionTaskFailureFailsExecution(t *testing.T) {
	orgID, workflowID := uuid.New(), uuid.New()
	definition := domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{"queue": "default"}},
			{ID: "after", Type: domain.WorkflowNodeTask, Name: "After", Config: map[string]any{"queue": "after"}},
		},
		Edges: []domain.WorkflowEdge{
			{ID: "a", From: "start", To: "task"},
			{ID: "b", From: "task", To: "after"},
		},
	}
	workflow, version := publishedExecutionTestWorkflow(orgID, workflowID, definition)
	memory := newWorkflowExecutionMemory(workflow, version)
	tasks := newWorkflowTaskDispatcherFake()
	metrics := &metricRecorderSpy{}
	service := NewWorkflowExecutionService(memory, memory, tasks, workflowWebhookDispatcherFake{}).
		WithMetricRecorder(metrics)
	result, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, IdempotencyKey: "failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, _ := service.GetExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	tasks.fail(taskIDForExecutionNode(detail.Nodes, "task"), "permanent failure")
	if err := service.AdvanceExecution(context.Background(), orgID, workflowID, result.ExecutionID); err != nil {
		t.Fatal(err)
	}
	detail, _ = service.GetExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	if detail.Status != domain.WorkflowExecutionFailed ||
		detail.ErrorCode != "node_execution_failed" ||
		statusForExecutionNode(detail.Nodes, "after") != domain.WorkflowNodeCancelled ||
		tasks.dispatchCount != 1 {
		t.Fatalf("failed detail = %+v dispatches=%d", detail, tasks.dispatchCount)
	}
	if !metrics.has(domain.MetricNodeExecutionFailed) ||
		!metrics.has(domain.MetricWorkflowExecutionFailed) ||
		!metrics.has(domain.MetricNodeExecutionCancelled) {
		t.Fatal("workflow failure lifecycle metrics were not recorded")
	}
}

func TestWorkflowExecutionWebhookCompletion(t *testing.T) {
	orgID, workflowID, endpointID := uuid.New(), uuid.New(), uuid.New()
	definition := domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{ID: "notify", Type: domain.WorkflowNodeWebhook, Name: "Notify", Config: map[string]any{"endpoint_id": endpointID.String()}},
		},
		Edges: []domain.WorkflowEdge{{ID: "edge", From: "start", To: "notify"}},
	}
	workflow, version := publishedExecutionTestWorkflow(orgID, workflowID, definition)
	memory := newWorkflowExecutionMemory(workflow, version)
	webhooks := newWorkflowWebhookDispatcherState()
	service := NewWorkflowExecutionService(memory, memory, newWorkflowTaskDispatcherFake(), webhooks)
	result, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, IdempotencyKey: "webhook",
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, _ := service.GetExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	var deliveryID uuid.UUID
	for _, node := range detail.Nodes {
		if node.NodeID == "notify" && node.WebhookDeliveryID != nil {
			deliveryID = *node.WebhookDeliveryID
		}
	}
	if deliveryID == uuid.Nil || statusForExecutionNode(detail.Nodes, "notify") != domain.WorkflowNodeQueued {
		t.Fatalf("queued webhook detail = %+v", detail)
	}
	webhooks.complete(deliveryID)
	if err := service.AdvanceExecution(context.Background(), orgID, workflowID, result.ExecutionID); err != nil {
		t.Fatal(err)
	}
	detail, _ = service.GetExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	if detail.Status != domain.WorkflowExecutionSucceeded ||
		statusForExecutionNode(detail.Nodes, "notify") != domain.WorkflowNodeSucceeded ||
		webhooks.dispatches != 1 {
		t.Fatalf("completed webhook detail = %+v dispatches=%d", detail, webhooks.dispatches)
	}
}

func TestWorkflowExecutionDraftAndCancellation(t *testing.T) {
	orgID, workflowID := uuid.New(), uuid.New()
	definition := domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{ID: "delay", Type: domain.WorkflowNodeDelay, Name: "Delay", Config: map[string]any{"duration_seconds": 60}},
		},
		Edges: []domain.WorkflowEdge{{ID: "edge", From: "start", To: "delay"}},
	}
	workflow, version := publishedExecutionTestWorkflow(orgID, workflowID, definition)
	memory := newWorkflowExecutionMemory(workflow, version)
	metrics := &metricRecorderSpy{}
	service := NewWorkflowExecutionService(memory, memory, newWorkflowTaskDispatcherFake(), workflowWebhookDispatcherFake{}).
		WithMetricRecorder(metrics)
	workflow.Status = domain.WorkflowStatusDraft
	memory.workflows[workflowID] = cloneExecutionTestValue(workflow)
	if _, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, IdempotencyKey: "draft",
	}); !errors.Is(err, domain.ErrWorkflowNotPublished) {
		t.Fatalf("draft error = %v", err)
	}
	workflow.Status = domain.WorkflowStatusPublished
	memory.workflows[workflowID] = cloneExecutionTestValue(workflow)
	result, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, IdempotencyKey: "cancel",
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := service.CancelExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	if err != nil || cancelled.Status != domain.WorkflowExecutionCancelled {
		t.Fatalf("cancelled = %+v err=%v", cancelled, err)
	}
	if !metrics.has(domain.MetricWorkflowExecutionCancelled) ||
		!metrics.has(domain.MetricNodeExecutionCancelled) {
		t.Fatal("workflow cancellation metrics were not recorded")
	}
	if _, err := service.CancelExecution(context.Background(), orgID, workflowID, result.ExecutionID); !errors.Is(err, domain.ErrWorkflowExecutionTerminal) {
		t.Fatalf("terminal cancel error = %v", err)
	}
	if _, err := service.GetExecution(context.Background(), uuid.New(), workflowID, result.ExecutionID); !errors.Is(err, domain.ErrWorkflowExecutionNotFound) {
		t.Fatalf("cross-org get error = %v", err)
	}
}

func TestWorkflowExecutionConcurrentStartDispatchesOnce(t *testing.T) {
	orgID, workflowID := uuid.New(), uuid.New()
	definition := domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{"queue": "default"}},
		},
		Edges: []domain.WorkflowEdge{{ID: "edge", From: "start", To: "task"}},
	}
	workflow, version := publishedExecutionTestWorkflow(orgID, workflowID, definition)
	memory := newWorkflowExecutionMemory(workflow, version)
	tasks := newWorkflowTaskDispatcherFake()
	service := NewWorkflowExecutionService(memory, memory, tasks, workflowWebhookDispatcherFake{})

	const callers = 12
	ids := make(chan uuid.UUID, callers)
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
				OrgID: orgID, WorkflowID: workflowID,
				Input:          json.RawMessage(`{"same":true}`),
				IdempotencyKey: "concurrent",
			})
			if err != nil {
				errs <- err
				return
			}
			ids <- result.ExecutionID
		}()
	}
	wait.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent start error = %v", err)
	}
	var executionID uuid.UUID
	for id := range ids {
		if executionID == uuid.Nil {
			executionID = id
		} else if id != executionID {
			t.Fatalf("multiple execution ids: %s and %s", executionID, id)
		}
	}
	if tasks.dispatchCount != 1 {
		t.Fatalf("dispatch count = %d, want 1", tasks.dispatchCount)
	}
}

func TestWorkflowExecutionDelayReconciliation(t *testing.T) {
	orgID, workflowID := uuid.New(), uuid.New()
	definition := domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{ID: "delay", Type: domain.WorkflowNodeDelay, Name: "Delay", Config: map[string]any{"duration_seconds": 10}},
		},
		Edges: []domain.WorkflowEdge{{ID: "edge", From: "start", To: "delay"}},
	}
	workflow, version := publishedExecutionTestWorkflow(orgID, workflowID, definition)
	memory := newWorkflowExecutionMemory(workflow, version)
	service := NewWorkflowExecutionService(memory, memory, newWorkflowTaskDispatcherFake(), workflowWebhookDispatcherFake{})
	current := time.Unix(1000, 0).UTC()
	service.now = func() time.Time { return current }
	result, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, IdempotencyKey: "delay",
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, _ := service.GetExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	if detail.Status != domain.WorkflowExecutionRunning ||
		statusForExecutionNode(detail.Nodes, "delay") != domain.WorkflowNodeQueued {
		t.Fatalf("before due = %+v", detail)
	}
	current = current.Add(11 * time.Second)
	count, err := service.ReconcileActive(context.Background(), 10)
	if err != nil || count != 1 {
		t.Fatalf("reconcile count=%d err=%v", count, err)
	}
	detail, _ = service.GetExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	if detail.Status != domain.WorkflowExecutionSucceeded ||
		statusForExecutionNode(detail.Nodes, "delay") != domain.WorkflowNodeSucceeded {
		t.Fatalf("after due = %+v", detail)
	}
}

func TestWorkflowExecutionVersionSelectionIsImmutable(t *testing.T) {
	orgID, workflowID := uuid.New(), uuid.New()
	definition := domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{ID: "delay", Type: domain.WorkflowNodeDelay, Name: "Delay", Config: map[string]any{"duration_seconds": 60}},
		},
		Edges: []domain.WorkflowEdge{{ID: "edge", From: "start", To: "delay"}},
	}
	workflow, versionOne := publishedExecutionTestWorkflow(orgID, workflowID, definition)
	versionTwo := cloneExecutionTestValue(versionOne)
	versionTwo.ID = uuid.New()
	versionTwo.Version = 2
	memory := newWorkflowExecutionMemory(workflow, versionOne, versionTwo)
	service := NewWorkflowExecutionService(memory, memory, newWorkflowTaskDispatcherFake(), workflowWebhookDispatcherFake{})

	latest, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, IdempotencyKey: "latest",
	})
	if err != nil || latest.WorkflowVersion != 2 {
		t.Fatalf("latest = %+v err=%v", latest, err)
	}
	explicitVersion := 1
	explicit, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, Version: &explicitVersion,
		IdempotencyKey: "explicit",
	})
	if err != nil || explicit.WorkflowVersion != 1 {
		t.Fatalf("explicit = %+v err=%v", explicit, err)
	}
	versionThree := cloneExecutionTestValue(versionTwo)
	versionThree.ID = uuid.New()
	versionThree.Version = 3
	memory.mu.Lock()
	memory.versions[workflowID] = append(memory.versions[workflowID], versionThree)
	memory.mu.Unlock()
	detail, err := service.GetExecution(context.Background(), orgID, workflowID, latest.ExecutionID)
	if err != nil || detail.WorkflowVersion != 2 || detail.WorkflowVersionID != versionTwo.ID {
		t.Fatalf("immutable selection = %+v err=%v", detail, err)
	}
	unknownVersion := 99
	if _, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, Version: &unknownVersion,
		IdempotencyKey: "unknown",
	}); !errors.Is(err, domain.ErrWorkflowVersionNotFound) {
		t.Fatalf("unknown version error = %v", err)
	}
}

func TestWorkflowExecutionRecoversStaleUnlinkedClaim(t *testing.T) {
	orgID, workflowID := uuid.New(), uuid.New()
	definition := domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{"queue": "default"}},
		},
		Edges: []domain.WorkflowEdge{{ID: "edge", From: "start", To: "task"}},
	}
	workflow, version := publishedExecutionTestWorkflow(orgID, workflowID, definition)
	memory := newWorkflowExecutionMemory(workflow, version)
	tasks := newWorkflowTaskDispatcherFake()
	service := NewWorkflowExecutionService(memory, memory, tasks, workflowWebhookDispatcherFake{})
	current := time.Unix(2000, 0).UTC()
	service.now = func() time.Time { return current }
	result, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgID, WorkflowID: workflowID, IdempotencyKey: "recover",
	})
	if err != nil {
		t.Fatal(err)
	}
	memory.mu.Lock()
	stale := memory.nodes[result.ExecutionID]["task"]
	stale.Status = domain.WorkflowNodeRunning
	stale.QueueTaskID = nil
	stale.UpdatedAt = current.Add(-workflowNodeDispatchRecoveryAfter - time.Second)
	memory.mu.Unlock()
	tasks.mu.Lock()
	tasks.dispatchCount = 0
	tasks.mu.Unlock()
	if err := service.AdvanceExecution(context.Background(), orgID, workflowID, result.ExecutionID); err != nil {
		t.Fatal(err)
	}
	detail, _ := service.GetExecution(context.Background(), orgID, workflowID, result.ExecutionID)
	if taskIDForExecutionNode(detail.Nodes, "task") == uuid.Nil ||
		statusForExecutionNode(detail.Nodes, "task") != domain.WorkflowNodeQueued ||
		tasks.dispatchCount != 1 {
		t.Fatalf("recovered detail = %+v dispatches=%d", detail, tasks.dispatchCount)
	}
}

func TestWorkflowExecutionIdempotencyIsIsolatedByOrganization(t *testing.T) {
	definition := domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{ID: "delay", Type: domain.WorkflowNodeDelay, Name: "Delay", Config: map[string]any{"duration_seconds": 60}},
		},
		Edges: []domain.WorkflowEdge{{ID: "edge", From: "start", To: "delay"}},
	}
	orgOne, workflowOneID := uuid.New(), uuid.New()
	workflowOne, versionOne := publishedExecutionTestWorkflow(orgOne, workflowOneID, definition)
	memory := newWorkflowExecutionMemory(workflowOne, versionOne)
	orgTwo, workflowTwoID := uuid.New(), uuid.New()
	workflowTwo, versionTwo := publishedExecutionTestWorkflow(orgTwo, workflowTwoID, definition)
	memory.workflows[workflowTwoID] = workflowTwo
	memory.versions[workflowTwoID] = []*domain.WorkflowVersion{versionTwo}
	service := NewWorkflowExecutionService(memory, memory, newWorkflowTaskDispatcherFake(), workflowWebhookDispatcherFake{})

	first, _, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgOne, WorkflowID: workflowOneID, IdempotencyKey: "shared-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, reused, err := service.StartExecution(context.Background(), StartWorkflowExecutionInput{
		OrgID: orgTwo, WorkflowID: workflowTwoID, IdempotencyKey: "shared-key",
	})
	if err != nil || reused || first.ExecutionID == second.ExecutionID {
		t.Fatalf("second=%+v reused=%v first=%+v err=%v", second, reused, first, err)
	}
}

func publishedExecutionTestWorkflow(
	orgID, workflowID uuid.UUID,
	definition domain.WorkflowDefinition,
) (*domain.Workflow, *domain.WorkflowVersion) {
	now := time.Now().UTC()
	versionID := uuid.New()
	workflow := &domain.Workflow{
		ID: workflowID, OrgID: orgID, Name: "Execution", Slug: "execution",
		Status: domain.WorkflowStatusPublished, Definition: definition,
		CreatedAt: now, UpdatedAt: now,
	}
	version := &domain.WorkflowVersion{
		ID: versionID, OrgID: orgID, WorkflowID: workflowID, Version: 1,
		Name: workflow.Name, Slug: workflow.Slug, Definition: definition,
		Status: domain.WorkflowVersionStatusPublished, PublishedAt: now, CreatedAt: now,
	}
	return workflow, version
}

func cloneExecutionTestValue[T any](value *T) *T {
	encoded, _ := json.Marshal(value)
	var cloned T
	_ = json.Unmarshal(encoded, &cloned)
	return &cloned
}

func cloneWorkflowExecutionTest(
	execution *domain.WorkflowExecution,
) *domain.WorkflowExecution {
	cloned := cloneExecutionTestValue(execution)
	cloned.IdempotencyKey = execution.IdempotencyKey
	cloned.RequestFingerprint = execution.RequestFingerprint
	return cloned
}

func executionStatusContains(values []domain.WorkflowExecutionStatus, target domain.WorkflowExecutionStatus) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func statusForExecutionNode(nodes []*domain.WorkflowNodeExecution, nodeID string) domain.WorkflowNodeExecutionStatus {
	for _, node := range nodes {
		if node.NodeID == nodeID {
			return node.Status
		}
	}
	return ""
}

func taskIDForExecutionNode(nodes []*domain.WorkflowNodeExecution, nodeID string) uuid.UUID {
	for _, node := range nodes {
		if node.NodeID == nodeID && node.QueueTaskID != nil {
			return *node.QueueTaskID
		}
	}
	return uuid.Nil
}

func mustJSONRaw(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}
