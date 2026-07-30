import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  cancelWorkflowExecutionAction,
  getWorkflowExecutionAction,
  startWorkflowExecutionAction,
} from "./actions";
import { QueueFlowAPIError } from "@/lib/queueflow";

const api = vi.hoisted(() => ({
  startWorkflowExecution: vi.fn(),
  getWorkflowExecution: vi.fn(),
  cancelWorkflowExecution: vi.fn(),
}));

vi.mock("@/lib/queueflow", () => ({
  ...api,
  QueueFlowAPIError: class QueueFlowAPIError extends Error {
    constructor(
      message: string,
      readonly status: number,
      readonly code?: string,
    ) {
      super(message);
    }
  },
}));

describe("workflow execution actions", () => {
  beforeEach(() => {
    for (const mock of Object.values(api)) mock.mockReset();
  });

  it("forwards version, input, and per-run idempotency key", async () => {
    api.startWorkflowExecution.mockResolvedValue({ execution_id: "execution-1" });
    await startWorkflowExecutionAction({
      workflowID: "workflow-1",
      idempotencyKey: "dashboard-key",
      request: { version: 2, input: { ready: true } },
    });
    expect(api.startWorkflowExecution).toHaveBeenCalledWith(
      "workflow-1",
      { version: 2, input: { ready: true } },
      "dashboard-key",
    );
  });

  it("maps payload and rate limit errors to safe messages", async () => {
    api.startWorkflowExecution.mockRejectedValueOnce(
      new QueueFlowAPIError("large", 413),
    );
    expect(
      await startWorkflowExecutionAction({
        workflowID: "workflow-1",
        idempotencyKey: "key",
        request: {},
      }),
    ).toEqual({
      ok: false,
      error: "execution input exceeds the 256 KiB limit",
      status: 413,
    });
    api.startWorkflowExecution.mockRejectedValueOnce(
      new QueueFlowAPIError("limited", 429),
    );
    expect(
      await startWorkflowExecutionAction({
        workflowID: "workflow-1",
        idempotencyKey: "key-2",
        request: {},
      }),
    ).toEqual({
      ok: false,
      error: "rate limit reached; wait briefly and retry",
      status: 429,
    });
  });

  it("loads detail and cancels through typed APIs", async () => {
    api.getWorkflowExecution.mockResolvedValue({ execution_id: "execution-1" });
    api.cancelWorkflowExecution.mockResolvedValue({
      execution_id: "execution-1",
      status: "cancelled",
    });
    expect(
      await getWorkflowExecutionAction("workflow-1", "execution-1"),
    ).toEqual({
      ok: true,
      data: { execution_id: "execution-1" },
    });
    expect(
      await cancelWorkflowExecutionAction("workflow-1", "execution-1"),
    ).toEqual({
      ok: true,
      data: { execution_id: "execution-1", status: "cancelled" },
    });
  });

  it("preserves deterministic terminal conflict errors", async () => {
    api.cancelWorkflowExecution.mockRejectedValue(
      new QueueFlowAPIError(
        "terminal workflow executions cannot be cancelled",
        409,
        "workflow_execution_terminal",
      ),
    );
    expect(
      await cancelWorkflowExecutionAction("workflow-1", "execution-1"),
    ).toEqual({
      ok: false,
      error: "terminal workflow executions cannot be cancelled",
      status: 409,
    });
  });
});
