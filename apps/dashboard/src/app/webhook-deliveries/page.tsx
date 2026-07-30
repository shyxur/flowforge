import type { Metadata } from "next";
import Link from "next/link";
import { WebhookDeliveryTable } from "@/components/webhook-delivery-table";
import {
  listWebhookDeliveries,
  QueueFlowAPIError,
} from "@/lib/queueflow";
import { WEBHOOK_EVENT_TYPES } from "@/lib/webhook-types";

export const metadata: Metadata = { title: "Webhook deliveries" };

type Props = {
  searchParams: Promise<{
    endpoint_id?: string;
    status?: string;
    event_type?: string;
  }>;
};

const deliveryStatuses = [
  "pending",
  "delivering",
  "delivered",
  "retrying",
  "failed",
];

export default async function WebhookDeliveriesPage({ searchParams }: Props) {
  const filters = await searchParams;
  let deliveries: Awaited<
    ReturnType<typeof listWebhookDeliveries>
  >["items"] = [];
  let error = "";
  try {
    deliveries = (await listWebhookDeliveries(filters)).items;
  } catch (caught) {
    error =
      caught instanceof QueueFlowAPIError
        ? caught.message
        : "unable to load webhook deliveries";
  }

  return (
    <div>
      <p className="eyebrow">eventforge</p>
      <h1 className="page-title">webhook deliveries</h1>
      <p className="page-copy">
        Inspect signed delivery attempts, HTTP outcomes, and scheduled retries.
      </p>

      <form className="filter-bar panel delivery-filters" method="get">
        <label>
          endpoint id
          <input
            defaultValue={filters.endpoint_id}
            name="endpoint_id"
            placeholder="all endpoints"
            type="search"
          />
        </label>
        <label>
          status
          <select defaultValue={filters.status ?? ""} name="status">
            <option value="">all statuses</option>
            {deliveryStatuses.map((status) => (
              <option key={status} value={status}>
                {status}
              </option>
            ))}
          </select>
        </label>
        <label>
          event type
          <select defaultValue={filters.event_type ?? ""} name="event_type">
            <option value="">all events</option>
            {WEBHOOK_EVENT_TYPES.map((eventType) => (
              <option key={eventType} value={eventType}>
                {eventType}
              </option>
            ))}
          </select>
        </label>
        <button className="button button-primary" type="submit">
          apply filters
        </button>
        <Link className="button button-quiet" href="/webhook-deliveries">
          reset
        </Link>
      </form>

      {error ? (
        <section className="panel api-error">
          <strong>deliveries unavailable</strong>
          <p>{error}</p>
        </section>
      ) : deliveries.length === 0 ? (
        <section className="panel empty-state">
          <span className="empty-state-mark">L</span>
          <h2>no matching deliveries</h2>
          <p>Adjust the filters or wait for a subscribed task event.</p>
        </section>
      ) : (
        <section className="panel table-panel">
          <WebhookDeliveryTable deliveries={deliveries} />
        </section>
      )}
    </div>
  );
}
