import type { Metadata } from "next";
import Link from "next/link";
import { StatusBadge } from "@/components/status-badge";
import {
  listWebhookEndpoints,
  QueueFlowAPIError,
} from "@/lib/queueflow";
import { deleteWebhookEndpointAction } from "./actions";

export const metadata: Metadata = { title: "Webhooks" };
export const dynamic = "force-dynamic";

export default async function WebhooksPage() {
  let endpoints: Awaited<
    ReturnType<typeof listWebhookEndpoints>
  >["items"] = [];
  let error = "";
  try {
    endpoints = (await listWebhookEndpoints()).items;
  } catch (caught) {
    error =
      caught instanceof QueueFlowAPIError
        ? caught.message
        : "unable to load webhook endpoints";
  }

  return (
    <div>
      <p className="eyebrow">eventforge</p>
      <div className="page-heading-row">
        <div>
          <h1 className="page-title">webhooks</h1>
          <p className="page-copy">
            Manage tenant-scoped destinations for signed task lifecycle events.
          </p>
        </div>
        <Link className="button button-primary" href="/webhooks/new">
          new endpoint
        </Link>
      </div>

      {error ? (
        <section className="panel api-error">
          <strong>webhooks unavailable</strong>
          <p>{error}</p>
        </section>
      ) : endpoints.length === 0 ? (
        <section className="panel empty-state">
          <span className="empty-state-mark">H</span>
          <h2>no webhook endpoints</h2>
          <p>Create an endpoint to begin forwarding QueueFlow task events.</p>
          <Link className="button button-primary empty-action" href="/webhooks/new">
            create endpoint
          </Link>
        </section>
      ) : (
        <section className="panel table-panel">
          <div className="table-scroll">
            <table className="task-table">
              <thead>
                <tr>
                  <th>name</th>
                  <th>url</th>
                  <th>status</th>
                  <th>event types</th>
                  <th>created</th>
                  <th>actions</th>
                </tr>
              </thead>
              <tbody>
                {endpoints.map((endpoint) => (
                  <tr key={endpoint.id}>
                    <td>
                      <Link className="resource-link" href={`/webhooks/${endpoint.id}`}>
                        {endpoint.name}
                      </Link>
                    </td>
                    <td className="url-cell">{endpoint.url}</td>
                    <td>
                      <StatusBadge status={endpoint.is_active ? "active" : "disabled"} />
                    </td>
                    <td>
                      <div className="chip-list">
                        {endpoint.event_types.map((eventType) => (
                          <span className="event-chip" key={eventType}>
                            {eventType}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td>{formatDate(endpoint.created_at)}</td>
                    <td>
                      <div className="table-actions">
                        <Link
                          className="button button-quiet button-small"
                          href={`/webhooks/${endpoint.id}`}
                        >
                          view / edit
                        </Link>
                        <form action={deleteWebhookEndpointAction.bind(null, endpoint.id)}>
                          <button
                            className="button button-danger button-small"
                            type="submit"
                          >
                            delete
                          </button>
                        </form>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
