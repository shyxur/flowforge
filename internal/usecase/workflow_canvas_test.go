package usecase

import (
	"encoding/json"
	"testing"
)

func TestParseWorkflowDefinitionAcceptsOptionalCanvasPosition(t *testing.T) {
	definition, err := parseWorkflowDefinition(json.RawMessage(`{
		"nodes": [
			{
				"id": "start",
				"type": "trigger",
				"name": "Start",
				"position": {"x": 120.5, "y": -40},
				"config": {}
			}
		],
		"edges": []
	}`))
	if err != nil {
		t.Fatalf("parse definition: %v", err)
	}
	if definition.Nodes[0].Position == nil {
		t.Fatal("expected canvas position")
	}
	if definition.Nodes[0].Position.X != 120.5 || definition.Nodes[0].Position.Y != -40 {
		t.Fatalf("unexpected canvas position: %#v", definition.Nodes[0].Position)
	}
}

func TestParseWorkflowDefinitionKeepsLegacyNodesWithoutPosition(t *testing.T) {
	definition, err := parseWorkflowDefinition(json.RawMessage(`{
		"nodes": [{"id": "start", "type": "trigger", "name": "Start", "config": {}}],
		"edges": []
	}`))
	if err != nil {
		t.Fatalf("parse definition: %v", err)
	}
	if definition.Nodes[0].Position != nil {
		t.Fatalf("legacy node unexpectedly gained a position: %#v", definition.Nodes[0].Position)
	}
}
