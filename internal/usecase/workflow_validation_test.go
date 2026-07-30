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
					{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{"queue": "default"}},
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

func TestValidateWorkflowExecutableNodeConfigs(t *testing.T) {
	tests := []struct {
		name string
		node domain.WorkflowNode
		edge json.RawMessage
		code string
	}{
		{
			name: "task queue required",
			node: domain.WorkflowNode{ID: "action", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{}},
			code: "invalid_node_config",
		},
		{
			name: "webhook endpoint UUID required",
			node: domain.WorkflowNode{ID: "action", Type: domain.WorkflowNodeWebhook, Name: "Webhook", Config: map[string]any{"endpoint_id": "bad"}},
			code: "invalid_node_config",
		},
		{
			name: "delay bounded",
			node: domain.WorkflowNode{ID: "action", Type: domain.WorkflowNodeDelay, Name: "Delay", Config: map[string]any{"duration_seconds": maxWorkflowDelaySeconds + 1}},
			code: "invalid_node_config",
		},
		{
			name: "condition operator supported",
			node: domain.WorkflowNode{ID: "action", Type: domain.WorkflowNodeCondition, Name: "Condition", Config: map[string]any{
				"field": "input.status", "operator": "contains", "value": "paid",
			}},
			code: "invalid_node_config",
		},
		{
			name: "condition branch boolean",
			node: domain.WorkflowNode{ID: "action", Type: domain.WorkflowNodeCondition, Name: "Condition", Config: map[string]any{
				"field": "input.status", "operator": "exists",
			}},
			edge: json.RawMessage(`{"branch":"yes"}`),
			code: "invalid_condition_branch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := domain.WorkflowDefinition{
				Nodes: []domain.WorkflowNode{
					{ID: "start", Type: domain.WorkflowNodeTrigger, Name: "Start", Config: map[string]any{}},
					test.node,
				},
				Edges: []domain.WorkflowEdge{{
					ID: "edge", From: "start", To: test.node.ID,
				}},
			}
			if test.edge != nil {
				definition.Nodes = append(definition.Nodes,
					domain.WorkflowNode{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{"queue": "default"}},
				)
				definition.Edges = []domain.WorkflowEdge{
					{ID: "a", From: "start", To: test.node.ID},
					{ID: "b", From: test.node.ID, To: "task", Condition: test.edge},
				}
			}
			result := ValidateWorkflowDefinition(definition)
			for _, validationError := range result.Errors {
				if validationError.Code == test.code {
					return
				}
			}
			t.Fatalf("error code %q not found in %+v", test.code, result.Errors)
		})
	}
}

func TestEvaluateWorkflowConditionOperators(t *testing.T) {
	input := json.RawMessage(`{"status":"paid","customer":{"id":"cus_1"}}`)
	tests := []struct {
		name   string
		config workflowConditionConfig
		want   bool
	}{
		{
			name:   "equals",
			config: workflowConditionConfig{Field: "input.status", Operator: "equals", Value: "paid"},
			want:   true,
		},
		{
			name:   "equals false",
			config: workflowConditionConfig{Field: "input.status", Operator: "equals", Value: "failed"},
			want:   false,
		},
		{
			name:   "not equals",
			config: workflowConditionConfig{Field: "input.status", Operator: "not_equals", Value: "failed"},
			want:   true,
		},
		{
			name:   "exists",
			config: workflowConditionConfig{Field: "input.customer.id", Operator: "exists"},
			want:   true,
		},
		{
			name:   "missing does not exist",
			config: workflowConditionConfig{Field: "input.missing", Operator: "exists"},
			want:   false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := evaluateWorkflowCondition(test.config, input)
			if err != nil || got != test.want {
				t.Fatalf("result=%v want=%v err=%v", got, test.want, err)
			}
		})
	}
}

func TestWorkflowConditionFalseBranchReadiness(t *testing.T) {
	condition := domain.WorkflowNode{
		ID: "condition", Type: domain.WorkflowNodeCondition, Name: "Condition",
	}
	target := domain.WorkflowNode{
		ID: "target", Type: domain.WorkflowNodeTask, Name: "Target",
	}
	states := map[string]*domain.WorkflowNodeExecution{
		"condition": {
			NodeID: "condition", NodeType: domain.WorkflowNodeCondition,
			Status: domain.WorkflowNodeSucceeded,
			Output: json.RawMessage(`{"result":false}`),
		},
	}
	definitions := map[string]domain.WorkflowNode{
		"condition": condition,
		"target":    target,
	}
	runnable, skipped, err := workflowNodeReadiness(
		target,
		[]domain.WorkflowEdge{{
			ID: "false", From: "condition", To: "target",
			Condition: json.RawMessage(`{"branch":false}`),
		}},
		definitions,
		states,
	)
	if err != nil || !runnable || skipped {
		t.Fatalf("false branch runnable=%v skipped=%v err=%v", runnable, skipped, err)
	}
	runnable, skipped, err = workflowNodeReadiness(
		target,
		[]domain.WorkflowEdge{{
			ID: "true", From: "condition", To: "target",
			Condition: json.RawMessage(`{"branch":true}`),
		}},
		definitions,
		states,
	)
	if err != nil || runnable || !skipped {
		t.Fatalf("true branch runnable=%v skipped=%v err=%v", runnable, skipped, err)
	}
}
