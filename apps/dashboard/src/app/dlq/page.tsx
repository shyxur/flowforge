export default function DLQPage() {
  return (
    <div>
      <p className="eyebrow">Recovery queue</p>
      <h1 className="page-title">Dead letter queue</h1>
      <p className="page-copy">
        Inspect exhausted tasks and safely return them to active processing.
      </p>
      <section className="panel empty-state">
        <span className="empty-state-mark">D</span>
        <h2>No dead letters loaded</h2>
        <p>
          Failed task details and requeue controls will be available from this
          workspace.
        </p>
      </section>
    </div>
  );
}
