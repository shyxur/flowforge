import "server-only";

export type TaskStatus =
  | "pending"
  | "processing"
  | "completed"
  | "failed"
  | "dead_letter"
  | "cancelled";

export type QueueTask = {
  id: string;
  queue: string;
  payload: unknown;
  result?: unknown;
  status: TaskStatus;
  priority: number;
  attempts: number;
  max_attempts: number;
  locked_by?: string;
  created_at: string;
  updated_at: string;
  last_error?: string;
};

export type TaskPage = {
  items: QueueTask[];
  next_cursor?: string;
};

type APIErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
  };
};

export class QueueFlowAPIError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

function config() {
  const baseURL = process.env.QUEUEFLOW_API_BASE_URL?.replace(/\/$/, "");
  const apiKey = process.env.QUEUEFLOW_API_KEY;
  if (!baseURL || !apiKey) {
    throw new QueueFlowAPIError(
      "Dashboard API configuration is incomplete. Set QUEUEFLOW_API_BASE_URL and QUEUEFLOW_API_KEY.",
      500,
    );
  }
  return { baseURL, apiKey };
}

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const { baseURL, apiKey } = config();
  const response = await fetch(`${baseURL}${path}`, {
    ...init,
    cache: "no-store",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${apiKey}`,
      ...init?.headers,
    },
  });
  if (!response.ok) {
    let message = `QueueFlow API returned ${response.status}`;
    try {
      const body = (await response.json()) as APIErrorEnvelope;
      message = body.error?.message || message;
    } catch {
      // Preserve the status-based message for non-JSON upstream failures.
    }
    throw new QueueFlowAPIError(message, response.status);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export async function listTasks(filters: {
  queue?: string;
  status?: string;
}): Promise<TaskPage> {
  const query = new URLSearchParams({ limit: "100" });
  if (filters.queue) query.set("queue", filters.queue);
  if (filters.status) query.set("status", filters.status);
  const page = await apiFetch<TaskPage>(`/v1/tasks?${query}`);
  return { ...page, items: page.items ?? [] };
}

export function getTask(id: string): Promise<QueueTask> {
  return apiFetch<QueueTask>(`/v1/tasks/${encodeURIComponent(id)}`);
}

export function retryTask(id: string): Promise<QueueTask> {
  return apiFetch<QueueTask>(`/v1/tasks/${encodeURIComponent(id)}/retry`, {
    method: "POST",
  });
}

export function cancelTask(id: string): Promise<QueueTask> {
  return apiFetch<QueueTask>(`/v1/tasks/${encodeURIComponent(id)}/cancel`, {
    method: "POST",
  });
}

export function deleteTask(id: string): Promise<void> {
  return apiFetch<void>(`/v1/tasks/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function openTaskEventStream(signal: AbortSignal) {
  const { baseURL, apiKey } = config();
  return fetch(`${baseURL}/v1/events/tasks`, {
    cache: "no-store",
    headers: {
      Accept: "text/event-stream",
      Authorization: `Bearer ${apiKey}`,
    },
    signal,
  });
}
