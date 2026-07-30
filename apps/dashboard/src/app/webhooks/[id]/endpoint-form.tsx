"use client";

import { useActionState } from "react";
import { SubmitButton } from "@/components/submit-button";
import {
  WEBHOOK_EVENT_TYPES,
  type WebhookEndpoint,
} from "@/lib/webhook-types";
import {
  type WebhookActionState,
  updateWebhookAction,
} from "../actions";

const initialState: WebhookActionState = {};

export function EndpointForm({ endpoint }: { endpoint: WebhookEndpoint }) {
  const [state, action] = useActionState(
    updateWebhookAction.bind(null, endpoint.id),
    initialState,
  );
  return (
    <form action={action} className="panel resource-form detail-form">
      <div className="section-heading">
        <div>
          <p className="eyebrow">configuration</p>
          <h2>endpoint settings</h2>
        </div>
        <StatusMessage state={state} />
      </div>

      <div className="form-grid">
        <label>
          <span>name</span>
          <input defaultValue={endpoint.name} name="name" required />
        </label>
        <label>
          <span>endpoint url</span>
          <input defaultValue={endpoint.url} name="url" required type="url" />
        </label>
      </div>

      <fieldset className="event-checklist">
        <legend>event types</legend>
        {WEBHOOK_EVENT_TYPES.map((eventType) => (
          <label key={eventType}>
            <input
              defaultChecked={endpoint.event_types.includes(eventType)}
              name="event_types"
              type="checkbox"
              value={eventType}
            />
            <span>{eventType}</span>
          </label>
        ))}
      </fieldset>

      <label className="switch-row">
        <input defaultChecked={endpoint.is_active} name="is_active" type="checkbox" />
        <span>
          <strong>active</strong>
          <small>allow the delivery worker to send matching events</small>
        </span>
      </label>

      {state.error && <p className="form-error">{state.error}</p>}
      <div className="form-actions">
        <SubmitButton>save changes</SubmitButton>
      </div>
    </form>
  );
}

function StatusMessage({ state }: { state: WebhookActionState }) {
  if (!state.message) return null;
  return <span className="success-note">{state.message}</span>;
}
