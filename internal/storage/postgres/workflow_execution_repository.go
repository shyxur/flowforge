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

const workflowExecutionColumns = `id, org_id, workflow_id, workflow_version_id,
	workflow_version, status, input, output, error_code, error_message,
	idempotency_key, request_fingerprint, started_at, completed_at, created_at, updated_at`

const workflowNodeExecutionColumns = `id, org_id, workflow_execution_id, node_id,
	node_type, status, attempt, input, output, error_code, error_message,
	queue_task_id, webhook_delivery_id, available_at, started_at, completed_at,
	created_at, updated_at`

var _ ports.WorkflowExecutionRepository = (*PostgresStorage)(nil)

func (s *PostgresStorage) GetWorkflowExecutionVersion(
	ctx context.Context,
	orgID, workflowID, versionID uuid.UUID,
) (*domain.WorkflowVersion, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM workflow_versions
		WHERE org_id=$1 AND workflow_id=$2 AND id=$3
	`, workflowVersionColumns), orgID, workflowID, versionID)
	return scanWorkflowVersionResult(row, "get workflow execution version")
}

func (s *PostgresStorage) CreateWorkflowExecution(
	ctx context.Context,
	execution *domain.WorkflowExecution,
	nodes []*domain.WorkflowNodeExecution,
) (*domain.WorkflowExecution, bool, error) {
	if existing, err := s.getWorkflowExecutionByIdempotency(
		ctx, execution.OrgID, execution.IdempotencyKey,
	); err == nil {
		if existing.RequestFingerprint != execution.RequestFingerprint {
			return nil, false, domain.ErrWorkflowExecutionIdempotencyConflict
		}
		return existing, true, nil
	} else if !errors.Is(err, domain.ErrWorkflowExecutionNotFound) {
		return nil, false, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("postgres: create workflow execution begin: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO workflow_executions (
			id, org_id, workflow_id, workflow_version_id, workflow_version, status,
			input, output, error_code, error_message, idempotency_key,
			request_fingerprint, started_at, completed_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`, execution.ID, execution.OrgID, execution.WorkflowID, execution.WorkflowVersionID,
		execution.WorkflowVersion, execution.Status, nullableJSON(execution.Input),
		nullableJSON(execution.Output), nullableString(execution.ErrorCode),
		nullableString(execution.ErrorMessage), execution.IdempotencyKey,
		execution.RequestFingerprint, execution.StartedAt, execution.CompletedAt,
		execution.CreatedAt, execution.UpdatedAt)
	if err != nil {
		if isConstraintViolation(err, "workflow_executions_org_idempotency_unique") {
			_ = tx.Rollback(ctx)
			existing, findErr := s.getWorkflowExecutionByIdempotency(
				ctx, execution.OrgID, execution.IdempotencyKey,
			)
			if findErr != nil {
				return nil, false, findErr
			}
			if existing.RequestFingerprint != execution.RequestFingerprint {
				return nil, false, domain.ErrWorkflowExecutionIdempotencyConflict
			}
			return existing, true, nil
		}
		return nil, false, fmt.Errorf("postgres: create workflow execution: %w", err)
	}

	for _, node := range nodes {
		_, err := tx.Exec(ctx, `
			INSERT INTO workflow_node_executions (
				id, org_id, workflow_execution_id, node_id, node_type, status,
				attempt, input, output, error_code, error_message, queue_task_id,
				webhook_delivery_id, available_at, started_at, completed_at,
				created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		`, node.ID, node.OrgID, node.WorkflowExecutionID, node.NodeID, node.NodeType,
			node.Status, node.Attempt, nullableJSON(node.Input), nullableJSON(node.Output),
			nullableString(node.ErrorCode), nullableString(node.ErrorMessage),
			node.QueueTaskID, node.WebhookDeliveryID, node.AvailableAt, node.StartedAt,
			node.CompletedAt, node.CreatedAt, node.UpdatedAt)
		if err != nil {
			return nil, false, fmt.Errorf("postgres: create workflow node execution: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("postgres: create workflow execution commit: %w", err)
	}
	return execution, false, nil
}

func (s *PostgresStorage) GetWorkflowExecution(
	ctx context.Context,
	orgID, workflowID, executionID uuid.UUID,
) (*domain.WorkflowExecution, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM workflow_executions
		WHERE org_id=$1 AND workflow_id=$2 AND id=$3
	`, workflowExecutionColumns), orgID, workflowID, executionID)
	return scanWorkflowExecutionResult(row, "get workflow execution")
}

func (s *PostgresStorage) getWorkflowExecutionByIdempotency(
	ctx context.Context,
	orgID uuid.UUID,
	idempotencyKey string,
) (*domain.WorkflowExecution, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM workflow_executions
		WHERE org_id=$1 AND idempotency_key=$2
	`, workflowExecutionColumns), orgID, idempotencyKey)
	return scanWorkflowExecutionResult(row, "get workflow execution by idempotency")
}

