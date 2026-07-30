import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import WorkflowExecutionsLoading from "./loading";
import WorkflowExecutionsPage from "./page";

const api = vi.hoisted(() => ({
  getWorkflow: vi.fn(),
  listWorkflowVersions: vi.fn(),
  listWorkflowExecutions: vi.fn(),
}));

vi.mock("@/lib/queueflow", () => ({
  ...api,
  QueueFlowAPIError: class QueueFlowAPIError extends Error {
    constructor(
      message: string,
      readonly status: number,
    ) {
      super(message);
    }
  },
}));
vi.mock("./run-workflow-dialog", () => ({
  RunWorkflowDialog: ({ workflowStatus }: { workflowStatus: string }) => (
    <button disabled={workflowStatus !== "published"}>run workflow</button>
  ),
}));

const workflow = {
  id: "workflow-1",
  name: "Order flow",
  status: "published",
};

describe("WorkflowExecutionsPage", () => {
  beforeEach(() => {
    api.getWorkflow.mockReset();
    api.listWorkflowVersions.mockReset();
    api.listWorkflowExecutions.mockReset();
    api.getWorkflow.mockResolvedValue(workflow);
    api.listWorkflowVersions.mockResolvedValue({
      items: [{ version: 1, version_id: "version-1" }],
    });
  });

  it("renders the loading state", () => {
    render(<WorkflowExecutionsLoading />);
    expect(screen.getByLabelText("loading workflow executions")).toHaveAttribute(
      "aria-busy",
      "true",
    );
  });

  it("renders an empty state with run action", async () => {
    api.listWorkflowExecutions.mockResolvedValue({ items: [] });
    render(
      await WorkflowExecutionsPage({
        params: Promise.resolve({ id: "workflow-1" }),
        searchParams: Promise.resolve({}),
      }),
    );
    expect(screen.getByText("no workflow executions")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "run workflow" })).toBeEnabled();
  });

  it("renders execution status, deterministic timestamps, and detail link", async () => {
    api.listWorkflowExecutions.mockResolvedValue({
      items: [
        {
          execution_id: "12345678-1234-4234-8234-123456789abc",
          workflow_version: 2,
          status: "failed",
          created_at: "2026-07-30T10:00:00Z",
          started_at: "2026-07-30T10:00:01Z",
          completed_at: "2026-07-30T10:00:03Z",
          error_message: "task failed",
        },
      ],
    });
    render(
      await WorkflowExecutionsPage({
        params: Promise.resolve({ id: "workflow-1" }),
        searchParams: Promise.resolve({ status: "failed" }),
      }),
    );
    expect(screen.getAllByText("failed")).toHaveLength(2);
    expect(screen.getByText("2026-07-30 10:00:00 UTC")).toBeInTheDocument();
    expect(screen.getByText("2 s")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /12345678/ })).toHaveAttribute(
      "href",
      "/workflows/workflow-1/executions/12345678-1234-4234-8234-123456789abc",
    );
  });

  it("preserves status filter in cursor pagination", async () => {
    api.listWorkflowExecutions.mockResolvedValue({
      items: [
        {
          execution_id: "12345678-1234-4234-8234-123456789abc",
          workflow_version: 1,
          status: "running",
          created_at: "2026-07-30T10:00:00Z",
        },
      ],
      next_cursor: "opaque cursor",
    });
    render(
      await WorkflowExecutionsPage({
        params: Promise.resolve({ id: "workflow-1" }),
        searchParams: Promise.resolve({ status: "running" }),
      }),
    );
    expect(screen.getByRole("link", { name: "next page" })).toHaveAttribute(
      "href",
      "/workflows/workflow-1/executions?cursor=opaque+cursor&status=running",
    );
  });

  it("renders retryable API failure without losing workflow context", async () => {
    api.listWorkflowExecutions.mockRejectedValue(new Error("offline"));
    render(
      await WorkflowExecutionsPage({
        params: Promise.resolve({ id: "workflow-1" }),
        searchParams: Promise.resolve({}),
      }),
    );
    expect(screen.getByText("executions unavailable")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "retry" })).toHaveAttribute(
      "href",
      "/workflows/workflow-1/executions",
    );
  });
});
