import type { Metadata } from "next";
import Link from "next/link";
import { StatusBadge } from "@/components/status-badge";
import {
  getWebhookDelivery,
  QueueFlowAPIError,
} from "@/lib/queueflow";
import { retryWebhookDeliveryAction } from "../actions";

export const metadata: Metadata = { title: "webhook delivery detail" };

type Props = { params: Promise<{ id: string }> };

export default async function WebhookDeliveryDetailPage({ params }: Props) {
  const { id } = await params;
  let delivery;
  try {
    delivery = await getWebhookDelivery(id);
  } catch (caught) {
    return (
      <div>
        <Link className="back-link" href="/webhook-deliveries">
          ← back to deliveries
        </Link>
        <section className="panel api-error">
          <strong>delivery unavailable</strong>
          <p>
            {caught instanceof QueueFlowAPIError
              ? caught.message
              : "unable to load this webhook delivery"}
          </p>
        </section>
      </div>
    );
  }

  const canRetry = ["failed", "retrying"].includes(delivery.status);

  return (
    <div>
      <Link className="back-link" href="/webhook-deliveries">
        ← back to deliveries
      </Link>
      <div className="detail-heading">
        <div>
          <p className="eyebrow">webhook delivery</p>
          <h1 className="resource-title mono-title">{delivery.id}</h1>
          <div className="detail-subline">
            <StatusBadge status={delivery.status} />
            <span>{delivery.event_type}</span>
          </div>
        </div>
        {canRetry && (
          <form action={retryWebhookDeliveryAction.bind(null, delivery.id)}>
            <button className="button button-primary" type="submit">
              retry delivery
            </button>
          </form>
        )}
      </div>

      <section className="detail-grid">
        <article className="panel detail-card">
          <p className="eyebrow">attempt</p>
          <dl className="detail-list">
            <Detail label="endpoint id" value={delivery.endpoint_id} />
            <Detail
              label="attempt count"
              value={`${delivery.attempt_count} / ${delivery.max_attempts}`}
            />
            <Detail
              label="response status"
              value={delivery.response_status?.toString() ?? "—"}
            />
            <Detail
              label="next attempt"
              value={formatOptionalDate(delivery.next_attempt_at)}
            />
          </dl>
        </article>
        <article className="panel detail-card">
          <p className="eyebrow">timing</p>
          <dl className="detail-list">
            <Detail label="created" value={formatDate(delivery.created_at)} />
            <Detail label="updated" value={formatDate(delivery.updated_at)} />
            <Detail
              label="last attempt"
              value={formatOptionalDate(delivery.last_attempt_at)}
            />
          </dl>
        </article>
      </section>

      {(delivery.last_error || delivery.response_body) && (
        <section className="message-grid">
          {delivery.last_error && (
            <article className="panel error-panel">
              <p className="eyebrow">last error</p>
              <pre>{delivery.last_error}</pre>
            </article>
          )}
          {delivery.response_body && (
            <article className="panel code-panel response-panel">
              <p className="eyebrow">response body</p>
              <pre>{delivery.response_body}</pre>
            </article>
          )}
        </section>
      )}

      <section className="panel code-panel delivery-payload">
        <p className="eyebrow">payload</p>
        <pre>{JSON.stringify(delivery.payload ?? null, null, 2)}</pre>
      </section>
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function formatOptionalDate(value?: string) {
  return value ? formatDate(value) : "—";
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "long",
  }).format(new Date(value));
}
