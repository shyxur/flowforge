import type { Metadata } from "next";
import Link from "next/link";
import { StatusBadge } from "@/components/status-badge";
import { getTask, QueueFlowAPIError } from "@/lib/queueflow";
import {
  cancelTaskAction,
  deleteTaskAction,
  retryTaskAction,
} from "./actions";

export const metadata: Metadata = { title: "Task detail" };

type Props = { params: Promise<{ id: string }> };

export default async function TaskDetailPage({ params }: Props) {
  const { id } = await params;
  let task;
  try {
    task = await getTask(id);
  } catch (caught) {
    const message =
      caught instanceof QueueFlowAPIError
        ? caught.message
        : "Unable to load this task.";
    return (
      <div>
        <Link className="back-link" href="/tasks">
          ← Back to tasks
        </Link>
        <section className="panel api-error">
          <strong>Task unavailable</strong>
          <p>{message}</p>
        </section>
      </div>
    );
  }

  const canRetry = ["failed", "dead_letter", "cancelled"].includes(task.status);
  const canCancel = task.status === "pending";
  const canDelete = ["completed", "failed", "dead_letter", "cancelled"].includes(
    task.status,
  );

  return (
    <div>
      <Link className="back-link" href="/tasks">
        ← Back to tasks
      </Link>
      <div className="detail-heading">
        <div>
          <p className="eyebrow">Task detail</p>
          <h1 className="task-id-title">{task.id}</h1>
          <div className="detail-subline">
            <StatusBadge status={task.status} />
            <span>{task.queue}</span>
          </div>
        </div>
        <div className="action-row">
          {canRetry && (
            <form action={retryTaskAction.bind(null, task.id)}>
              <button className="button button-primary" type="submit">
                Retry task
              </button>
            </form>
          )}
          {canCancel && (
            <form action={cancelTaskAction.bind(null, task.id)}>
              <button className="button button-secondary" type="submit">
                Cancel
              </button>
            </form>
          )}
          {canDelete && (
            <form action={deleteTaskAction.bind(null, task.id)}>
              <button className="button button-danger" type="submit">
                Delete
              </button>
            </form>
          )}
        </div>
      </div>

      <section className="detail-grid">
        <article className="panel detail-card">
          <p className="eyebrow">Execution</p>
          <dl className="detail-list">
            <Detail label="Queue" value={task.queue} />
            <Detail label="Priority" value={String(task.priority)} />
            <Detail label="Retry count" value={String(task.attempts)} />
            <Detail
              label="Max retries"
              value={String(Math.max(0, task.max_attempts - 1))}
            />
            <Detail label="Worker ID" value={task.locked_by || "—"} />
          </dl>
        </article>
        <article className="panel detail-card">
          <p className="eyebrow">Timing</p>
          <dl className="detail-list">
            <Detail label="Created" value={formatDate(task.created_at)} />
            <Detail label="Updated" value={formatDate(task.updated_at)} />
          </dl>
        </article>
      </section>

      {task.last_error && (
        <section className="panel error-panel">
          <p className="eyebrow">Last error</p>
          <pre>{task.last_error}</pre>
        </section>
      )}

      <section className="json-grid">
        <JSONPanel label="Payload" value={task.payload} />
        <JSONPanel label="Result" value={task.result ?? null} />
      </section>
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function JSONPanel({ label, value }: { label: string; value: unknown }) {
  return (
    <article className="panel code-panel">
      <p className="eyebrow">{label}</p>
      <pre>{JSON.stringify(value, null, 2)}</pre>
    </article>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "long",
  }).format(new Date(value));
}
