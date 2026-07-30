export default function DLQPage() {
  return (
    <div>
      <p className="eyebrow">recovery queue</p>
      <h1 className="page-title">dead letter queue</h1>
      <p className="page-copy">
        inspect exhausted tasks and safely return them to active processing.
      </p>
      <section className="panel empty-state">
        <span className="empty-state-mark">04</span>
        <h2>no dead letters loaded</h2>
        <p>
          failed task details and requeue controls will be available from this
          workspace.
        </p>
      </section>
    </div>
  );
}
