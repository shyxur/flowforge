export default function WorkersPage() {
  return (
    <div>
      <p className="eyebrow">Execution fleet</p>
      <h1 className="page-title">Workers</h1>
      <p className="page-copy">
        Track worker health, queue assignment, and heartbeat activity.
      </p>
      <section className="panel empty-state">
        <span className="empty-state-mark">W</span>
        <h2>No worker data loaded</h2>
        <p>
          Worker heartbeat data will appear here when the dashboard API
          connection is enabled.
        </p>
      </section>
    </div>
  );
}
