import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import {
  getWorkflow,
  listWebhookEndpoints,
  listWorkflowVersions,
  QueueFlowAPIError,
} from "@/lib/queueflow";
import { WorkflowEditor } from "./workflow-editor";

export const metadata: Metadata = { title: "workflow editor" };
export const dynamic = "force-dynamic";

export default async function WorkflowEditorPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ created?: string }>;
}) {
  const { id } = await params;
  const { created } = await searchParams;
  let workflow;
  let endpoints;
  let versions;
  try {
    [workflow, endpoints, versions] = await Promise.all([
      getWorkflow(id),
      listWebhookEndpoints().catch(() => ({ items: [] })),
      listWorkflowVersions(id).catch(() => ({ items: [] })),
    ]);
  } catch (error) {
    if (error instanceof QueueFlowAPIError && error.status === 404) notFound();
    return (
      <div className="narrow-page">
        <Link className="back-link" href="/workflows">
          ← back to workflows
        </Link>
        <section className="panel api-error">
          <strong>workflow unavailable</strong>
          <p>
            {error instanceof QueueFlowAPIError
              ? error.message
              : "unable to load workflow"}
          </p>
        </section>
      </div>
    );
  }
  return (
    <WorkflowEditor
      created={created === "1"}
      initialVersions={versions.items}
      initialWorkflow={workflow}
      webhookEndpoints={endpoints.items.filter((endpoint) => endpoint.is_active)}
    />
  );
}
