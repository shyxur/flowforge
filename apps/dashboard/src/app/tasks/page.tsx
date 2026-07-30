import type { Metadata } from "next";
import Link from "next/link";
import { StatusBadge } from "@/components/status-badge";
import { listTasks } from "@/lib/queueflow";

export const metadata: Metadata = { title: "Tasks" };

type Props = {
  searchParams: Promise<{ queue?: string; status?: string }>;
};

export default async function TasksPage({ searchParams }: Props) {
  const filters = await searchParams;
  let error = "";
  let tasks: Awaited<ReturnType<typeof listTasks>>["items"] = [];
  try {
    tasks = (await listTasks(filters)).items;
  } catch (caught) {
    error = caught instanceof Error ? caught.message : "Unable to load tasks.";
  }

  return (
    <div>
      <p className="eyebrow">Task explorer</p>
      <div className="page-heading-row">
        <div>
          <h1 className="page-title">Tasks</h1>
          <p className="page-copy">
            Search and inspect tasks scoped to the authenticated organization.
          </p>
        </div>
      </div>

      <form className="filter-bar panel" method="get">
        <label>
          Queue
          <input
            defaultValue={filters.queue}
            name="queue"
            placeholder="All queues"
            type="search"
          />
        </label>
        <label>
          Status
          <select defaultValue={filters.status ?? ""} name="status">
            <option value="">All statuses</option>
            <option value="pending">Pending</option>
            <option value="processing">Processing</option>
            <option value="completed">Completed</option>
            <option value="failed">Failed</option>
            <option value="dead_letter">Dead letter</option>
            <option value="cancelled">Cancelled</option>
          </select>
        </label>
        <button className="button button-primary" type="submit">
          Apply filters
        </button>
        <Link className="button button-quiet" href="/tasks">
          Reset
        </Link>
      </form>

      {error ? (
        <section className="panel api-error">
          <strong>Could not reach QueueFlow</strong>
          <p>{error}</p>
        </section>
      ) : tasks.length === 0 ? (
        <section className="panel empty-state">
          <span className="empty-state-mark">T</span>
          <h2>No matching tasks</h2>
          <p>Try a different queue or status filter.</p>
        </section>
      ) : (
        <section className="panel table-panel">
          <div className="table-scroll">
            <table className="task-table">
              <thead>
                <tr>
                  <th>Task</th>
                  <th>Queue</th>
                  <th>Status</th>
                  <th>Attempts</th>
                  <th>Worker</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {tasks.map((task) => (
                  <tr key={task.id}>
                    <td>
                      <Link className="task-link" href={`/tasks/${task.id}`}>
                        {task.id}
                      </Link>
                    </td>
                    <td>{task.queue}</td>
                    <td>
                      <StatusBadge status={task.status} />
                    </td>
                    <td>
                      {task.attempts} / {task.max_attempts}
                    </td>
                    <td>{task.locked_by || "—"}</td>
                    <td>{formatDate(task.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
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