func (s *PostgresStorage) ListWorkflowExecutions(
	ctx context.Context,
	orgID, workflowID uuid.UUID,
	filter domain.WorkflowExecutionFilter,
) (*domain.WorkflowExecutionPage, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	args := []any{orgID, workflowID}
	var query strings.Builder
	fmt.Fprintf(&query, "SELECT %s FROM workflow_executions WHERE org_id=$1 AND workflow_id=$2", workflowExecutionColumns)
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
		return nil, fmt.Errorf("postgres: list workflow executions: %w", err)
	}
	defer rows.Close()
	page := &domain.WorkflowExecutionPage{Executions: make([]*domain.WorkflowExecution, 0, filter.Limit)}
	for rows.Next() {
		execution, scanErr := scanWorkflowExecution(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: list workflow executions scan: %w", scanErr)
		}
		page.Executions = append(page.Executions, execution)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list workflow executions rows: %w", err)
	}
	if len(page.Executions) > filter.Limit {
		page.Executions = page.Executions[:filter.Limit]
		last := page.Executions[len(page.Executions)-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s *PostgresStorage) GetWorkflowNodeExecutions(
	ctx context.Context,
	orgID, executionID uuid.UUID,
) ([]*domain.WorkflowNodeExecution, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM workflow_node_executions
		WHERE org_id=$1 AND workflow_execution_id=$2
		ORDER BY created_at ASC, node_id ASC
	`, workflowNodeExecutionColumns), orgID, executionID)
	if err != nil {
		return nil, fmt.Errorf("postgres: get workflow node executions: %w", err)
	}
	defer rows.Close()
	nodes := make([]*domain.WorkflowNodeExecution, 0)
	for rows.Next() {
		node, scanErr := scanWorkflowNodeExecution(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: get workflow node executions scan: %w", scanErr)
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: get workflow node executions rows: %w", err)
	}
	return nodes, nil
}

func (s *PostgresStorage) ClaimWorkflowNode(
	ctx context.Context,
	orgID, executionID uuid.UUID,
	nodeID string,
	now time.Time,
) (*domain.WorkflowNodeExecution, bool, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE workflow_node_executions
		SET status='running', attempt=attempt+1, started_at=COALESCE(started_at,$1),
			updated_at=$1
		WHERE org_id=$2 AND workflow_execution_id=$3 AND node_id=$4
		  AND status='pending'
		RETURNING %s
	`, workflowNodeExecutionColumns), now, orgID, executionID, nodeID)
	node, err := scanWorkflowNodeExecution(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("postgres: claim workflow node: %w", err)
	}
	return node, true, nil
}

