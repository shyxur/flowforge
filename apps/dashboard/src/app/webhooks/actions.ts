"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import {
  createWebhookEndpoint,
  deleteWebhookEndpoint,
  QueueFlowAPIError,
  rotateWebhookSecret,
  updateWebhookEndpoint,
} from "@/lib/queueflow";
import {
  WEBHOOK_EVENT_TYPES,
  type WebhookEventType,
} from "@/lib/webhook-types";

export type WebhookActionState = {
  error?: string;
  message?: string;
  secret?: string;
  endpointId?: string;
};

export async function createWebhookAction(
  _previousState: WebhookActionState,
  formData: FormData,
): Promise<WebhookActionState> {
  try {
    const result = await createWebhookEndpoint({
      name: String(formData.get("name") ?? ""),
      url: String(formData.get("url") ?? ""),
      event_types: readEventTypes(formData),
      is_active: formData.get("is_active") === "on",
    });
    revalidatePath("/webhooks");
    return {
      message: "webhook endpoint created",
      secret: result.secret,
      endpointId: result.id,
    };
  } catch (error) {
    return { error: actionError(error, "unable to create webhook endpoint") };
  }
}

export async function updateWebhookAction(
  id: string,
  _previousState: WebhookActionState,
  formData: FormData,
): Promise<WebhookActionState> {
  try {
    await updateWebhookEndpoint(id, {
      name: String(formData.get("name") ?? ""),
      url: String(formData.get("url") ?? ""),
      event_types: readEventTypes(formData),
      is_active: formData.get("is_active") === "on",
    });
    revalidatePath("/webhooks");
    revalidatePath(`/webhooks/${id}`);
    return { message: "endpoint settings saved" };
  } catch (error) {
    return { error: actionError(error, "unable to update webhook endpoint") };
  }
}

export async function rotateWebhookSecretAction(
  id: string,
  _previousState: WebhookActionState,
): Promise<WebhookActionState> {
  void _previousState;
  try {
    const result = await rotateWebhookSecret(id);
    return {
      message: "signing secret rotated",
      secret: result.secret,
      endpointId: id,
    };
  } catch (error) {
    return { error: actionError(error, "unable to rotate signing secret") };
  }
}

export async function deleteWebhookEndpointAction(id: string) {
  await deleteWebhookEndpoint(id);
  revalidatePath("/webhooks");
  redirect("/webhooks");
}

function readEventTypes(formData: FormData): WebhookEventType[] {
  const allowed = new Set<string>(WEBHOOK_EVENT_TYPES);
  return formData
    .getAll("event_types")
    .map(String)
    .filter((value): value is WebhookEventType => allowed.has(value));
}

function actionError(error: unknown, fallback: string) {
  return error instanceof QueueFlowAPIError ? error.message : fallback;
}
