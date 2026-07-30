package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/usecase"
	"go.uber.org/zap"
)

type workflowExecutionServiceStub struct {
	orgID       uuid.UUID
	workflowID  uuid.UUID
	executionID uuid.UUID
	cancelled   bool
}

func (stub *workflowExecutionServiceStub) StartExecution(
	_ context.Context,
	input usecase.StartWorkflowExecutionInput,
) (*domain.WorkflowExecutionStartResult, bool, error) {
	if input.OrgID != stub.orgID || input.WorkflowID != stub.workflowID {
		return nil, false, domain.ErrWorkflowNotFound
	}
	return &domain.WorkflowExecutionStartResult{
		ExecutionID:     stub.executionID,
		WorkflowID:      stub.workflowID,
		WorkflowVersion: 2,
		Status:          domain.WorkflowExecutionRunning,
		CreatedAt:       time.Unix(100, 0).UTC(),
	}, false, nil
}

func (stub *workflowExecutionServiceStub) ListExecutions(
	_ context.Context,
	orgID, workflowID uuid.UUID,
	_ domain.WorkflowExecutionFilter,
) (*domain.WorkflowExecutionPage, error) {
	if orgID != stub.orgID || workflowID != stub.workflowID {
		return nil, domain.ErrWorkflowNotFound
	}
	return &domain.WorkflowExecutionPage{Executions: []*domain.WorkflowExecution{{
		ID: stub.executionID, OrgID: orgID, WorkflowID: workflowID,
		WorkflowVersion: 2, Status: domain.WorkflowExecutionRunning,
	}}}, nil
}

func (stub *workflowExecutionServiceStub) GetExecution(
	_ context.Context,
	orgID, workflowID, executionID uuid.UUID,
) (*domain.WorkflowExecutionDetail, error) {
	if orgID != stub.orgID || workflowID != stub.workflowID || executionID != stub.executionID {
		return nil, domain.ErrWorkflowExecutionNotFound
	}
	return &domain.WorkflowExecutionDetail{
		WorkflowExecution: &domain.WorkflowExecution{
			ID: executionID, OrgID: orgID, WorkflowID: workflowID,
			WorkflowVersion: 2, Status: domain.WorkflowExecutionRunning,
		},
		Nodes: []*domain.WorkflowNodeExecution{{
			NodeID: "start", NodeType: domain.WorkflowNodeTrigger,
			Status: domain.WorkflowNodeSucceeded,
		}},
	}, nil
}

func (stub *workflowExecutionServiceStub) CancelExecution(
	_ context.Context,
	orgID, workflowID, executionID uuid.UUID,
) (*domain.WorkflowExecution, error) {
	if orgID != stub.orgID || workflowID != stub.workflowID || executionID != stub.executionID {
		return nil, domain.ErrWorkflowExecutionNotFound
	}
	if stub.cancelled {
		return nil, domain.ErrWorkflowExecutionTerminal
	}
	stub.cancelled = true
	return &domain.WorkflowExecution{
		ID: executionID, OrgID: orgID, WorkflowID: workflowID,
		Status: domain.WorkflowExecutionCancelled,
	}, nil
}

func TestWorkflowExecutionAPI(t *testing.T) {
	orgID, workflowID, executionID := uuid.New(), uuid.New(), uuid.New()
	service := &workflowExecutionServiceStub{
		orgID: orgID, workflowID: workflowID, executionID: executionID,
	}
	router := testWorkflowExecutionRouter(service, orgID)
	base := "/v1/workflows/" + workflowID.String() + "/executions"

	missingKey := performWorkflowRequest(router, http.MethodPost, base, `{}`)
	if missingKey.Code != http.StatusBadRequest {
		t.Fatalf("missing key = %d %s", missingKey.Code, missingKey.Body.String())
	}
	start := performWorkflowExecutionRequest(
		router, http.MethodPost, base, `{"version":2,"input":{"status":"paid"}}`, "exec-one",
	)
	if start.Code != http.StatusAccepted {
		t.Fatalf("start = %d %s", start.Code, start.Body.String())
	}
	var startResult domain.WorkflowExecutionStartResult
	decodeWorkflowResponse(t, start, &startResult)
	if startResult.ExecutionID != executionID || startResult.WorkflowVersion != 2 {
		t.Fatalf("start result = %+v", startResult)
	}
	list := performWorkflowRequest(router, http.MethodGet, base+"?status=running", "")
	var page domain.WorkflowExecutionPage
	decodeWorkflowResponse(t, list, &page)
	if list.Code != http.StatusOK || len(page.Executions) != 1 {
		t.Fatalf("list = %d %+v", list.Code, page)
	}
	detailPath := base + "/" + executionID.String()
	detail := performWorkflowRequest(router, http.MethodGet, detailPath, "")
	var executionDetail domain.WorkflowExecutionDetail
	decodeWorkflowResponse(t, detail, &executionDetail)
	if detail.Code != http.StatusOK || len(executionDetail.Nodes) != 1 {
		t.Fatalf("detail = %d %+v", detail.Code, executionDetail)
	}
	cancel := performWorkflowRequest(router, http.MethodPost, detailPath+"/cancel", "")
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel = %d %s", cancel.Code, cancel.Body.String())
	}
	terminalCancel := performWorkflowRequest(router, http.MethodPost, detailPath+"/cancel", "")
	if terminalCancel.Code != http.StatusConflict {
		t.Fatalf("terminal cancel = %d %s", terminalCancel.Code, terminalCancel.Body.String())
	}
}

func TestWorkflowExecutionAPIValidationAndAuth(t *testing.T) {
	orgID, workflowID := uuid.New(), uuid.New()
	service := &workflowExecutionServiceStub{
		orgID: orgID, workflowID: workflowID, executionID: uuid.New(),
	}
	router := testWorkflowExecutionRouter(service, orgID)
	base := "/v1/workflows/" + workflowID.String() + "/executions"

	if response := performWorkflowRequest(router, http.MethodGet, base+"?status=unknown", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d %s", response.Code, response.Body.String())
	}
	if response := performWorkflowRequest(router, http.MethodGet, base+"/not-a-uuid", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid execution id = %d %s", response.Code, response.Body.String())
	}
	oversized := `{"input":"` + strings.Repeat("x", maxPayloadBytes+1) + `"}`
	if response := performWorkflowExecutionRequest(
		router, http.MethodPost, base, oversized, "large",
	); response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized input = %d %s", response.Code, response.Body.String())
	}
	request, _ := http.NewRequest(http.MethodGet, base, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d %s", response.Code, response.Body.String())
	}
}

func testWorkflowExecutionRouter(
	service WorkflowExecutionService,
	orgID uuid.UUID,
) http.Handler {
	handler := NewHandler(&serviceStub{}, zap.NewNop()).
		WithWorkflowExecutionService(service)
	return NewRouter(handler, allowAuth(orgID), nil, zap.NewNop())
}

func performWorkflowExecutionRequest(
	router http.Handler,
	method, path, body, idempotencyKey string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer valid")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
