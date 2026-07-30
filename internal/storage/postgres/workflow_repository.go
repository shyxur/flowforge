package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/ports"
)

const workflowColumns = `id, org_id, name, slug, description, status, definition,
	created_at, updated_at, deleted_at`

var _ ports.WorkflowRepository = (*PostgresStorage)(nil)

type workflowScanner interface {
	Scan(...any) error
}

func (s *PostgresStorage) CreateWorkflow(ctx context.Context, workflow *domain.Workflow) error {
	definition, err := json.Marshal(workflow.Definition)
	if err != nil {
		return fmt.Errorf("postgres: marshal workflow definition: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO workflows (id, org_id, name, slug, description, status, definition, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, workflow.ID, workflow.OrgID, workflow.Name, workflow.Slug, workflow.Description,
		workflow.Status, definition, workflow.CreatedAt, workflow.UpdatedAt)
	if isUniqueViolation(err) {
		return domain.ErrWorkflowSlugConflict
	}
	if err != nil {
		return fmt.Errorf("postgres: create workflow: %w", err)
	}
	return nil
}

func (s *PostgresStorage) GetWorkflowByID(ctx context.Context, orgID, id uuid.UUID) (*domain.Workflow, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM workflows WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL
	`, workflowColumns), orgID, id)
	return scanWorkflowResult(row, "get workflow")
}

func (s *PostgresStorage) GetWorkflowBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*domain.Workflow, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM workflows WHERE org_id=$1 AND slug=$2 AND deleted_at IS NULL
	`, workflowColumns), orgID, slug)
	return scanWorkflowResult(row, "get workflow by slug")
}

func (s *PostgresStorage) ListWorkflows(ctx context.Context, orgID uuid.UUID, filter domain.WorkflowFilter) (*domain.WorkflowPage, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	args := []any{orgID}
	var query strings.Builder
	fmt.Fprintf(&query, "SELECT %s FROM workflows WHERE org_id=$1 AND deleted_at IS NULL", workflowColumns)
	if filter.Status != "" {
		args = append(args, filter.Status)
		fmt.Fprintf(&query, " AND status=$%d", len(args))
	}
	if filter.Cursor != "" {
		cursorTime, cursorID, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, domain.ErrInvalidInput
		}
		args = append(args, cursorTime, cursorID)
		fmt.Fprintf(&query, " AND (created_at,id) < ($%d,$%d)", len(args)-1, len(args))
	}
	args = append(args, filter.Limit+1)
	fmt.Fprintf(&query, " ORDER BY created_at DESC,id DESC LIMIT $%d", len(args))

	rows, err := s.pool.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list workflows: %w", err)
	}
	defer rows.Close()

	page := &domain.WorkflowPage{Workflows: make([]*domain.Workflow, 0, filter.Limit)}
	for rows.Next() {
		workflow, scanErr := scanWorkflow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: list workflows scan: %w", scanErr)
		}
		page.Workflows = append(page.Workflows, workflow)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list workflows rows: %w", err)
	}
	if len(page.Workflows) > filter.Limit {
		page.Workflows = page.Workflows[:filter.Limit]
		last := page.Workflows[len(page.Workflows)-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s *PostgresStorage) UpdateWorkflow(ctx context.Context, workflow *domain.Workflow) error {
	definition, err := json.Marshal(workflow.Definition)
	if err != nil {
		return fmt.Errorf("postgres: marshal workflow definition: %w", err)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE workflows SET name=$1, slug=$2, description=$3, definition=$4, updated_at=$5
		WHERE org_id=$6 AND id=$7 AND deleted_at IS NULL
	`, workflow.Name, workflow.Slug, workflow.Description, definition, workflow.UpdatedAt,
		workflow.OrgID, workflow.ID)
	if isUniqueViolation(err) {
		return domain.ErrWorkflowSlugConflict
	}
	if err != nil {
		return fmt.Errorf("postgres: update workflow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrWorkflowNotFound
	}
	return nil
}

func (s *PostgresStorage) SoftDeleteWorkflow(ctx context.Context, orgID, id uuid.UUID, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE workflows SET deleted_at=$1, updated_at=$1
		WHERE org_id=$2 AND id=$3 AND deleted_at IS NULL
	`, now, orgID, id)
	if err != nil {
		return fmt.Errorf("postgres: soft delete workflow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrWorkflowNotFound
	}
	return nil
}

func scanWorkflowResult(row workflowScanner, operation string) (*domain.Workflow, error) {
	workflow, err := scanWorkflow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrWorkflowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: %s: %w", operation, err)
	}
	return workflow, nil
}

func scanWorkflow(row workflowScanner) (*domain.Workflow, error) {
	workflow := &domain.Workflow{}
	var definition []byte
	if err := row.Scan(&workflow.ID, &workflow.OrgID, &workflow.Name, &workflow.Slug,
		&workflow.Description, &workflow.Status, &definition, &workflow.CreatedAt,
		&workflow.UpdatedAt, &workflow.DeletedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(definition, &workflow.Definition); err != nil {
		return nil, fmt.Errorf("decode workflow definition: %w", err)
	}
	return workflow, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
