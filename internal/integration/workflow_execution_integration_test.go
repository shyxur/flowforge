//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/storage/postgres"
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
		TRUNCATE workflow_node_executions, workflow_executions, workflow_versions, workflows
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
