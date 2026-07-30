"use server";

import { revalidatePath } from "next/cache";
import {
  createWorkflow,
  deleteWorkflow,
  getWorkflowVersion,
  publishWorkflow,
  QueueFlowAPIError,
  updateWorkflow,
  validateWorkflow,
} from "@/lib/queueflow";
import type {
  Workflow,
  WorkflowDefinition,
  WorkflowPublishResult,
  WorkflowValidationResult,
  WorkflowVersionDetail,
} from "@/lib/workflow-types";

export type WorkflowActionResult<T> =
  | { ok: true; data: T }
  | { ok: false; error: string; status?: number };

export async function createWorkflowAction(input: {
  name: string;
  description?: string;
}): Promise<WorkflowActionResult<Workflow>> {
  if (!input.name.trim()) return failure("workflow name is required", 400);
  try {
    const workflow = await createWorkflow({
      name: input.name.trim(),
      description: input.description?.trim() || null,
      definition: {
        nodes: [
          {
            id: "trigger",
            type: "trigger",
            name: "Workflow trigger",
            position: { x: 80, y: 120 },
            config: {},
          },
        ],
        edges: [],
      },
    });
    revalidatePath("/workflows");
    return { ok: true, data: workflow };
  } catch (error) {
    return fromError(error, "unable to create workflow");
  }
}

export async function saveWorkflowAction(input: {
  id: string;
  name: string;
  description: string;
  definition: WorkflowDefinition;
}): Promise<WorkflowActionResult<Workflow>> {
  try {
    const workflow = await updateWorkflow(input.id, {
      name: input.name.trim(),
      description: input.description.trim() || null,
      definition: input.definition,
    });
    revalidatePath("/workflows");
    revalidatePath(`/workflows/${input.id}`);
    return { ok: true, data: workflow };
  } catch (error) {
    return fromError(error, "unable to save workflow");
  }
}

export async function validateWorkflowAction(
  id: string,
): Promise<WorkflowActionResult<WorkflowValidationResult>> {
  try {
    return { ok: true, data: await validateWorkflow(id) };
  } catch (error) {
    return fromError(error, "unable to validate workflow");
  }
}

export async function publishWorkflowAction(
  id: string,
): Promise<WorkflowActionResult<WorkflowPublishResult | WorkflowValidationResult>> {
  try {
    const published = await publishWorkflow(id);
    revalidatePath("/workflows");
    revalidatePath(`/workflows/${id}`);
    return { ok: true, data: published };
  } catch (error) {
    if (
      error instanceof QueueFlowAPIError &&
      error.code === "workflow_validation_failed"
    ) {
      const details = error.details as
        | { errors?: WorkflowValidationResult["errors"] }
        | undefined;
      return {
        ok: true,
        data: { valid: false, errors: details?.errors ?? [] },
      };
    }
    return fromError(error, "unable to publish workflow");
  }
}

export async function getWorkflowVersionAction(
  id: string,
  version: number,
): Promise<WorkflowActionResult<WorkflowVersionDetail>> {
  try {
    return { ok: true, data: await getWorkflowVersion(id, version) };
  } catch (error) {
    return fromError(error, "unable to load workflow version");
  }
}

export async function deleteWorkflowAction(
  id: string,
): Promise<WorkflowActionResult<null>> {
  try {
    await deleteWorkflow(id);
    revalidatePath("/workflows");
    return { ok: true, data: null };
  } catch (error) {
    return fromError(error, "unable to delete workflow");
  }
}

function fromError<T>(
  error: unknown,
  fallback: string,
): WorkflowActionResult<T> {
  if (error instanceof QueueFlowAPIError) {
    return failure(friendlyMessage(error), error.status);
  }
  return failure(fallback);
}

function failure(error: string, status?: number) {
  return { ok: false as const, error, status };
}

function friendlyMessage(error: QueueFlowAPIError) {
  if (error.status === 413) return "workflow payload is too large";
  if (error.status === 429) return "rate limit reached; wait briefly and retry";
  if (error.status >= 500) return "windylane is unavailable; retry in a moment";
  return error.message;
}
