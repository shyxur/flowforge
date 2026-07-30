import type { Metadata } from "next";
import Link from "next/link";
import { StatusBadge } from "@/components/status-badge";
import {
  listWorkflows,
  listWorkflowVersions,
  QueueFlowAPIError,
} from "@/lib/queueflow";

export const metadata: Metadata = { title: "workflows" };
export const dynamic = "force-dynamic";

export default async function WorkflowsPage({
  searchParams,
}: {
  searchParams: Promise<{ cursor?: string }>;
}) {
  const { cursor } = await searchParams;
  let error = "";
  let page: Awaited<ReturnType<typeof listWorkflows>> = { items: [] };
  try {
    page = await listWorkflows({ cursor, limit: 50 });
  } catch (caught) {
    error =
      caught instanceof QueueFlowAPIError
        ? caught.message
        : "unable to load workflows";
  }

  const latestVersions = new Map<string, number>();
  if (!error) {
    await Promise.all(
      page.items.map(async (workflow) => {
        try {
          const versions = await listWorkflowVersions(workflow.id);
          if (versions.items[0]) {
            latestVersions.set(workflow.id, versions.items[0].version);
          }
        } catch {
          // The workflow list remains useful when one version lookup fails.
        }
      }),
    );
  }

  return (
    <div>
      <p className="eyebrow">taskcanvas</p>
      <div className="page-heading-row">
        <div>
          <h1 className="page-title">workflows</h1>
          <p className="page-copy">
            connect triggers and actions in durable, publishable flows.
          </p>
        </div>
        <Link className="button button-primary" href="/workflows/new">
          new workflow
        </Link>
      </div>

      {error ? (
        <section className="panel api-error">
          <strong>workflows unavailable</strong>
          <p>{error}</p>
          <Link className="button button-secondary empty-action" href="/workflows">
            retry
          </Link>
        </section>
      ) : page.items.length === 0 ? (
        <section className="panel empty-state">
          <span className="empty-state-mark">TC</span>
          <h2>no workflows yet</h2>
          <p>connect a trigger to tasks, webhooks, delays and conditions.</p>
          <Link className="button button-primary empty-action" href="/workflows/new">
            create workflow
          </Link>
        </section>
      ) : (
        <>
          <section className="panel table-panel">
            <div className="table-scroll">
              <table className="task-table">
                <thead>
                  <tr>
                    <th>name</th>
                    <th>slug</th>
                    <th>status</th>
                    <th>latest version</th>
                    <th>updated</th>
                    <th>action</th>
                  </tr>
                </thead>
                <tbody>
                  {page.items.map((workflow) => (
                    <tr key={workflow.id}>
                      <td>
                        <Link
                          className="resource-link"
                          href={`/workflows/${workflow.id}`}
                        >
                          {workflow.name}
                        </Link>
                        {workflow.description && (
                          <small className="workflow-description">
                            {workflow.description}
                          </small>
                        )}
                      </td>
                      <td className="mono-cell">{workflow.slug}</td>
                      <td>
                        <StatusBadge status={workflow.status} />
                      </td>
                      <td>
                        {latestVersions.has(workflow.id)
                          ? `v${latestVersions.get(workflow.id)}`
                          : "—"}
                      </td>
                      <td>{formatDate(workflow.updated_at)}</td>
                      <td>
                        <Link
                          className="button button-quiet button-small"
                          href={`/workflows/${workflow.id}`}
                        >
                          open editor
                        </Link>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
          {page.next_cursor && (
            <div className="pagination-row">
              <Link
                className="button button-secondary"
                href={`/workflows?cursor=${encodeURIComponent(page.next_cursor)}`}
              >
                load newer page
              </Link>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
