"use server";

import {
  cancelWorkflowExecution,
  getWorkflowExecution,
  QueueFlowAPIError,
  startWorkflowExecution,
} from "@/lib/queueflow";
import type {
  StartWorkflowExecutionRequest,
  StartWorkflowExecutionResponse,
  WorkflowExecution,
  WorkflowExecutionSummary,
} from "@/lib/workflow-execution-types";
import type { WorkflowActionResult } from "../../actions";

export async function startWorkflowExecutionAction(input: {
  workflowID: string;
  request: StartWorkflowExecutionRequest;
  idempotencyKey: string;
}): Promise<WorkflowActionResult<StartWorkflowExecutionResponse>> {
  try {
    return {
      ok: true,
      data: await startWorkflowExecution(
        input.workflowID,
        input.request,
        input.idempotencyKey,
      ),
    };
  } catch (error) {
    return executionError(error, "unable to start workflow execution");
  }
}

export async function getWorkflowExecutionAction(
  workflowID: string,
  executionID: string,
): Promise<WorkflowActionResult<WorkflowExecution>> {
  try {
    return {
      ok: true,
      data: await getWorkflowExecution(workflowID, executionID),
    };
  } catch (error) {
    return executionError(error, "unable to refresh workflow execution");
  }
}

export async function cancelWorkflowExecutionAction(
  workflowID: string,
  executionID: string,
): Promise<WorkflowActionResult<WorkflowExecutionSummary>> {
  try {
    return {
      ok: true,
      data: await cancelWorkflowExecution(workflowID, executionID),
    };
  } catch (error) {
    return executionError(error, "unable to cancel workflow execution");
  }
}

function executionError<T>(
  error: unknown,
  fallback: string,
): WorkflowActionResult<T> {
  if (!(error instanceof QueueFlowAPIError)) {
    return { ok: false, error: fallback };
  }
  const messages: Record<number, string> = {
    413: "execution input exceeds the 256 KiB limit",
    429: "rate limit reached; wait briefly and retry",
  };
  return {
    ok: false,
    error: messages[error.status] ?? error.message,
    status: error.status,
  };
}
