import { describe, expect, it, vi } from "vitest";
import {
  canConnect,
  createWorkflowNode,
  definitionToEditor,
  editorToDefinition,
  fallbackPosition,
  validateDraft,
} from "./workflow-editor-model";
import type { WorkflowDefinition } from "./workflow-types";

const definition: WorkflowDefinition = {
  nodes: [
    { id: "start", type: "trigger", name: "Start", config: {} },
    {
      id: "route",
      type: "condition",
      name: "Route",
      position: { x: 410, y: 170 },
      config: { field: "input.ready", operator: "exists" },
    },
    {
      id: "task",
      type: "task",
      name: "Run",
      config: { queue: "primary" },
    },
  ],
  edges: [
    { id: "a", from: "start", to: "route", condition: null },
    {
      id: "b",
      from: "route",
      to: "task",
      condition: { branch: true },
    },
  ],
};

describe("workflow editor model", () => {
  it("maps legacy nodes to deterministic fallback positions", () => {
    const first = definitionToEditor(definition);
    const second = definitionToEditor(definition);
    expect(first.nodes[0].position).toEqual(fallbackPosition(0));
    expect(first.nodes[2].position).toEqual(fallbackPosition(2));
    expect(second.nodes.map((node) => node.position)).toEqual(
      first.nodes.map((node) => node.position),
    );
    expect(first.nodes[1].position).toEqual({ x: 410, y: 170 });
  });

  it("serializes positions and condition branches exactly", () => {
    const editor = definitionToEditor(definition);
    editor.nodes[0].position = { x: 123.4, y: 88.7 };
    const serialized = editorToDefinition(editor.nodes, editor.edges);
    expect(serialized.nodes[0].position).toEqual({ x: 123, y: 89 });
    expect(serialized.edges[1].condition).toEqual({ branch: true });
  });

  it("rejects self-loops, duplicates, and incoming trigger edges", () => {
    const editor = definitionToEditor(definition);
    expect(canConnect("task", "task", editor.nodes, editor.edges)).toMatch(
      /self-loop/,
    );
    expect(canConnect("start", "route", editor.nodes, editor.edges)).toMatch(
      /already exists/,
    );
    expect(canConnect("task", "start", editor.nodes, editor.edges)).toMatch(
      /trigger/,
    );
  });

  it("creates every supported node with collision-resistant defaults", () => {
    vi.spyOn(crypto, "randomUUID")
      .mockReturnValueOnce("11111111-1111-4111-8111-111111111111")
      .mockReturnValueOnce("22222222-2222-4222-8222-222222222222")
      .mockReturnValue("33333333-3333-4333-8333-333333333333");
    const kinds = ["trigger", "task", "webhook", "delay", "condition"] as const;
    const nodes = kinds.map((type) =>
      createWorkflowNode(type, { x: 10, y: 20 }, new Set()),
    );
    expect(nodes.map((node) => node.data.workflowNode.type)).toEqual(kinds);
    expect(new Set(nodes.map((node) => node.id)).size).toBe(nodes.length);
  });

  it("checks required node configuration without replacing backend validation", () => {
    const invalid: WorkflowDefinition = {
      nodes: [
        { id: "task", type: "task", name: "Task", config: { queue: "" } },
        {
          id: "hook",
          type: "webhook",
          name: "Hook",
          config: { endpoint_id: "" },
        },
        {
          id: "delay",
          type: "delay",
          name: "Delay",
          config: { duration_seconds: 604801 },
        },
        {
          id: "condition",
          type: "condition",
          name: "Condition",
          config: { field: "status", operator: "equals" },
        },
      ],
      edges: [],
    };
    expect(validateDraft(invalid)).toMatchObject({
      task: "task queue is required",
      hook: "webhook endpoint is required",
      delay: "delay must be between 0 and 604800 seconds",
      condition: "condition field must begin with input.",
    });
  });

  it("does not require a condition value for exists", () => {
    expect(
      validateDraft({
        nodes: [
          {
            id: "condition",
            type: "condition",
            name: "Condition",
            config: { field: "input.status", operator: "exists" },
          },
        ],
        edges: [],
      }),
    ).toEqual({});
  });
});
