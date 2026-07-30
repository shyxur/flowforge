export function OneTimeSecret({
  secret,
  endpointId,
}: {
  secret: string;
  endpointId?: string;
}) {
  return (
    <section aria-live="polite" className="secret-panel">
      <div>
        <p className="eyebrow">one-time signing secret</p>
        <h2>copy this secret now</h2>
        <p>
          It will not be shown again. Store it in your webhook consumer&apos;s
          secret manager.
        </p>
      </div>
      <code>{secret}</code>
      {endpointId && (
        <a className="button button-secondary" href={`/webhooks/${endpointId}`}>
          open endpoint
        </a>
      )}
    </section>
  );
}
