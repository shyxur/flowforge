package usecase

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/shyxur/windylane/internal/domain"
)

func ValidateWorkflowDefinition(definition domain.WorkflowDefinition) domain.WorkflowValidationResult {
	validationErrors := make([]domain.WorkflowValidationError, 0)
	addError := func(code, message, path string) {
		validationErrors = append(validationErrors, domain.WorkflowValidationError{
			Code: code, Message: message, Path: path,
		})
	}

	if definition.Nodes == nil {
		addError("nodes_required", "definition must include a nodes array", "nodes")
	}
	if definition.Edges == nil {
		addError("edges_required", "definition must include an edges array", "edges")
	}

	nodes := make(map[string]domain.WorkflowNode, len(definition.Nodes))
	nodeIndexes := make(map[string]int, len(definition.Nodes))
	triggerCount := 0
	actionCount := 0
	for index, node := range definition.Nodes {
		path := fmt.Sprintf("nodes[%d]", index)
		if node.ID == "" {
			addError("node_id_required", "node id is required", path+".id")
		} else if _, exists := nodes[node.ID]; exists {
			addError("duplicate_node_id", "node id must be unique", path+".id")
		} else {
			nodes[node.ID] = node
			nodeIndexes[node.ID] = index
		}
		if !node.Type.Valid() {
			addError("unsupported_node_type", "node type is not supported", path+".type")
		}
		if node.Config == nil {
			addError("unsupported_config_shape", "node config must be an object", path+".config")
		}
		if node.Type == domain.WorkflowNodeTrigger {
			triggerCount++
		}
		switch node.Type {
		case domain.WorkflowNodeTask, domain.WorkflowNodeWebhook, domain.WorkflowNodeDelay, domain.WorkflowNodeCondition:
			actionCount++
		}
	}
	if triggerCount == 0 {
		addError("missing_trigger", "workflow must contain at least one trigger node", "nodes")
	}
	if actionCount == 0 {
		addError("missing_action", "workflow must contain at least one executable or action node", "nodes")
	}

	edgeIDs := make(map[string]struct{}, len(definition.Edges))
	edgePairs := make(map[string]struct{}, len(definition.Edges))
	adjacency := make(map[string][]string, len(nodes))
	incoming := make(map[string]int, len(nodes))
	degree := make(map[string]int, len(nodes))
	for index, edge := range definition.Edges {
		path := fmt.Sprintf("edges[%d]", index)
		if edge.ID == "" {
			addError("edge_id_required", "edge id is required", path+".id")
		} else if _, exists := edgeIDs[edge.ID]; exists {
			addError("duplicate_edge_id", "edge id must be unique", path+".id")
		} else {
			edgeIDs[edge.ID] = struct{}{}
		}

		_, fromExists := nodes[edge.From]
		_, toExists := nodes[edge.To]
		if !fromExists {
			addError("invalid_edge_reference", "edge source must reference an existing node", path+".from")
		}
		if !toExists {
			addError("invalid_edge_reference", "edge target must reference an existing node", path+".to")
		}
		if edge.From != "" && edge.From == edge.To {
			addError("self_loop", "self-loop edges are not allowed", path+".to")
		}
		pair := edge.From + "\x00" + edge.To
		if _, exists := edgePairs[pair]; exists {
			addError("duplicate_edge", "duplicate edges between the same nodes are not allowed", path)
		} else {
			edgePairs[pair] = struct{}{}
		}
		if len(edge.Condition) > 0 && !bytes.Equal(bytes.TrimSpace(edge.Condition), []byte("null")) {
			var condition map[string]any
			if err := json.Unmarshal(edge.Condition, &condition); err != nil || condition == nil {
				addError("unsupported_config_shape", "edge condition must be an object or null", path+".condition")
			}
		}
		if fromExists && toExists {
			adjacency[edge.From] = append(adjacency[edge.From], edge.To)
			incoming[edge.To]++
			degree[edge.From]++
			degree[edge.To]++
		}
	}

	for index, node := range definition.Nodes {
		nodeID := node.ID
		if nodeIndexes[nodeID] != index {
			continue
		}
		if node.Type == domain.WorkflowNodeTrigger && incoming[nodeID] > 0 {
			addError("trigger_has_incoming_edge", "trigger nodes cannot have incoming edges", fmt.Sprintf("nodes[%d]", index))
		}
		if degree[nodeID] == 0 {
			addError("orphan_node", "publishable workflows cannot contain isolated nodes", fmt.Sprintf("nodes[%d]", index))
		}
	}

	reachable := make(map[string]bool, len(nodes))
	queue := make([]string, 0, len(nodes))
	for index, node := range definition.Nodes {
		nodeID := node.ID
		if nodeIndexes[nodeID] != index {
			continue
		}
		if node.Type == domain.WorkflowNodeTrigger {
			reachable[nodeID] = true
			queue = append(queue, nodeID)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if !reachable[next] {
				reachable[next] = true
				queue = append(queue, next)
			}
		}
	}
	for index, node := range definition.Nodes {
		nodeID := node.ID
		if nodeIndexes[nodeID] != index {
			continue
		}
		if node.Type != domain.WorkflowNodeTrigger && !reachable[nodeID] {
			addError("unreachable_node", "non-trigger nodes must be reachable from a trigger", fmt.Sprintf("nodes[%d]", index))
		}
	}

	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(nodes))
	hasCycle := false
	var visit func(string)
	visit = func(nodeID string) {
		if hasCycle {
			return
		}
		state[nodeID] = visiting
		for _, next := range adjacency[nodeID] {
			if state[next] == visiting {
				hasCycle = true
				return
			}
			if state[next] == unvisited {
				visit(next)
			}
		}
		state[nodeID] = visited
	}
	for _, node := range definition.Nodes {
		if state[node.ID] == unvisited {
			visit(node.ID)
		}
	}
	if hasCycle {
		addError("cycle_detected", "workflow graph must be acyclic", "edges")
	}

	return domain.WorkflowValidationResult{
		Valid: len(validationErrors) == 0, Errors: validationErrors,
	}
}