func (s *PostgresStorage) UpdateWorkflowNodeExecution(
	ctx context.Context,
	node *domain.WorkflowNodeExecution,
	expected domain.WorkflowNodeExecutionStatus,
) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE workflow_node_executions
		SET status=$1, attempt=$2, input=$3, output=$4, error_code=$5,
			error_message=$6, queue_task_id=$7, webhook_delivery_id=$8,
			available_at=$9, started_at=$10, completed_at=$11, updated_at=$12
		WHERE org_id=$13 AND workflow_execution_id=$14 AND id=$15 AND status=$16
	`, node.Status, node.Attempt, nullableJSON(node.Input), nullableJSON(node.Output),
		nullableString(node.ErrorCode), nullableString(node.ErrorMessage),
		node.QueueTaskID, node.WebhookDeliveryID, node.AvailableAt, node.StartedAt,
		node.CompletedAt, node.UpdatedAt, node.OrgID, node.WorkflowExecutionID,
		node.ID, expected)
	if err != nil {
		return false, fmt.Errorf("postgres: update workflow node execution: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PostgresStorage) UpdateWorkflowExecution(
	ctx context.Context,
	execution *domain.WorkflowExecution,
	expected []domain.WorkflowExecutionStatus,
) (bool, error) {
	statuses := make([]string, len(expected))
	for index, status := range expected {
		statuses[index] = string(status)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE workflow_executions
		SET status=$1, output=$2, error_code=$3, error_message=$4,
			started_at=$5, completed_at=$6, updated_at=$7
		WHERE org_id=$8 AND id=$9 AND status=ANY($10)
	`, execution.Status, nullableJSON(execution.Output), nullableString(execution.ErrorCode),
		nullableString(execution.ErrorMessage), execution.StartedAt,
		execution.CompletedAt, execution.UpdatedAt, execution.OrgID, execution.ID,
		statuses)
	if err != nil {
		return false, fmt.Errorf("postgres: update workflow execution: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PostgresStorage) FinalizeWorkflowExecution(
	ctx context.Context,
	execution *domain.WorkflowExecution,
	expected []domain.WorkflowExecutionStatus,
	cancelOpenNodes bool,
) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("postgres: finalize workflow execution begin: %w", err)
	}
	defer tx.Rollback(ctx)
	statuses := make([]string, len(expected))
	for index, status := range expected {
		statuses[index] = string(status)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE workflow_executions
		SET status=$1, output=$2, error_code=$3, error_message=$4,
			started_at=$5, completed_at=$6, updated_at=$7
		WHERE org_id=$8 AND id=$9 AND status=ANY($10)
	`, execution.Status, nullableJSON(execution.Output), nullableString(execution.ErrorCode),
		nullableString(execution.ErrorMessage), execution.StartedAt,
		execution.CompletedAt, execution.UpdatedAt, execution.OrgID, execution.ID,
		statuses)
	if err != nil {
		return false, fmt.Errorf("postgres: finalize workflow execution update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if cancelOpenNodes {
		if _, err := tx.Exec(ctx, `
			UPDATE workflow_node_executions
			SET status='cancelled', error_code=$1, error_message=$2,
				completed_at=$3, updated_at=$3
			WHERE org_id=$4 AND workflow_execution_id=$5
			  AND status IN ('pending','queued','running')
		`, execution.ErrorCode, execution.ErrorMessage, execution.UpdatedAt,
			execution.OrgID, execution.ID); err != nil {
			return false, fmt.Errorf("postgres: finalize workflow nodes: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("postgres: finalize workflow execution commit: %w", err)
	}
	return true, nil
}

func (s *PostgresStorage) CancelWorkflowExecution(
	ctx context.Context,
	orgID, workflowID, executionID uuid.UUID,
	now time.Time,
) (*domain.WorkflowExecution, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: cancel workflow execution begin: %w", err)
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM workflow_executions
		WHERE org_id=$1 AND workflow_id=$2 AND id=$3
		FOR UPDATE
	`, workflowExecutionColumns), orgID, workflowID, executionID)
	execution, err := scanWorkflowExecutionResult(row, "cancel workflow execution select")
	if err != nil {
		return nil, err
	}
	if execution.Status.Terminal() {
		return nil, domain.ErrWorkflowExecutionTerminal
	}
	execution.Status = domain.WorkflowExecutionCancelled
	execution.ErrorCode = "workflow_execution_cancelled"
	execution.ErrorMessage = "workflow execution was cancelled"
	execution.CompletedAt = &now
	execution.UpdatedAt = now
	if _, err := tx.Exec(ctx, `
		UPDATE workflow_executions
		SET status=$1, error_code=$2, error_message=$3, completed_at=$4, updated_at=$4
		WHERE org_id=$5 AND id=$6
	`, execution.Status, execution.ErrorCode, execution.ErrorMessage, now,
		orgID, executionID); err != nil {
		return nil, fmt.Errorf("postgres: cancel workflow execution update: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workflow_node_executions
		SET status='cancelled', error_code='workflow_execution_cancelled',
			error_message='workflow execution was cancelled',
			completed_at=$1, updated_at=$1
		WHERE org_id=$2 AND workflow_execution_id=$3
		  AND status IN ('pending','queued','running')
	`, now, orgID, executionID); err != nil {
		return nil, fmt.Errorf("postgres: cancel workflow nodes: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("postgres: cancel workflow execution commit: %w", err)
	}
	return execution, nil
}

func (s *PostgresStorage) ListWorkflowExecutionsForReconciliation(
	ctx context.Context,
	limit int,
) ([]*domain.WorkflowExecution, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM workflow_executions
		WHERE status IN ('pending','running')
		ORDER BY updated_at ASC, id ASC
		LIMIT $1
	`, workflowExecutionColumns), limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list workflow executions for reconciliation: %w", err)
	}
	defer rows.Close()
	executions := make([]*domain.WorkflowExecution, 0)
	for rows.Next() {
		execution, scanErr := scanWorkflowExecution(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: reconciliation execution scan: %w", scanErr)
		}
		executions = append(executions, execution)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: reconciliation execution rows: %w", err)
	}
	return executions, nil
}

