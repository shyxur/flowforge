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
const workflowVersionColumns = `id, org_id, workflow_id, version, name, slug, description,
	definition, status, published_at, created_at`

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
		UPDATE workflows SET name=$1, slug=$2, description=$3, definition=$4, status=$5, updated_at=$6
		WHERE org_id=$7 AND id=$8 AND deleted_at IS NULL
	`, workflow.Name, workflow.Slug, workflow.Description, definition, workflow.Status,
		workflow.UpdatedAt, workflow.OrgID, workflow.ID)
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

func (s *PostgresStorage) PublishWorkflow(
	ctx context.Context,
	expected *domain.Workflow,
	publishedAt time.Time,
) (*domain.WorkflowVersion, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: publish workflow begin: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM workflows
		WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL
		FOR UPDATE
	`, workflowColumns), expected.OrgID, expected.ID)
	current, err := scanWorkflowResult(row, "publish workflow select")
	if err != nil {
		return nil, err
	}
	if current.Status == domain.WorkflowStatusArchived {
		return nil, domain.ErrInvalidStateTransition
	}
	if current.UpdatedAt.UnixMicro() != expected.UpdatedAt.UnixMicro() {
		return nil, domain.ErrWorkflowPublishConflict
	}

	var nextVersion int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM workflow_versions
		WHERE org_id=$1 AND workflow_id=$2
	`, current.OrgID, current.ID).Scan(&nextVersion); err != nil {
		return nil, fmt.Errorf("postgres: publish workflow next version: %w", err)
	}

	version := &domain.WorkflowVersion{
		ID: uuid.New(), OrgID: current.OrgID, WorkflowID: current.ID, Version: nextVersion,
		Name: current.Name, Slug: current.Slug, Description: current.Description,
		Definition: current.Definition, Status: domain.WorkflowVersionStatusPublished,
		PublishedAt: publishedAt, CreatedAt: publishedAt,
	}
	definition, err := json.Marshal(version.Definition)
	if err != nil {
		return nil, fmt.Errorf("postgres: marshal workflow version definition: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO workflow_versions
			(id, org_id, workflow_id, version, name, slug, description, definition, status, published_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, version.ID, version.OrgID, version.WorkflowID, version.Version, version.Name, version.Slug,
		version.Description, definition, version.Status, version.PublishedAt, version.CreatedAt); err != nil {
		return nil, fmt.Errorf("postgres: publish workflow insert version: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE workflows SET status=$1, updated_at=$2
		WHERE org_id=$3 AND id=$4 AND deleted_at IS NULL
	`, domain.WorkflowStatusPublished, publishedAt, current.OrgID, current.ID)
	if err != nil {
		return nil, fmt.Errorf("postgres: publish workflow update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, domain.ErrWorkflowNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("postgres: publish workflow commit: %w", err)
	}
	return version, nil
}

func (s *PostgresStorage) CreateWorkflowVersion(ctx context.Context, version *domain.WorkflowVersion) error {
	definition, err := json.Marshal(version.Definition)
	if err != nil {
		return fmt.Errorf("postgres: marshal workflow version definition: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO workflow_versions
			(id, org_id, workflow_id, version, name, slug, description, definition, status, published_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, version.ID, version.OrgID, version.WorkflowID, version.Version, version.Name, version.Slug,
		version.Description, definition, version.Status, version.PublishedAt, version.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create workflow version: %w", err)
	}
	return nil
}

func (s *PostgresStorage) GetWorkflowVersion(
	ctx context.Context,
	orgID, workflowID uuid.UUID,
	version int,
) (*domain.WorkflowVersion, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM workflow_versions v
		JOIN workflows w ON w.id=v.workflow_id AND w.org_id=v.org_id
		WHERE v.org_id=$1 AND v.workflow_id=$2 AND v.version=$3 AND w.deleted_at IS NULL
	`, qualifyWorkflowVersionColumns("v")), orgID, workflowID, version)
	return scanWorkflowVersionResult(row, "get workflow version")
}

func (s *PostgresStorage) ListWorkflowVersions(
	ctx context.Context,
	orgID, workflowID uuid.UUID,
) (*domain.WorkflowVersionPage, error) {
	if _, err := s.GetWorkflowByID(ctx, orgID, workflowID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT version, id, status, published_at, name, slug
		FROM workflow_versions
		WHERE org_id=$1 AND workflow_id=$2
		ORDER BY version DESC
	`, orgID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list workflow versions: %w", err)
	}
	defer rows.Close()

	page := &domain.WorkflowVersionPage{Versions: make([]domain.WorkflowVersionSummary, 0)}
	for rows.Next() {
		var summary domain.WorkflowVersionSummary
		if err := rows.Scan(&summary.Version, &summary.VersionID, &summary.Status,
			&summary.PublishedAt, &summary.Name, &summary.Slug); err != nil {
			return nil, fmt.Errorf("postgres: list workflow versions scan: %w", err)
		}
		page.Versions = append(page.Versions, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list workflow versions rows: %w", err)
	}
	return page, nil
}

func (s *PostgresStorage) GetLatestWorkflowVersion(
	ctx context.Context,
	orgID, workflowID uuid.UUID,
) (*domain.WorkflowVersion, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM workflow_versions v
		JOIN workflows w ON w.id=v.workflow_id AND w.org_id=v.org_id
		WHERE v.org_id=$1 AND v.workflow_id=$2 AND w.deleted_at IS NULL
		ORDER BY v.version DESC
		LIMIT 1
	`, qualifyWorkflowVersionColumns("v")), orgID, workflowID)
	return scanWorkflowVersionResult(row, "get latest workflow version")
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

func scanWorkflowVersionResult(row workflowScanner, operation string) (*domain.WorkflowVersion, error) {
	version, err := scanWorkflowVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrWorkflowVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: %s: %w", operation, err)
	}
	return version, nil
}

func scanWorkflowVersion(row workflowScanner) (*domain.WorkflowVersion, error) {
	version := &domain.WorkflowVersion{}
	var definition []byte
	if err := row.Scan(&version.ID, &version.OrgID, &version.WorkflowID, &version.Version,
		&version.Name, &version.Slug, &version.Description, &definition, &version.Status,
		&version.PublishedAt, &version.CreatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(definition, &version.Definition); err != nil {
		return nil, fmt.Errorf("decode workflow version definition: %w", err)
	}
	return version, nil
}

func qualifyWorkflowVersionColumns(alias string) string {
	columns := strings.Split(workflowVersionColumns, ",")
	for index := range columns {
		columns[index] = alias + "." + strings.TrimSpace(columns[index])
	}
	return strings.Join(columns, ", ")
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
