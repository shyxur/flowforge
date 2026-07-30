import type { Metadata } from "next";
import Link from "next/link";
import {
  getWorkflow,
  listWorkflowExecutions,
  listWorkflowVersions,
  QueueFlowAPIError,
} from "@/lib/queueflow";
import {
  durationMilliseconds,
  formatDuration,
  formatTimestamp,
} from "@/lib/workflow-execution-model";
import { StatusBadge } from "@/components/status-badge";
import { RunWorkflowDialog } from "./run-workflow-dialog";

export const metadata: Metadata = { title: "workflow executions" };
export const dynamic = "force-dynamic";

const executionStatuses = [
  "pending",
  "running",
  "succeeded",
  "failed",
  "cancelled",
] as const;

export default async function WorkflowExecutionsPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{
    status?: string;
    cursor?: string;
    run?: string;
  }>;
}) {
  const { id } = await params;
  const filters = await searchParams;
  let workflow;
  let versions: Awaited<ReturnType<typeof listWorkflowVersions>> = { items: [] };
  let executions: Awaited<ReturnType<typeof listWorkflowExecutions>> = {
    items: [],
  };
  let error = "";
  try {
    workflow = await getWorkflow(id);
  } catch (caught) {
    error =
      caught instanceof QueueFlowAPIError
        ? friendlyListError(caught)
        : "unable to load workflow";
  }
  if (workflow) {
    try {
      [versions, executions] = await Promise.all([
      listWorkflowVersions(id),
      listWorkflowExecutions(id, {
        status: filters.status,
        cursor: filters.cursor,
        limit: 50,
      }),
      ]);
    } catch (caught) {
      error =
        caught instanceof QueueFlowAPIError
          ? friendlyListError(caught)
          : "unable to load workflow executions";
    }
  }

  if (!workflow) {
    return (
      <div>
        <Link className="back-link" href="/workflows">
          ← back to workflows
        </Link>
        <section className="panel api-error">
          <strong>executions unavailable</strong>
          <p>{error}</p>
        </section>
      </div>
    );
  }

  const canRun = workflow.status === "published" && versions.items.length > 0;

  return (
    <div>
      <Link className="back-link" href={`/workflows/${workflow.id}`}>
        ← back to workflow
      </Link>
      <p className="eyebrow">taskcanvas runtime</p>
      <div className="page-heading-row">
        <div>
          <h1 className="page-title">{workflow.name} executions</h1>
          <p className="page-copy">
            inspect immutable workflow runs and their node-level state.
          </p>
        </div>
        <RunWorkflowDialog
          initialOpen={filters.run === "1"}
          versions={versions.items}
          workflowID={workflow.id}
          workflowStatus={workflow.status}
        />
      </div>

      {!canRun && (
        <p className="editor-banner">
          publish a valid workflow version before starting an execution.
        </p>
      )}

      <form className="filter-bar panel" method="get">
        <label>
          status
          <select defaultValue={filters.status ?? ""} name="status">
            <option value="">all statuses</option>
            {executionStatuses.map((status) => (
              <option key={status} value={status}>
                {status}
              </option>
            ))}
          </select>
        </label>
        <button className="button button-primary" type="submit">
          apply filter
        </button>
        <Link className="button button-quiet" href={`/workflows/${id}/executions`}>
          reset
        </Link>
        <Link
          className="button button-quiet"
          href={`/workflows/${id}/executions${filters.status ? `?status=${filters.status}` : ""}`}
        >
          refresh
        </Link>
      </form>

      {error ? (
        <section className="panel api-error">
          <strong>executions unavailable</strong>
          <p>{error}</p>
          <Link
            className="button button-secondary empty-action"
            href={`/workflows/${id}/executions`}
          >
            retry
          </Link>
        </section>
      ) : executions.items.length === 0 ? (
        <section className="panel empty-state">
          <span className="empty-state-mark">RUN</span>
          <h2>no workflow executions</h2>
          <p>run a published immutable version to begin runtime inspection.</p>
        </section>
      ) : (
        <>
          <section className="panel table-panel">
            <div className="table-scroll">
              <table className="task-table execution-table">
                <thead>
                  <tr>
                    <th>execution</th>
                    <th>version</th>
                    <th>status</th>
                    <th>created</th>
                    <th>started</th>
                    <th>completed</th>
                    <th>duration</th>
                    <th>failure</th>
                  </tr>
                </thead>
                <tbody>
                  {executions.items.map((execution) => (
                    <tr key={execution.execution_id}>
                      <td>
                        <Link
                          className="task-link"
                          href={`/workflows/${id}/executions/${execution.execution_id}`}
                          title={execution.execution_id}
                        >
                          {shortID(execution.execution_id)}
                        </Link>
                      </td>
                      <td>v{execution.workflow_version}</td>
                      <td>
                        <StatusBadge status={execution.status} />
                      </td>
                      <td>{formatTimestamp(execution.created_at)}</td>
                      <td>{formatTimestamp(execution.started_at)}</td>
                      <td>{formatTimestamp(execution.completed_at)}</td>
                      <td>
                        {formatDuration(
                          durationMilliseconds(
                            execution.started_at,
                            execution.completed_at,
                          ),
                        )}
                      </td>
                      <td className="execution-failure-cell">
                        {execution.error_message || "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
          {executions.next_cursor && (
            <div className="pagination-row">
              <Link
                className="button button-secondary"
                href={`/workflows/${id}/executions?${nextPageQuery(
                  executions.next_cursor,
                  filters.status,
                )}`}
              >
                next page
              </Link>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function nextPageQuery(cursor: string, status?: string) {
  const query = new URLSearchParams({ cursor });
  if (status) query.set("status", status);
  return query.toString();
}

function shortID(id: string) {
  return `${id.slice(0, 8)}…${id.slice(-4)}`;
}

function friendlyListError(error: QueueFlowAPIError) {
  if (error.status === 429) return "rate limit reached; wait briefly and retry";
  if (error.status >= 500) return "windylane is unavailable; retry in a moment";
  return error.message;
}
