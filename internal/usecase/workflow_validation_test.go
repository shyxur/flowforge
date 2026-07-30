package usecase

import (
	"encoding/json"
	"testing"

	"github.com/shyxur/windylane/internal/domain"
)

func TestValidateWorkflowDefinition(t *testing.T) {
	tests := []struct {
		name string
		def  domain.WorkflowDefinition
		code string
		path string
	}{
		{
			name: "valid graph",
			def: workflowDefinitionForValidation(
				[]domain.WorkflowNode{
					{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
					{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{}},
				},
				[]domain.WorkflowEdge{{ID: "one", From: "start", To: "task", Condition: json.RawMessage("null")}},
			),
		},
		{
			name: "missing trigger",
			def: workflowDefinitionForValidation(
				[]domain.WorkflowNode{
					{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{}},
					{ID: "delay", Type: domain.WorkflowNodeDelay, Name: "Delay", Config: map[string]any{}},
				},
				[]domain.WorkflowEdge{{ID: "one", From: "task", To: "delay"}},
			),
			code: "missing_trigger", path: "nodes",
		},
		{
			name: "unreachable node",
			def: workflowDefinitionForValidation(
				[]domain.WorkflowNode{
					{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
					{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{}},
					{ID: "other", Type: domain.WorkflowNodeDelay, Name: "Other", Config: map[string]any{}},
				},
				[]domain.WorkflowEdge{
					{ID: "one", From: "start", To: "task"},
					{ID: "two", From: "other", To: "task"},
				},
			),
			code: "unreachable_node", path: "nodes[2]",
		},
		{
			name: "cycle",
			def: workflowDefinitionForValidation(
				[]domain.WorkflowNode{
					{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
					{ID: "one", Type: domain.WorkflowNodeTask, Name: "One", Config: map[string]any{}},
					{ID: "two", Type: domain.WorkflowNodeCondition, Name: "Two", Config: map[string]any{}},
				},
				[]domain.WorkflowEdge{
					{ID: "a", From: "start", To: "one"},
					{ID: "b", From: "one", To: "two"},
					{ID: "c", From: "two", To: "one"},
				},
			),
			code: "cycle_detected", path: "edges",
		},
		{
			name: "self loop",
			def: workflowDefinitionForValidation(
				[]domain.WorkflowNode{
					{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
					{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{}},
				},
				[]domain.WorkflowEdge{
					{ID: "a", From: "start", To: "task"},
					{ID: "b", From: "task", To: "task"},
				},
			),
			code: "self_loop", path: "edges[1].to",
		},
		{
			name: "duplicate edge",
			def: workflowDefinitionForValidation(
				[]domain.WorkflowNode{
					{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
					{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{}},
				},
				[]domain.WorkflowEdge{
					{ID: "a", From: "start", To: "task"},
					{ID: "b", From: "start", To: "task"},
				},
			),
			code: "duplicate_edge", path: "edges[1]",
		},
		{
			name: "trigger incoming",
			def: workflowDefinitionForValidation(
				[]domain.WorkflowNode{
					{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
					{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{}},
				},
				[]domain.WorkflowEdge{
					{ID: "a", From: "start", To: "task"},
					{ID: "b", From: "task", To: "start"},
				},
			),
			code: "trigger_has_incoming_edge", path: "nodes[0]",
		},
		{
			name: "invalid edge reference",
			def: workflowDefinitionForValidation(
				[]domain.WorkflowNode{
					{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
					{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{}},
				},
				[]domain.WorkflowEdge{
					{ID: "a", From: "start", To: "task"},
					{ID: "b", From: "task", To: "missing"},
				},
			),
			code: "invalid_edge_reference", path: "edges[1].to",
		},
		{
			name: "no action",
			def: workflowDefinitionForValidation(
				[]domain.WorkflowNode{
					{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
					{ID: "other", Type: domain.WorkflowNodeTrigger, Name: "Other", Config: map[string]any{}},
				},
				[]domain.WorkflowEdge{{ID: "a", From: "start", To: "other"}},
			),
			code: "missing_action", path: "nodes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ValidateWorkflowDefinition(test.def)
			if test.code == "" {
				if !result.Valid || len(result.Errors) != 0 {
					t.Fatalf("valid graph result = %+v", result)
				}
				return
			}
			if result.Valid {
				t.Fatalf("invalid graph reported valid")
			}
			for _, validationError := range result.Errors {
				if validationError.Code == test.code {
					if validationError.Message == "" || validationError.Path != test.path {
						t.Fatalf("error = %+v, want path %q and message", validationError, test.path)
					}
					return
				}
			}
			t.Fatalf("error code %q not found in %+v", test.code, result.Errors)
		})
	}
}

func workflowDefinitionForValidation(
	nodes []domain.WorkflowNode,
	edges []domain.WorkflowEdge,
) domain.WorkflowDefinition {
	return domain.WorkflowDefinition{Nodes: nodes, Edges: edges}
}
