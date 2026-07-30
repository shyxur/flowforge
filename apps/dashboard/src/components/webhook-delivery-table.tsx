import Link from "next/link";
import { StatusBadge } from "@/components/status-badge";
import type { WebhookDelivery } from "@/lib/queueflow";

export function WebhookDeliveryTable({
  deliveries,
}: {
  deliveries: WebhookDelivery[];
}) {
  return (
    <div className="table-scroll">
      <table className="task-table">
        <thead>
          <tr>
            <th>delivery</th>
            <th>endpoint</th>
            <th>event</th>
            <th>status</th>
            <th>attempts</th>
            <th>response</th>
            <th>next attempt</th>
            <th>created</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {deliveries.map((delivery) => (
            <tr key={delivery.id}>
              <td className="mono-cell">{shortID(delivery.id)}</td>
              <td className="mono-cell">{shortID(delivery.endpoint_id)}</td>
              <td>{delivery.event_type}</td>
              <td>
                <StatusBadge status={delivery.status} />
              </td>
              <td>
                {delivery.attempt_count} / {delivery.max_attempts}
              </td>
              <td>{delivery.response_status ?? "—"}</td>
              <td>{formatOptionalDate(delivery.next_attempt_at)}</td>
              <td>{formatDate(delivery.created_at)}</td>
              <td>
                <Link
                  aria-label={`view delivery ${delivery.id}`}
                  className="button button-quiet button-small"
                  href={`/webhook-deliveries/${delivery.id}`}
                >
                  view
                </Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function shortID(value: string) {
  return `${value.slice(0, 8)}…${value.slice(-4)}`;
}

function formatOptionalDate(value?: string) {
  return value ? formatDate(value) : "—";
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
