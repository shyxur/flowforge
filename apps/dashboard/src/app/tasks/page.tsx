export default function TasksPage() {
  return (
    <div>
      <p className="eyebrow">Task explorer</p>
      <h1 className="page-title">Tasks</h1>
      <p className="page-copy">
        Search, filter, inspect, retry, and cancel tenant-scoped tasks.
      </p>
      <section className="panel empty-state">
        <span className="empty-state-mark">T</span>
        <h2>Task data is next</h2>
        <p>
          The application shell is ready. This page will connect to the
          QueueFlow task API in the next dashboard increment.
        </p>
      </section>
    </div>
  );
}
