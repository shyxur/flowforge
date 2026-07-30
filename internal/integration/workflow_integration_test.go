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

func TestWorkflowRepository(t *testing.T) {
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
	if _, err := pool.Exec(ctx, "TRUNCATE workflow_node_executions, workflow_executions, workflow_versions, workflows"); err != nil {
		t.Fatal(err)
	}
	storage, err := postgres.NewPostgresStorage(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	orgID := uuid.MustParse(devOrgID)
	now := time.Now().UTC()
	workflow := &domain.Workflow{
		ID: uuid.New(), OrgID: orgID, Name: "Integration workflow", Slug: "integration-workflow",
		Status: domain.WorkflowStatusDraft,
		Definition: domain.WorkflowDefinition{
			Nodes: []domain.WorkflowNode{
				{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
				{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{}},
			},
			Edges: []domain.WorkflowEdge{{ID: "start-task", From: "start", To: "task"}},
		},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if _, err := storage.GetWorkflowByID(ctx, orgID, workflow.ID); err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if _, err := storage.GetWorkflowBySlug(ctx, orgID, workflow.Slug); err != nil {
		t.Fatalf("get workflow by slug: %v", err)
	}
	duplicate := *workflow
	duplicate.ID = uuid.New()
	if err := storage.CreateWorkflow(ctx, &duplicate); !errors.Is(err, domain.ErrWorkflowSlugConflict) {
		t.Fatalf("same-org duplicate slug error = %v", err)
	}
	secondOrg := uuid.New()
	if _, err := pool.Exec(ctx, "INSERT INTO organizations (id, name) VALUES ($1,$2)", secondOrg, "Workflow integration tenant"); err != nil {
		t.Fatalf("create second workflow org: %v", err)
	}
	crossOrg := duplicate
	crossOrg.ID = uuid.New()
	crossOrg.OrgID = secondOrg
	if err := storage.CreateWorkflow(ctx, &crossOrg); err != nil {
		t.Fatalf("cross-org duplicate slug: %v", err)
	}
	if _, err := storage.GetWorkflowByID(ctx, secondOrg, workflow.ID); !errors.Is(err, domain.ErrWorkflowNotFound) {
		t.Fatalf("cross-org workflow read error = %v", err)
	}
	workflow.Name = "Updated integration workflow"
	workflow.UpdatedAt = now.Add(time.Second)
	if err := storage.UpdateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("update workflow: %v", err)
	}
	page, err := storage.ListWorkflows(ctx, orgID, domain.WorkflowFilter{Status: domain.WorkflowStatusDraft, Limit: 10})
	if err != nil || len(page.Workflows) != 1 || page.Workflows[0].Name != workflow.Name {
		t.Fatalf("list workflows = %+v err=%v", page, err)
	}
	first, err := storage.PublishWorkflow(ctx, workflow, now.Add(2*time.Second))
	if err != nil || first.Version != 1 {
		t.Fatalf("first publish = %+v err=%v", first, err)
	}
	published, err := storage.GetWorkflowByID(ctx, orgID, workflow.ID)
	if err != nil || published.Status != domain.WorkflowStatusPublished {
		t.Fatalf("published workflow = %+v err=%v", published, err)
	}
	published.Name = "Second snapshot"
	published.Definition.Nodes[1].Name = "Changed task"
	published.Status = domain.WorkflowStatusDraft
	published.UpdatedAt = now.Add(3 * time.Second)
	if err := storage.UpdateWorkflow(ctx, published); err != nil {
		t.Fatalf("update published workflow draft: %v", err)
	}
	second, err := storage.PublishWorkflow(ctx, published, now.Add(4*time.Second))
	if err != nil || second.Version != 2 {
		t.Fatalf("second publish = %+v err=%v", second, err)
	}
	firstSnapshot, err := storage.GetWorkflowVersion(ctx, orgID, workflow.ID, 1)
	if err != nil || firstSnapshot.Name != workflow.Name || firstSnapshot.Definition.Nodes[1].Name != "Task" {
		t.Fatalf("immutable first snapshot = %+v err=%v", firstSnapshot, err)
	}
	latest, err := storage.GetLatestWorkflowVersion(ctx, orgID, workflow.ID)
	if err != nil || latest.Version != 2 {
		t.Fatalf("latest version = %+v err=%v", latest, err)
	}
	versions, err := storage.ListWorkflowVersions(ctx, orgID, workflow.ID)
	if err != nil || len(versions.Versions) != 2 || versions.Versions[0].Version != 2 {
		t.Fatalf("versions = %+v err=%v", versions, err)
	}
	if _, err := storage.GetWorkflowVersion(ctx, secondOrg, workflow.ID, 1); !errors.Is(err, domain.ErrWorkflowVersionNotFound) {
		t.Fatalf("cross-org workflow version read error = %v", err)
	}
	if _, err := storage.ListWorkflowVersions(ctx, secondOrg, workflow.ID); !errors.Is(err, domain.ErrWorkflowNotFound) {
		t.Fatalf("cross-org workflow versions list error = %v", err)
	}

	if err := storage.SoftDeleteWorkflow(ctx, orgID, workflow.ID, now.Add(5*time.Second)); err != nil {
		t.Fatalf("soft delete workflow: %v", err)
	}
	if _, err := storage.GetWorkflowByID(ctx, orgID, workflow.ID); !errors.Is(err, domain.ErrWorkflowNotFound) {
		t.Fatalf("soft-deleted workflow read error = %v", err)
	}
	page, err = storage.ListWorkflows(ctx, orgID, domain.WorkflowFilter{Limit: 10})
	if err != nil || len(page.Workflows) != 0 {
		t.Fatalf("soft-deleted workflow list = %+v err=%v", page, err)
	}
}
