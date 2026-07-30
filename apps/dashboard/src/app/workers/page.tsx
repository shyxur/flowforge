export default function WorkersPage() {
  return (
    <div>
      <p className="eyebrow">execution fleet</p>
      <h1 className="page-title">workers</h1>
      <p className="page-copy">
        track worker health, queue assignment, and heartbeat activity.
      </p>
      <section className="panel empty-state">
        <span className="empty-state-mark">03</span>
        <h2>no worker data loaded</h2>
        <p>
          worker heartbeat data will appear here when the dashboard api
          connection is enabled.
        </p>
      </section>
    </div>
  );
}
