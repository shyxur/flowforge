import type { Metadata } from "next";
import Link from "next/link";
import {
  getWorkflowExecution,
  getWorkflowVersion,
  QueueFlowAPIError,
} from "@/lib/queueflow";
import { WorkflowExecutionDetail } from "./workflow-execution-detail";

export const metadata: Metadata = { title: "workflow execution detail" };
export const dynamic = "force-dynamic";

export default async function WorkflowExecutionDetailPage({
  params,
}: {
  params: Promise<{ id: string; executionId: string }>;
}) {
  const { id, executionId } = await params;
  let execution;
  let version;
  try {
    execution = await getWorkflowExecution(id, executionId);
    version = await getWorkflowVersion(id, execution.workflow_version);
  } catch (caught) {
    const message =
      caught instanceof QueueFlowAPIError
        ? caught.status === 404
          ? "execution not found"
          : caught.message
        : "unable to load workflow execution";
    return (
      <div>
        <Link className="back-link" href={`/workflows/${id}/executions`}>
          ← back to executions
        </Link>
        <section className="panel api-error">
          <strong>execution unavailable</strong>
          <p>{message}</p>
        </section>
      </div>
    );
  }
  return (
    <WorkflowExecutionDetail
      initialExecution={execution}
      version={version}
      workflowName={version.name}
    />
  );
}
