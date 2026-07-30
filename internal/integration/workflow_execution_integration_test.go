//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	redisbroker "github.com/shyxur/windylane/internal/broker/redis"
	"github.com/shyxur/windylane/internal/domain"
	metricspkg "github.com/shyxur/windylane/internal/metrics"
	"github.com/shyxur/windylane/internal/storage/postgres"
	"github.com/shyxur/windylane/internal/usecase"
)

func TestWorkflowExecutionRepository(t *testing.T) {
	if os.Getenv("QUEUEFLOW_INTEGRATION") != "1" {
		t.Skip("run with make test-integration")
	}
	ctx := context.Background()
	dsn := os.Getenv("INTEGRATION_DB_DSN")
	if dsn == "" {
		t.Fatal("integration database address is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `
		TRUNCATE metric_events, workflow_node_executions, workflow_executions, workflow_versions, workflows
	`); err != nil {
		t.Fatal(err)
	}
	storage, err := postgres.NewPostgresStorage(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	orgID := uuid.MustParse(devOrgID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	workflow := &domain.Workflow{
		ID: uuid.New(), OrgID: orgID, Name: "Execution integration",
		Slug: "execution-integration", Status: domain.WorkflowStatusPublished,
		Definition: domain.WorkflowDefinition{
			Nodes: []domain.WorkflowNode{
				{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
				{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{"queue": "default"}},
			},
			Edges: []domain.WorkflowEdge{{ID: "edge", From: "start", To: "task"}},
		},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatal(err)
	}
	version := &domain.WorkflowVersion{
		ID: uuid.New(), OrgID: orgID, WorkflowID: workflow.ID, Version: 1,
		Name: workflow.Name, Slug: workflow.Slug, Definition: workflow.Definition,
		Status:      domain.WorkflowVersionStatusPublished,
		PublishedAt: now, CreatedAt: now,
	}
	if err := storage.CreateWorkflowVersion(ctx, version); err != nil {
		t.Fatal(err)
	}
	executionVersion, err := storage.GetWorkflowExecutionVersion(
		ctx, orgID, workflow.ID, version.ID,
	)
	if err != nil || executionVersion.Version != 1 {
		t.Fatalf("get execution version = %+v err=%v", executionVersion, err)
	}
	execution := &domain.WorkflowExecution{
		ID: uuid.New(), OrgID: orgID, WorkflowID: workflow.ID,
		WorkflowVersionID: version.ID, WorkflowVersion: 1,
		Status:             domain.WorkflowExecutionPending,
		Input:              []byte(`{"status":"paid"}`),
		IdempotencyKey:     "execution-integration",
		RequestFingerprint: "fingerprint-one",
		CreatedAt:          now, UpdatedAt: now,
	}
	nodes := []*domain.WorkflowNodeExecution{
		{
			ID: uuid.New(), OrgID: orgID, WorkflowExecutionID: execution.ID,
			NodeID: "start", NodeType: domain.WorkflowNodeTrigger,
			Status: domain.WorkflowNodePending, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: uuid.New(), OrgID: orgID, WorkflowExecutionID: execution.ID,
			NodeID: "task", NodeType: domain.WorkflowNodeTask,
			Status: domain.WorkflowNodePending, CreatedAt: now, UpdatedAt: now,
		},
	}
	created, reused, err := storage.CreateWorkflowExecution(ctx, execution, nodes)
	if err != nil || reused || created.ID != execution.ID {
		t.Fatalf("create execution = %+v reused=%v err=%v", created, reused, err)
	}
	duplicate := *execution
	duplicate.ID = uuid.New()
	duplicateNodes := []*domain.WorkflowNodeExecution{{
		ID: uuid.New(), OrgID: orgID, WorkflowExecutionID: duplicate.ID,
		NodeID: "start", NodeType: domain.WorkflowNodeTrigger,
		Status: domain.WorkflowNodePending, CreatedAt: now, UpdatedAt: now,
	}}
	existing, reused, err := storage.CreateWorkflowExecution(ctx, &duplicate, duplicateNodes)
	if err != nil || !reused || existing.ID != execution.ID {
		t.Fatalf("duplicate execution = %+v reused=%v err=%v", existing, reused, err)
	}
	duplicate.RequestFingerprint = "different"
	if _, _, err := storage.CreateWorkflowExecution(ctx, &duplicate, duplicateNodes); !errors.Is(err, domain.ErrWorkflowExecutionIdempotencyConflict) {
		t.Fatalf("idempotency conflict error = %v", err)
	}

	secondExecution := *execution
	secondExecution.ID = uuid.New()
	secondExecution.IdempotencyKey = "execution-integration-two"
	secondExecution.RequestFingerprint = "fingerprint-two"
	secondExecution.CreatedAt = now.Add(time.Millisecond)
	secondExecution.UpdatedAt = secondExecution.CreatedAt
	secondNodes := []*domain.WorkflowNodeExecution{{
		ID: uuid.New(), OrgID: orgID, WorkflowExecutionID: secondExecution.ID,
		NodeID: "start", NodeType: domain.WorkflowNodeTrigger,
		Status:    domain.WorkflowNodePending,
		CreatedAt: secondExecution.CreatedAt, UpdatedAt: secondExecution.UpdatedAt,
	}}
	if _, reused, err := storage.CreateWorkflowExecution(ctx, &secondExecution, secondNodes); err != nil || reused {
		t.Fatalf("second execution reused=%v err=%v", reused, err)
	}
	page, err := storage.ListWorkflowExecutions(ctx, orgID, workflow.ID, domain.WorkflowExecutionFilter{Limit: 1})
	if err != nil || len(page.Executions) != 1 || page.NextCursor == "" {
		t.Fatalf("list executions = %+v err=%v", page, err)
	}
	nextPage, err := storage.ListWorkflowExecutions(ctx, orgID, workflow.ID, domain.WorkflowExecutionFilter{
		Limit: 1, Cursor: page.NextCursor,
	})
	if err != nil || len(nextPage.Executions) != 1 || nextPage.Executions[0].ID == page.Executions[0].ID {
		t.Fatalf("next execution page = %+v err=%v", nextPage, err)
	}
	storedNodes, err := storage.GetWorkflowNodeExecutions(ctx, orgID, execution.ID)
	if err != nil || len(storedNodes) != 2 || storedNodes[0].NodeID != "start" {
		t.Fatalf("node executions = %+v err=%v", storedNodes, err)
	}
	claimed, won, err := storage.ClaimWorkflowNode(ctx, orgID, execution.ID, "start", now.Add(time.Second))
	if err != nil || !won || claimed.Attempt != 1 {
		t.Fatalf("claim node = %+v won=%v err=%v", claimed, won, err)
	}
	if _, won, err := storage.ClaimWorkflowNode(ctx, orgID, execution.ID, "start", now.Add(time.Second)); err != nil || won {
		t.Fatalf("duplicate claim won=%v err=%v", won, err)
	}
	claimed.Status = domain.WorkflowNodeSucceeded
	completedAt := now.Add(2 * time.Second)
	claimed.CompletedAt = &completedAt
	claimed.UpdatedAt = completedAt
	if updated, err := storage.UpdateWorkflowNodeExecution(ctx, claimed, domain.WorkflowNodeRunning); err != nil || !updated {
		t.Fatalf("complete node updated=%v err=%v", updated, err)
	}
	if updated, err := storage.UpdateWorkflowNodeExecution(ctx, claimed, domain.WorkflowNodeRunning); err != nil || updated {
		t.Fatalf("duplicate completion updated=%v err=%v", updated, err)
	}

	cancelled, err := storage.CancelWorkflowExecution(ctx, orgID, workflow.ID, execution.ID, now.Add(3*time.Second))
	if err != nil || cancelled.Status != domain.WorkflowExecutionCancelled {
		t.Fatalf("cancel execution = %+v err=%v", cancelled, err)
	}
	if _, err := storage.CancelWorkflowExecution(ctx, orgID, workflow.ID, execution.ID, now.Add(4*time.Second)); !errors.Is(err, domain.ErrWorkflowExecutionTerminal) {
		t.Fatalf("terminal cancel error = %v", err)
	}
	if _, err := storage.CancelWorkflowExecution(ctx, orgID, workflow.ID, secondExecution.ID, now.Add(4*time.Second)); err != nil {
		t.Fatalf("cancel second execution: %v", err)
	}
	if _, err := storage.GetWorkflowExecution(ctx, uuid.New(), workflow.ID, execution.ID); !errors.Is(err, domain.ErrWorkflowExecutionNotFound) {
		t.Fatalf("cross-org execution read error = %v", err)
	}
	reconcile, err := storage.ListWorkflowExecutionsForReconciliation(ctx, 10)
	if err != nil || len(reconcile) != 0 {
		t.Fatalf("reconciliation list = %+v err=%v", reconcile, err)
	}
}

func TestWorkflowExecutionLifecycleIntegration(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `
		TRUNCATE metric_events, workflow_node_executions, workflow_executions, workflow_versions,
			workflows, webhook_deliveries, webhook_endpoints, tasks
	`); err != nil {
		t.Fatal(err)
	}
	storage, err := postgres.NewPostgresStorage(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	metricRecorder := metricspkg.NewBufferedRecorder(
		usecase.NewMetricsService(storage),
		metricspkg.Config{
			Capacity: 256, BatchSize: 20,
			FlushInterval: time.Hour, WriteTimeout: time.Second,
		},
		nil,
	)
	broker := redisbroker.NewRedisBroker(redisAddr, "", 0)
	defer broker.Close()
	if err := broker.Client().FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	orgID := uuid.MustParse(devOrgID)
	workflowService := usecase.NewWorkflowService(storage)
	taskService := usecase.NewService(storage, broker).WithMetricRecorder(metricRecorder)
	executionService := usecase.NewWorkflowExecutionService(
		storage,
		storage,
		usecase.NewQueueFlowWorkflowTaskDispatcher(taskService),
		usecase.NewEventForgeWorkflowWebhookDispatcher(storage, storage, 3).
			WithMetricRecorder(metricRecorder),
	).WithMetricRecorder(metricRecorder)

	linearDefinition := json.RawMessage(`{
		"nodes": [
			{"id":"start","type":"trigger","name":"Original trigger","config":{}},
			{"id":"task","type":"task","name":"Original task","config":{"queue":"integration"}}
		],
		"edges": [{"id":"start-task","from":"start","to":"task","condition":null}]
	}`)
	workflow, err := workflowService.CreateWorkflow(ctx, usecase.CreateWorkflowInput{
		OrgID: orgID, Name: "Lifecycle integration",
		Slug: "lifecycle-integration", Definition: linearDefinition,
	})
	if err != nil {
		t.Fatal(err)
	}
	validation, err := workflowService.ValidateWorkflow(ctx, orgID, workflow.ID)
	if err != nil || !validation.Valid {
		t.Fatalf("validate workflow = %+v err=%v", validation, err)
	}
	v1, err := workflowService.PublishWorkflow(ctx, orgID, workflow.ID)
	if err != nil || v1.Version != 1 {
		t.Fatalf("publish v1 = %+v err=%v", v1, err)
	}

	started, reused, err := executionService.StartExecution(
		ctx,
		usecase.StartWorkflowExecutionInput{
			OrgID: orgID, WorkflowID: workflow.ID,
			Input: json.RawMessage(`{"customer":"one"}`), IdempotencyKey: "lifecycle-v1",
		},
	)
	if err != nil || reused || started.WorkflowVersion != 1 {
		t.Fatalf("start execution = %+v reused=%v err=%v", started, reused, err)
	}
	detail, err := executionService.GetExecution(ctx, orgID, workflow.ID, started.ExecutionID)
	if err != nil || len(detail.Nodes) != 2 {
		t.Fatalf("initial detail = %+v err=%v", detail, err)
	}
	var taskNode *domain.WorkflowNodeExecution
	for _, node := range detail.Nodes {
		if node.NodeID == "task" {
			taskNode = node
		}
	}
	if taskNode == nil || taskNode.Status != domain.WorkflowNodeQueued || taskNode.QueueTaskID == nil {
		t.Fatalf("queued task node = %+v", taskNode)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := storage.ClaimForProcessing(ctx, orgID, *taskNode.QueueTaskID, "integration-worker", now); err != nil {
		t.Fatal(err)
	}
	if err := storage.Complete(ctx, orgID, *taskNode.QueueTaskID, now.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := executionService.AdvanceExecution(ctx, orgID, workflow.ID, started.ExecutionID); err != nil {
		t.Fatal(err)
	}
	detail, err = executionService.GetExecution(ctx, orgID, workflow.ID, started.ExecutionID)
	if err != nil || detail.Status != domain.WorkflowExecutionSucceeded {
		t.Fatalf("terminal detail = %+v err=%v", detail, err)
	}
	replayed, reused, err := executionService.StartExecution(
		ctx,
		usecase.StartWorkflowExecutionInput{
			OrgID: orgID, WorkflowID: workflow.ID,
			Input: json.RawMessage(`{"customer":"one"}`), IdempotencyKey: "lifecycle-v1",
		},
	)
	if err != nil || !reused || replayed.ExecutionID != started.ExecutionID {
		t.Fatalf("idempotent replay = %+v reused=%v err=%v", replayed, reused, err)
	}
	if _, _, err := executionService.StartExecution(
		ctx,
		usecase.StartWorkflowExecutionInput{
			OrgID: orgID, WorkflowID: workflow.ID,
			Input: json.RawMessage(`{"customer":"different"}`), IdempotencyKey: "lifecycle-v1",
		},
	); !errors.Is(err, domain.ErrWorkflowExecutionIdempotencyConflict) {
		t.Fatalf("idempotency conflict = %v", err)
	}
	page, err := executionService.ListExecutions(
		ctx, orgID, workflow.ID, domain.WorkflowExecutionFilter{Limit: 1},
	)
	if err != nil || len(page.Executions) != 1 {
		t.Fatalf("list executions = %+v err=%v", page, err)
	}
	if _, err := executionService.GetExecution(
		ctx, uuid.New(), workflow.ID, started.ExecutionID,
	); !errors.Is(err, domain.ErrWorkflowExecutionNotFound) {
		t.Fatalf("cross-org detail error = %v", err)
	}

	updatedDefinition := json.RawMessage(`{
		"nodes": [
			{"id":"start","type":"trigger","name":"Updated trigger","config":{}},
			{"id":"task","type":"task","name":"Updated task","config":{"queue":"integration"}}
		],
		"edges": [{"id":"start-task","from":"start","to":"task","condition":null}]
	}`)
	updatedName := "Lifecycle integration v2"
	if _, err := workflowService.UpdateWorkflow(ctx, orgID, workflow.ID, usecase.UpdateWorkflowInput{
		Name: &updatedName, Definition: updatedDefinition,
	}); err != nil {
		t.Fatal(err)
	}
	v2, err := workflowService.PublishWorkflow(ctx, orgID, workflow.ID)
	if err != nil || v2.Version != 2 {
		t.Fatalf("publish v2 = %+v err=%v", v2, err)
	}
	explicitV1 := 1
	historical, _, err := executionService.StartExecution(
		ctx,
		usecase.StartWorkflowExecutionInput{
			OrgID: orgID, WorkflowID: workflow.ID, Version: &explicitV1,
			IdempotencyKey: "historical-v1",
		},
	)
	if err != nil || historical.WorkflowVersion != 1 {
		t.Fatalf("historical execution = %+v err=%v", historical, err)
	}
	historicalVersion, err := workflowService.GetWorkflowVersion(ctx, orgID, workflow.ID, 1)
	if err != nil || historicalVersion.Definition.Nodes[0].Name != "Original trigger" {
		t.Fatalf("immutable v1 = %+v err=%v", historicalVersion, err)
	}

	delayDefinition := json.RawMessage(`{
		"nodes": [
			{"id":"start","type":"trigger","name":"Start","config":{}},
			{"id":"wait","type":"delay","name":"Wait","config":{"duration_seconds":3600}},
			{"id":"after","type":"task","name":"After","config":{"queue":"integration"}}
		],
		"edges": [
			{"id":"start-wait","from":"start","to":"wait","condition":null},
			{"id":"wait-after","from":"wait","to":"after","condition":null}
		]
	}`)
	delayWorkflow, err := workflowService.CreateWorkflow(ctx, usecase.CreateWorkflowInput{
		OrgID: orgID, Name: "Cancellation integration",
		Slug: "cancellation-integration", Definition: delayDefinition,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflowService.PublishWorkflow(ctx, orgID, delayWorkflow.ID); err != nil {
		t.Fatal(err)
	}
	cancellable, _, err := executionService.StartExecution(
		ctx,
		usecase.StartWorkflowExecutionInput{
			OrgID: orgID, WorkflowID: delayWorkflow.ID,
			IdempotencyKey: "cancel-delay",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := executionService.CancelExecution(
		ctx, orgID, delayWorkflow.ID, cancellable.ExecutionID,
	)
	if err != nil || cancelled.Status != domain.WorkflowExecutionCancelled {
		t.Fatalf("cancel execution = %+v err=%v", cancelled, err)
	}
	cancelledDetail, err := executionService.GetExecution(
		ctx, orgID, delayWorkflow.ID, cancellable.ExecutionID,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range cancelledDetail.Nodes {
		if node.NodeID == "after" && node.Status != domain.WorkflowNodeCancelled {
			t.Fatalf("downstream node after cancel = %+v", node)
		}
	}
	if _, err := executionService.CancelExecution(
		ctx, orgID, delayWorkflow.ID, cancellable.ExecutionID,
	); !errors.Is(err, domain.ErrWorkflowExecutionTerminal) {
		t.Fatalf("terminal cancellation error = %v", err)
	}

	conditionDefinition := json.RawMessage(`{
		"nodes": [
			{"id":"start","type":"trigger","name":"Start","config":{}},
			{"id":"route","type":"condition","name":"Route","config":{"field":"input.ready","operator":"equals","value":true}},
			{"id":"yes","type":"delay","name":"True branch","config":{"duration_seconds":0}},
			{"id":"no","type":"delay","name":"False branch","config":{"duration_seconds":0}}
		],
		"edges": [
			{"id":"start-route","from":"start","to":"route","condition":null},
			{"id":"route-yes","from":"route","to":"yes","condition":{"branch":true}},
			{"id":"route-no","from":"route","to":"no","condition":{"branch":false}}
		]
	}`)
	conditionWorkflow, err := workflowService.CreateWorkflow(ctx, usecase.CreateWorkflowInput{
		OrgID: orgID, Name: "Condition integration",
		Slug: "condition-integration", Definition: conditionDefinition,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflowService.PublishWorkflow(ctx, orgID, conditionWorkflow.ID); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name       string
		input      string
		activeNode string
		skipNode   string
	}{
		{name: "true", input: `{"ready":true}`, activeNode: "yes", skipNode: "no"},
		{name: "false", input: `{"ready":false}`, activeNode: "no", skipNode: "yes"},
	} {
		t.Run("condition_"+scenario.name, func(t *testing.T) {
			result, _, err := executionService.StartExecution(
				ctx,
				usecase.StartWorkflowExecutionInput{
					OrgID: orgID, WorkflowID: conditionWorkflow.ID,
					Input:          json.RawMessage(scenario.input),
					IdempotencyKey: "condition-" + scenario.name,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := executionService.AdvanceExecution(
				ctx, orgID, conditionWorkflow.ID, result.ExecutionID,
			); err != nil {
				t.Fatal(err)
			}
			detail, err := executionService.GetExecution(
				ctx, orgID, conditionWorkflow.ID, result.ExecutionID,
			)
			if err != nil || detail.Status != domain.WorkflowExecutionSucceeded {
				t.Fatalf("condition detail = %+v err=%v", detail, err)
			}
			statuses := make(map[string]domain.WorkflowNodeExecutionStatus)
			for _, node := range detail.Nodes {
				statuses[node.NodeID] = node.Status
			}
			if statuses[scenario.activeNode] != domain.WorkflowNodeSucceeded ||
				statuses[scenario.skipNode] != domain.WorkflowNodeSkipped {
				t.Fatalf("condition statuses = %+v", statuses)
			}
		})
	}

	endpointID := uuid.New()
	endpoint := &domain.WebhookEndpoint{
		ID: endpointID, OrgID: orgID, Name: "Workflow integration endpoint",
		URL: "https://example.com/workflow", SecretHash: "hash",
		SecretCiphertext: "ciphertext", EventTypes: []domain.WebhookEventType{
			domain.WebhookEventTaskCompleted,
		},
		IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateWebhookEndpoint(ctx, endpoint); err != nil {
		t.Fatal(err)
	}
	webhookDefinition, err := json.Marshal(domain.WorkflowDefinition{
		Nodes: []domain.WorkflowNode{
			{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
			{
				ID: "notify", Type: domain.WorkflowNodeWebhook, Name: "Notify",
				Config: map[string]any{"endpoint_id": endpointID.String()},
			},
		},
		Edges: []domain.WorkflowEdge{{ID: "start-notify", From: "start", To: "notify"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	webhookWorkflow, err := workflowService.CreateWorkflow(ctx, usecase.CreateWorkflowInput{
		OrgID: orgID, Name: "Webhook integration",
		Slug: "webhook-integration-workflow", Definition: webhookDefinition,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflowService.PublishWorkflow(ctx, orgID, webhookWorkflow.ID); err != nil {
		t.Fatal(err)
	}
	webhookExecution, _, err := executionService.StartExecution(
		ctx,
		usecase.StartWorkflowExecutionInput{
			OrgID: orgID, WorkflowID: webhookWorkflow.ID,
			IdempotencyKey: "webhook-execution",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	webhookDetail, err := executionService.GetExecution(
		ctx, orgID, webhookWorkflow.ID, webhookExecution.ExecutionID,
	)
	if err != nil {
		t.Fatal(err)
	}
	var deliveryID *uuid.UUID
	for _, node := range webhookDetail.Nodes {
		if node.NodeID == "notify" {
			deliveryID = node.WebhookDeliveryID
		}
	}
	if deliveryID == nil {
		t.Fatal("webhook node did not create an EventForge delivery")
	}
	delivery, err := storage.GetWebhookDelivery(ctx, orgID, *deliveryID)
	if err != nil {
		t.Fatal(err)
	}
	delivery.Status = domain.WebhookDeliveryDelivered
	delivery.AttemptCount = 1
	deliveredAt := now.Add(time.Second)
	delivery.LastAttemptAt = &deliveredAt
	delivery.UpdatedAt = deliveredAt
	if err := storage.UpdateWebhookDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	if err := executionService.AdvanceExecution(
		ctx, orgID, webhookWorkflow.ID, webhookExecution.ExecutionID,
	); err != nil {
		t.Fatal(err)
	}
	webhookDetail, err = executionService.GetExecution(
		ctx, orgID, webhookWorkflow.ID, webhookExecution.ExecutionID,
	)
	if err != nil || webhookDetail.Status != domain.WorkflowExecutionSucceeded {
		t.Fatalf("webhook terminal detail = %+v err=%v", webhookDetail, err)
	}
	closeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := metricRecorder.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	assertMetricTypes(
		t, ctx, storage, orgID, domain.MetricSourceTaskCanvas,
		domain.MetricWorkflowExecutionCreated, domain.MetricWorkflowExecutionStarted,
		domain.MetricWorkflowExecutionSucceeded, domain.MetricWorkflowExecutionCancelled,
		domain.MetricNodeExecutionStarted, domain.MetricNodeExecutionSucceeded,
		domain.MetricNodeExecutionSkipped, domain.MetricNodeExecutionCancelled,
	)
}
