import Link from "next/link";

const operationalSurfaces = [
  {
    href: "/tasks",
    label: "tasks",
    description: "inspect queue state, attempts, workers, and timestamps.",
  },
  {
    href: "/webhooks",
    label: "webhooks",
    description: "manage signed task lifecycle event destinations.",
  },
  {
    href: "/webhook-deliveries",
    label: "webhook deliveries",
    description: "review delivery attempts, responses, and scheduled retries.",
  },
  {
    href: "/dlq",
    label: "dead letter queue",
    description: "inspect exhausted tasks and recovery actions.",
  },
];

export default function OverviewPage() {
  return (
    <div className="space-y-8">
      <section>
        <p className="eyebrow">operations overview</p>
        <div className="mt-2 flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
          <div>
            <h1 className="page-title">queue operations</h1>
            <p className="page-copy">
              inspect tasks, workers, retries, and webhook delivery from one
              tenant-safe control plane.
            </p>
          </div>
          <span className="status-pill">
            <span className="status-dot" />
            tenant scoped
          </span>
        </div>
      </section>

      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {operationalSurfaces.map((surface) => (
          <Link
            className="panel metric-card operation-card"
            href={surface.href}
            key={surface.href}
          >
            <p className="metric-label">{surface.label}</p>
            <p className="metric-note">{surface.description}</p>
            <span>open →</span>
          </Link>
        ))}
      </section>

      <section className="grid gap-4 xl:grid-cols-[1.4fr_1fr]">
        <article className="panel p-6">
          <div className="section-heading">
            <div>
              <p className="eyebrow">task lifecycle</p>
              <h2>reliable by default</h2>
            </div>
            <span className="tag">at-least-once</span>
          </div>
          <div className="mt-8 grid gap-3 sm:grid-cols-4">
            {["pending", "processing", "retry", "completed / dlq"].map(
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
              <p className="eyebrow">infrastructure</p>
              <h2>system roles</h2>
            </div>
          </div>
          <dl className="mt-6 space-y-4">
            <div className="definition-row">
              <dt>postgres</dt>
              <dd>durable source of truth</dd>
            </div>
            <div className="definition-row">
              <dt>redis</dt>
              <dd>broker and hot state</dd>
            </div>
            <div className="definition-row">
              <dt>workers</dt>
              <dd>tenant-scoped execution</dd>
            </div>
          </dl>
        </article>
      </section>
    </div>
  );
}
