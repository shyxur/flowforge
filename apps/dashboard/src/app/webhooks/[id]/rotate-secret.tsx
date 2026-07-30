"use client";

import { useActionState } from "react";
import { OneTimeSecret } from "@/components/one-time-secret";
import { SubmitButton } from "@/components/submit-button";
import {
  rotateWebhookSecretAction,
  type WebhookActionState,
} from "../actions";

const initialState: WebhookActionState = {};

export function RotateSecret({ endpointId }: { endpointId: string }) {
  const [state, action] = useActionState(
    rotateWebhookSecretAction.bind(null, endpointId),
    initialState,
  );
  if (state.secret) {
    return <OneTimeSecret secret={state.secret} />;
  }
  return (
    <section className="panel sensitive-panel">
      <div>
        <p className="eyebrow">signing secret</p>
        <h2>rotate credentials</h2>
        <p>
          Rotation immediately invalidates signatures generated with the
          previous secret.
        </p>
      </div>
      <form action={action}>
        <SubmitButton
          className="button button-secondary"
          pendingLabel="rotating…"
        >
          rotate secret
        </SubmitButton>
      </form>
      {state.error && <p className="form-error">{state.error}</p>}
    </section>
  );
}
