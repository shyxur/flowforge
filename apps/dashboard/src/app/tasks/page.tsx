import type { Metadata } from "next";
import Link from "next/link";
import { StatusBadge } from "@/components/status-badge";
import { TaskEvents } from "@/components/task-events";
import { listTasks } from "@/lib/queueflow";

export const metadata: Metadata = { title: "tasks" };

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
    error = caught instanceof Error ? caught.message : "unable to load tasks.";
  }

  return (
    <div>
      <TaskEvents />
      <p className="eyebrow">task explorer</p>
      <div className="page-heading-row">
        <div>
          <h1 className="page-title">tasks</h1>
          <p className="page-copy">
            search and inspect tasks scoped to the authenticated organization.
          </p>
        </div>
      </div>

      <form className="filter-bar panel" method="get">
        <label>
          queue
          <input
            defaultValue={filters.queue}
            name="queue"
            placeholder="all queues"
            type="search"
          />
        </label>
        <label>
          status
          <select defaultValue={filters.status ?? ""} name="status">
            <option value="">all statuses</option>
            <option value="pending">pending</option>
            <option value="processing">processing</option>
            <option value="completed">completed</option>
            <option value="failed">failed</option>
            <option value="dead_letter">dead letter</option>
            <option value="cancelled">cancelled</option>
          </select>
        </label>
        <button className="button button-primary" type="submit">
          apply filters
        </button>
        <Link className="button button-quiet" href="/tasks">
          reset
        </Link>
      </form>

      {error ? (
        <section className="panel api-error">
          <strong>could not reach windylane</strong>
          <p>{error}</p>
        </section>
      ) : tasks.length === 0 ? (
        <section className="panel empty-state">
          <span className="empty-state-mark">01</span>
          <h2>no matching tasks</h2>
          <p>try a different queue or status filter.</p>
        </section>
      ) : (
        <section className="panel table-panel">
          <div className="table-scroll">
            <table className="task-table">
              <thead>
                <tr>
                  <th>task</th>
                  <th>queue</th>
                  <th>status</th>
                  <th>attempts</th>
                  <th>worker</th>
                  <th>created</th>
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
