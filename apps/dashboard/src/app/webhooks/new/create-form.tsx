"use client";

import { useActionState } from "react";
import { OneTimeSecret } from "@/components/one-time-secret";
import { SubmitButton } from "@/components/submit-button";
import { WEBHOOK_EVENT_TYPES } from "@/lib/webhook-types";
import {
  createWebhookAction,
  type WebhookActionState,
} from "../actions";

const initialState: WebhookActionState = {};

export function CreateWebhookForm() {
  const [state, action] = useActionState(
    createWebhookAction,
    initialState,
  );

  if (state.secret) {
    return (
      <OneTimeSecret secret={state.secret} endpointId={state.endpointId} />
    );
  }

  return (
    <form action={action} className="panel resource-form">
      <div className="form-grid">
        <label>
          <span>name</span>
          <input
            autoComplete="off"
            name="name"
            placeholder="production task events"
            required
          />
        </label>
        <label>
          <span>endpoint url</span>
          <input
            autoComplete="url"
            name="url"
            placeholder="https://example.com/webhooks/flowforge"
            required
            type="url"
          />
        </label>
      </div>

      <fieldset className="event-checklist">
        <legend>event types</legend>
        {WEBHOOK_EVENT_TYPES.map((eventType) => (
          <label key={eventType}>
            <input name="event_types" type="checkbox" value={eventType} />
            <span>{eventType}</span>
          </label>
        ))}
      </fieldset>

      <label className="switch-row">
        <input defaultChecked name="is_active" type="checkbox" />
        <span>
          <strong>active</strong>
          <small>deliver matching events immediately after creation</small>
        </span>
      </label>

      {state.error && <p className="form-error">{state.error}</p>}
      <div className="form-actions">
        <SubmitButton pendingLabel="creating…">create endpoint</SubmitButton>
      </div>
    </form>
  );
}
