import { describe, expect, it } from "vitest";
import {
  durationMilliseconds,
  executionNodeSummary,
  formatDuration,
  formatTimestamp,
  isExecutionTerminal,
  orderNodeExecutions,
  topologicalNodeIDs,
} from "./workflow-execution-model";
import type { WorkflowNodeExecution } from "./workflow-execution-types";
import type { WorkflowDefinition } from "./workflow-types";

const definition: WorkflowDefinition = {
  nodes: [
    { id: "finish", type: "webhook", name: "Finish", config: { endpoint_id: "e" } },
    { id: "false", type: "delay", name: "Wait", config: { duration_seconds: 30 } },
    { id: "route", type: "condition", name: "Route", config: { field: "input.ready", operator: "exists" } },
    { id: "start", type: "trigger", name: "Start", config: {} },
    { id: "true", type: "task", name: "Run", config: { queue: "default" } },
  ],
  edges: [
    { id: "a", from: "start", to: "route", condition: null },
    { id: "b", from: "route", to: "true", condition: { branch: true } },
    { id: "c", from: "route", to: "false", condition: { branch: false } },
    { id: "d", from: "true", to: "finish", condition: null },
    { id: "e", from: "false", to: "finish", condition: null },
  ],
};

function node(
  nodeID: string,
  status: WorkflowNodeExecution["status"] = "pending",
): WorkflowNodeExecution {
  return {
    id: `row-${nodeID}`,
    org_id: "org",
    workflow_execution_id: "execution",
    node_id: nodeID,
    node_type:
      definition.nodes.find((item) => item.id === nodeID)?.type ?? "task",
    status,
    attempt: 1,
    created_at: "2026-07-30T10:00:00Z",
    updated_at: "2026-07-30T10:00:00Z",
  };
}

describe("workflow execution model", () => {
  it("derives deterministic topological order from the immutable definition", () => {
    expect(topologicalNodeIDs(definition)).toEqual([
      "start",
      "route",
      "false",
      "true",
      "finish",
    ]);
  });

  it("orders execution nodes by graph rank rather than API order", () => {
    expect(
      orderNodeExecutions(
        [node("finish"), node("true"), node("start"), node("route"), node("false")],
        definition,
      ).map((item) => item.node_id),
    ).toEqual(["start", "route", "false", "true", "finish"]);
  });

  it("uses stable ID fallback for unknown or cyclic nodes", () => {
    const cyclic: WorkflowDefinition = {
      nodes: [
        { id: "b", type: "task", name: "B", config: { queue: "b" } },
        { id: "a", type: "task", name: "A", config: { queue: "a" } },
      ],
      edges: [
        { id: "ab", from: "a", to: "b", condition: null },
        { id: "ba", from: "b", to: "a", condition: null },
      ],
    };
    expect(topologicalNodeIDs(cyclic)).toEqual(["a", "b"]);
  });

  it("formats timestamps in deterministic UTC without locale output", () => {
    expect(formatTimestamp("2026-07-30T10:11:12.123Z")).toBe(
      "2026-07-30 10:11:12 UTC",
    );
    expect(formatTimestamp()).toBe("—");
  });

  it("derives valid durations and rejects inverted timestamps", () => {
    expect(
      durationMilliseconds(
        "2026-07-30T10:00:00Z",
        "2026-07-30T10:01:05Z",
      ),
    ).toBe(65000);
    expect(formatDuration(65000)).toBe("1m 5s");
    expect(
      durationMilliseconds(
        "2026-07-30T10:01:00Z",
        "2026-07-30T10:00:00Z",
      ),
    ).toBeUndefined();
  });

  it("recognizes only execution terminal states", () => {
    expect(isExecutionTerminal("pending")).toBe(false);
    expect(isExecutionTerminal("running")).toBe(false);
    expect(isExecutionTerminal("succeeded")).toBe(true);
    expect(isExecutionTerminal("failed")).toBe(true);
    expect(isExecutionTerminal("cancelled")).toBe(true);
  });

  it("renders type-specific summaries only from exposed data", () => {
    const task = node("true", "queued");
    task.queue_task_id = "task-id";
    expect(
      executionNodeSummary(
        task,
        definition.nodes.find((item) => item.id === "true"),
      ),
    ).toBe("queue default · task task-id · attempt 1");
    expect(
      executionNodeSummary(
        node("route", "succeeded"),
        definition.nodes.find((item) => item.id === "route"),
      ),
    ).toBe("input.ready exists");
  });
});