func scanWorkflowExecutionResult(
	row workflowScanner,
	operation string,
) (*domain.WorkflowExecution, error) {
	execution, err := scanWorkflowExecution(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrWorkflowExecutionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: %s: %w", operation, err)
	}
	return execution, nil
}

func scanWorkflowExecution(row workflowScanner) (*domain.WorkflowExecution, error) {
	execution := &domain.WorkflowExecution{}
	var input, output []byte
	var errorCode, errorMessage *string
	if err := row.Scan(
		&execution.ID, &execution.OrgID, &execution.WorkflowID,
		&execution.WorkflowVersionID, &execution.WorkflowVersion, &execution.Status,
		&input, &output, &errorCode, &errorMessage, &execution.IdempotencyKey,
		&execution.RequestFingerprint, &execution.StartedAt, &execution.CompletedAt,
		&execution.CreatedAt, &execution.UpdatedAt,
	); err != nil {
		return nil, err
	}
	execution.Input = cloneRawJSON(input)
	execution.Output = cloneRawJSON(output)
	if errorCode != nil {
		execution.ErrorCode = *errorCode
	}
	if errorMessage != nil {
		execution.ErrorMessage = *errorMessage
	}
	return execution, nil
}

func scanWorkflowNodeExecution(row workflowScanner) (*domain.WorkflowNodeExecution, error) {
	node := &domain.WorkflowNodeExecution{}
	var input, output []byte
	var errorCode, errorMessage *string
	if err := row.Scan(
		&node.ID, &node.OrgID, &node.WorkflowExecutionID, &node.NodeID,
		&node.NodeType, &node.Status, &node.Attempt, &input, &output,
		&errorCode, &errorMessage, &node.QueueTaskID, &node.WebhookDeliveryID,
		&node.AvailableAt, &node.StartedAt, &node.CompletedAt,
		&node.CreatedAt, &node.UpdatedAt,
	); err != nil {
		return nil, err
	}
	node.Input = cloneRawJSON(input)
	node.Output = cloneRawJSON(output)
	if errorCode != nil {
		node.ErrorCode = *errorCode
	}
	if errorMessage != nil {
		node.ErrorMessage = *errorMessage
	}
	return node, nil
}

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func cloneRawJSON(value []byte) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func isConstraintViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		pgErr.ConstraintName == constraint
}
