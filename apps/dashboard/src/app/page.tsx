const metrics = [
  { label: "Pending", value: "—", tone: "amber" },
  { label: "Processing", value: "—", tone: "blue" },
  { label: "Completed", value: "—", tone: "green" },
  { label: "Dead letter", value: "—", tone: "red" },
];

export default function OverviewPage() {
  return (
    <div className="space-y-8">
      <section>
        <p className="eyebrow">Operations overview</p>
        <div className="mt-2 flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
          <div>
            <h1 className="page-title">Queue health at a glance</h1>
            <p className="page-copy">
              Monitor task throughput, workers, retries, and dead letters from
              one tenant-safe control plane.
            </p>
          </div>
          <span className="status-pill">
            <span className="status-dot" />
            Waiting for API
          </span>
        </div>
      </section>

      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {metrics.map((metric) => (
          <article className="panel metric-card" key={metric.label}>
            <div className={`metric-mark metric-${metric.tone}`} />
            <p className="metric-label">{metric.label}</p>
            <p className="metric-value">{metric.value}</p>
            <p className="metric-note">Live data will appear after connection.</p>
          </article>
        ))}
      </section>

      <section className="grid gap-4 xl:grid-cols-[1.4fr_1fr]">
        <article className="panel p-6">
          <div className="section-heading">
            <div>
              <p className="eyebrow">Task lifecycle</p>
              <h2>Reliable by default</h2>
            </div>
            <span className="tag">At-least-once</span>
          </div>
          <div className="mt-8 grid gap-3 sm:grid-cols-4">
            {["Pending", "Processing", "Retry", "Completed / DLQ"].map(
              (step, index) => (
                <div className="flow-step" key={step}>
                  <span>{String(index + 1).padStart(2, "0")}</span>
                  <strong>{step}</strong>
                </div>
              ),
            )}
          </div>
        </article>

        <article className="panel p-6">
          <div className="section-heading">
            <div>
              <p className="eyebrow">Infrastructure</p>
              <h2>System roles</h2>
            </div>
          </div>
          <dl className="mt-6 space-y-4">
            <div className="definition-row">
              <dt>Postgres</dt>
              <dd>Durable source of truth</dd>
            </div>
            <div className="definition-row">
              <dt>Redis</dt>
              <dd>Broker and hot state</dd>
            </div>
            <div className="definition-row">
              <dt>Workers</dt>
              <dd>Tenant-scoped execution</dd>
            </div>
          </dl>
        </article>
      </section>
    </div>
  );
}
