export const WEBHOOK_EVENT_TYPES = [
  "task.created",
  "task.processing",
  "task.completed",
  "task.failed",
  "task.dead_letter",
  "task.cancelled",
] as const;

export type WebhookEventType = (typeof WEBHOOK_EVENT_TYPES)[number];

export type WebhookEndpoint = {
  id: string;
  org_id: string;
  name: string;
  url: string;
  event_types: WebhookEventType[];
  is_active: boolean;
  created_at: string;
  updated_at: string;
};

export type WebhookEndpointCreateResult = WebhookEndpoint & {
  secret: string;
};

export type WebhookDeliveryStatus =
  | "pending"
  | "delivering"
  | "delivered"
  | "retrying"
  | "failed";

export type WebhookDelivery = {
  id: string;
  endpoint_id: string;
  event_type: WebhookEventType;
  status: WebhookDeliveryStatus;
  attempt_count: number;
  max_attempts: number;
  next_attempt_at?: string;
  last_attempt_at?: string;
  response_status?: number;
  response_body?: string;
  last_error?: string;
  payload?: unknown;
  created_at: string;
  updated_at: string;
};

export type WebhookDeliveryPage = {
  items: WebhookDelivery[];
  next_cursor?: string;
};
