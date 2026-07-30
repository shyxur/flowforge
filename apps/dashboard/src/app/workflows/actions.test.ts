import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createWorkflowAction,
  deleteWorkflowAction,
  publishWorkflowAction,
  saveWorkflowAction,
  validateWorkflowAction,
} from "./actions";
import { QueueFlowAPIError } from "@/lib/queueflow";

const api = vi.hoisted(() => ({
  createWorkflow: vi.fn(),
  deleteWorkflow: vi.fn(),
  getWorkflowVersion: vi.fn(),
  publishWorkflow: vi.fn(),
  updateWorkflow: vi.fn(),
  validateWorkflow: vi.fn(),
}));

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/queueflow", () => ({
  ...api,
  QueueFlowAPIError: class QueueFlowAPIError extends Error {
    constructor(
      message: string,
      readonly status: number,
      readonly code?: string,
      readonly details?: unknown,
    ) {
      super(message);
    }
  },
}));

describe("workflow server actions", () => {
  beforeEach(() => {
    for (const mock of Object.values(api)) mock.mockReset();
  });

  it("rejects a blank workflow name before the API call", async () => {
    expect(await createWorkflowAction({ name: " " })).toEqual({
      ok: false,
      error: "workflow name is required",
      status: 400,
    });
    expect(api.createWorkflow).not.toHaveBeenCalled();
  });

  it("creates the safe initial trigger draft", async () => {
    api.createWorkflow.mockResolvedValue({ id: "workflow-1" });
    const result = await createWorkflowAction({
      name: " Order flow ",
      description: " ",
    });
    expect(result.ok).toBe(true);
    expect(api.createWorkflow).toHaveBeenCalledWith({
      name: "Order flow",
      description: null,
      definition: {
        nodes: [
          expect.objectContaining({
            id: "trigger",
            type: "trigger",
            position: { x: 80, y: 120 },
          }),
        ],
        edges: [],
      },
    });
  });

  it("sends an exact definition payload when saving", async () => {
    const definition = {
      nodes: [
        {
          id: "condition",
          type: "condition" as const,
          name: "Route",
          position: { x: 10, y: 20 },
          config: {
            field: "input.ready",
            operator: "exists" as const,
          },
        },
      ],
      edges: [],
    };
    api.updateWorkflow.mockResolvedValue({ id: "workflow-1" });
    await saveWorkflowAction({
      id: "workflow-1",
      name: "Flow",
      description: "",
      definition,
    });
    expect(api.updateWorkflow).toHaveBeenCalledWith("workflow-1", {
      name: "Flow",
      description: null,
      definition,
    });
  });

  it("treats HTTP 200 valid=false as a successful validation result", async () => {
    api.validateWorkflow.mockResolvedValue({
      valid: false,
      errors: [{ code: "missing_action", message: "missing", path: "nodes" }],
    });
    expect(await validateWorkflowAction("workflow-1")).toEqual({
      ok: true,
      data: {
        valid: false,
        errors: [{ code: "missing_action", message: "missing", path: "nodes" }],
      },
    });
  });

  it("maps publish validation details back into structured errors", async () => {
    api.publishWorkflow.mockRejectedValue(
      new QueueFlowAPIError(
        "workflow graph validation failed",
        400,
        "workflow_validation_failed",
        { errors: [{ code: "orphan_node", message: "orphan" }] },
      ),
    );
    expect(await publishWorkflowAction("workflow-1")).toEqual({
      ok: true,
      data: {
        valid: false,
        errors: [{ code: "orphan_node", message: "orphan" }],
      },
    });
  });

  it("deletes through the typed API action", async () => {
    api.deleteWorkflow.mockResolvedValue(undefined);
    expect(await deleteWorkflowAction("workflow-1")).toEqual({
      ok: true,
      data: null,
    });
    expect(api.deleteWorkflow).toHaveBeenCalledWith("workflow-1");
  });
});
