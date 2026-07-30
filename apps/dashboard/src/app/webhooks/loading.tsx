export default function WebhooksLoading() {
  return (
    <div aria-busy="true">
      <p className="eyebrow">eventforge</p>
      <h1 className="page-title">loading webhooks…</h1>
      <div className="panel loading-panel">
        <span />
        <span />
        <span />
      </div>
    </div>
  );
}
