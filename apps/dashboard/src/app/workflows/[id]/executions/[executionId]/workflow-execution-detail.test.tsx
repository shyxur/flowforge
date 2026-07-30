import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WorkflowExecutionDetail } from "./workflow-execution-detail";
import type { WorkflowExecution } from "@/lib/workflow-execution-types";
import type { WorkflowVersionDetail } from "@/lib/workflow-types";

const getExecution = vi.fn();
const cancelExecution = vi.fn();

vi.mock("../actions", () => ({
  getWorkflowExecutionAction: (...args: unknown[]) => getExecution(...args),
  cancelWorkflowExecutionAction: (...args: unknown[]) => cancelExecution(...args),
}));

const version: WorkflowVersionDetail = {
  version: 1,
  version_id: "version-1",
  workflow_id: "workflow-1",
  status: "published",
  published_at: "2026-07-30T10:00:00Z",
  created_at: "2026-07-30T10:00:00Z",
  name: "Historical flow",
  slug: "historical-flow",
  description: null,
  definition: {
    nodes: [
      { id: "task", type: "task", name: "Old task label", config: { queue: "old-queue" } },
      { id: "start", type: "trigger", name: "Old trigger label", config: {} },
    ],
    edges: [{ id: "edge", from: "start", to: "task", condition: null }],
  },
};

function execution(
  status: WorkflowExecution["status"] = "running",
): WorkflowExecution {
  return {
    execution_id: "execution-1",
    org_id: "org-1",
    workflow_id: "workflow-1",
    workflow_version_id: "version-1",
    workflow_version: 1,
    status,
    input: { customer: "one" },
    created_at: "2026-07-30T10:00:00Z",
    started_at: "2026-07-30T10:00:01Z",
    updated_at: "2026-07-30T10:00:01Z",
    nodes: [
      {
        id: "node-task",
        org_id: "org-1",
        workflow_execution_id: "execution-1",
        node_id: "task",
        node_type: "task",
        status: status === "succeeded" ? "succeeded" : "queued",
        attempt: 1,
        queue_task_id: "queue-task-1",
        created_at: "2026-07-30T10:00:00Z",
        updated_at: "2026-07-30T10:00:01Z",
      },
      {
        id: "node-start",
        org_id: "org-1",
        workflow_execution_id: "execution-1",
        node_id: "start",
        node_type: "trigger",
        status: "succeeded",
        attempt: 1,
        created_at: "2026-07-30T10:00:00Z",
        updated_at: "2026-07-30T10:00:00Z",
      },
    ],
  };
}

describe("WorkflowExecutionDetail", () => {
  beforeEach(() => {
    getExecution.mockReset();
    cancelExecution.mockReset();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it("uses immutable version labels and deterministic timeline order", () => {
    render(
      <WorkflowExecutionDetail
        initialExecution={execution("succeeded")}
        version={version}
        workflowName="Current mutable name"
      />,
    );
    const items = screen.getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Old trigger label");
    expect(items[1]).toHaveTextContent("Old task label");
    expect(items[1]).toHaveTextContent("queue old-queue");
  });

  it("does not poll terminal executions", async () => {
    vi.useFakeTimers();
    render(
      <WorkflowExecutionDetail
        initialExecution={execution("succeeded")}
        version={version}
        workflowName="Flow"
      />,
    );
    await act(async () => vi.advanceTimersByTime(6000));
    expect(getExecution).not.toHaveBeenCalled();
    vi.useRealTimers();
  });

  it("polls active execution and stops after terminal refresh", async () => {
    vi.useFakeTimers();
    getExecution.mockResolvedValue({
      ok: true,
      data: { ...execution("succeeded"), completed_at: "2026-07-30T10:00:03Z" },
    });
    render(
      <WorkflowExecutionDetail
        initialExecution={execution("running")}
        version={version}
        workflowName="Flow"
      />,
    );
    await act(async () => vi.advanceTimersByTime(2100));
    expect(getExecution).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTime(6000));
    expect(getExecution).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });

  it("preserves the last successful state after transient polling failure", async () => {
    getExecution.mockResolvedValue({
      ok: false,
      error: "temporary refresh failure",
    });
    render(
      <WorkflowExecutionDetail
        initialExecution={execution("running")}
        version={version}
        workflowName="Flow"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "refresh" }));
    expect(await screen.findByText("temporary refresh failure")).toBeInTheDocument();
    expect(screen.getAllByText("running").length).toBeGreaterThan(0);
  });

  it("confirms cancellation and refreshes node state", async () => {
    cancelExecution.mockResolvedValue({
      ok: true,
      data: { ...execution("cancelled"), nodes: undefined },
    });
    getExecution.mockResolvedValue({
      ok: true,
      data: execution("cancelled"),
    });
    render(
      <WorkflowExecutionDetail
        initialExecution={execution("running")}
        version={version}
        workflowName="Flow"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "cancel execution" }));
    await waitFor(() =>
      expect(cancelExecution).toHaveBeenCalledWith(
        "workflow-1",
        "execution-1",
      ),
    );
    expect(window.confirm).toHaveBeenCalled();
    expect(getExecution).toHaveBeenCalled();
  });

  it("hides cancel action for terminal execution and exposes structured failure", () => {
    const failed = execution("failed");
    failed.error_code = "node_execution_failed";
    failed.error_message = "queue task failed";
    failed.nodes[1].status = "failed";
    failed.nodes[1].error_code = "task_failed";
    failed.nodes[1].error_message = "worker rejected task";
    render(
      <WorkflowExecutionDetail
        initialExecution={failed}
        version={version}
        workflowName="Flow"
      />,
    );
    expect(
      screen.queryByRole("button", { name: "cancel execution" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("node_execution_failed")).toBeInTheDocument();
    expect(screen.getByText("worker rejected task")).toBeInTheDocument();
  });
});
