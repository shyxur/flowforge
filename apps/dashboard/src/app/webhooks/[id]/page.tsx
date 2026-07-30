import type { Metadata } from "next";
import Link from "next/link";
import { StatusBadge } from "@/components/status-badge";
import { WebhookDeliveryTable } from "@/components/webhook-delivery-table";
import {
  getWebhookEndpoint,
  listWebhookDeliveries,
  QueueFlowAPIError,
} from "@/lib/queueflow";
import { deleteWebhookEndpointAction } from "../actions";
import { EndpointForm } from "./endpoint-form";
import { RotateSecret } from "./rotate-secret";

export const metadata: Metadata = { title: "Webhook detail" };

type Props = { params: Promise<{ id: string }> };

export default async function WebhookDetailPage({ params }: Props) {
  const { id } = await params;
  let endpoint;
  try {
    endpoint = await getWebhookEndpoint(id);
  } catch (caught) {
    return (
      <ResourceError
        message={
          caught instanceof QueueFlowAPIError
            ? caught.message
            : "unable to load this webhook endpoint"
        }
      />
    );
  }

  let deliveries: Awaited<
    ReturnType<typeof listWebhookDeliveries>
  >["items"] = [];
  let deliveryError = "";
  try {
    deliveries = (
      await listWebhookDeliveries({ endpoint_id: id, limit: 20 })
    ).items;
  } catch (caught) {
    deliveryError =
      caught instanceof QueueFlowAPIError
        ? caught.message
        : "unable to load recent deliveries";
  }

  return (
    <div>
      <Link className="back-link" href="/webhooks">
        ← back to webhooks
      </Link>
      <div className="detail-heading">
        <div>
          <p className="eyebrow">webhook endpoint</p>
          <h1 className="resource-title">{endpoint.name}</h1>
          <div className="detail-subline">
            <StatusBadge status={endpoint.is_active ? "active" : "disabled"} />
            <span>{endpoint.url}</span>
          </div>
        </div>
        <form action={deleteWebhookEndpointAction.bind(null, endpoint.id)}>
          <button className="button button-danger" type="submit">
            delete endpoint
          </button>
        </form>
      </div>

      <section className="detail-grid compact-details">
        <article className="panel detail-card">
          <p className="eyebrow">identity</p>
          <dl className="detail-list">
            <Detail label="endpoint id" value={endpoint.id} />
            <Detail label="organization" value={endpoint.org_id} />
          </dl>
        </article>
        <article className="panel detail-card">
          <p className="eyebrow">timestamps</p>
          <dl className="detail-list">
            <Detail label="created" value={formatDate(endpoint.created_at)} />
            <Detail label="updated" value={formatDate(endpoint.updated_at)} />
          </dl>
        </article>
      </section>

      <div className="resource-stack">
        <EndpointForm endpoint={endpoint} />
        <RotateSecret endpointId={endpoint.id} />
      </div>

      <section className="recent-section">
        <div className="section-heading">
          <div>
            <p className="eyebrow">delivery activity</p>
            <h2>recent deliveries</h2>
          </div>
          <Link
            className="button button-secondary"
            href={`/webhook-deliveries?endpoint_id=${endpoint.id}`}
          >
            view all
          </Link>
        </div>
        {deliveryError ? (
          <section className="panel api-error">
            <strong>delivery history unavailable</strong>
            <p>{deliveryError}</p>
          </section>
        ) : deliveries.length === 0 ? (
          <section className="panel empty-state compact-empty">
            <h2>no deliveries yet</h2>
            <p>Matching task events will appear here after they are emitted.</p>
          </section>
        ) : (
          <section className="panel table-panel">
            <WebhookDeliveryTable deliveries={deliveries} />
          </section>
        )}
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

function ResourceError({ message }: { message: string }) {
  return (
    <div>
      <Link className="back-link" href="/webhooks">
        ← back to webhooks
      </Link>
      <section className="panel api-error">
        <strong>webhook unavailable</strong>
        <p>{message}</p>
      </section>
    </div>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "long",
  }).format(new Date(value));
}
