import type {
  WorkflowExecutionStatus,
  WorkflowNodeExecution,
  WorkflowNodeExecutionStatus,
} from "./workflow-execution-types";
import type {
  WorkflowDefinition,
  WorkflowNode,
} from "./workflow-types";

export const EXECUTION_STATUS_LABELS: Record<WorkflowExecutionStatus, string> = {
  pending: "pending",
  running: "running",
  succeeded: "succeeded",
  failed: "failed",
  cancelled: "cancelled",
};

export const NODE_STATUS_LABELS: Record<WorkflowNodeExecutionStatus, string> = {
  pending: "pending",
  queued: "queued",
  running: "running",
  succeeded: "succeeded",
  failed: "failed",
  skipped: "skipped",
  cancelled: "cancelled",
};

export function isExecutionTerminal(status: WorkflowExecutionStatus) {
  return ["succeeded", "failed", "cancelled"].includes(status);
}

export function formatTimestamp(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return `${date.toISOString().slice(0, 10)} ${date
    .toISOString()
    .slice(11, 19)} UTC`;
}

export function durationMilliseconds(start?: string, end?: string) {
  if (!start || !end) return undefined;
  const duration = new Date(end).getTime() - new Date(start).getTime();
  return Number.isFinite(duration) && duration >= 0 ? duration : undefined;
}

export function formatDuration(milliseconds?: number) {
  if (milliseconds === undefined) return "—";
  if (milliseconds < 1000) return `${milliseconds} ms`;
  const seconds = milliseconds / 1000;
  if (seconds < 60) return `${trimDecimal(seconds)} s`;
  const minutes = Math.floor(seconds / 60);
  const remaining = Math.floor(seconds % 60);
  if (minutes < 60) return `${minutes}m ${remaining}s`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ${minutes % 60}m`;
}

export function topologicalNodeIDs(definition: WorkflowDefinition) {
  const nodeIDs = new Set(definition.nodes.map((node) => node.id));
  const incoming = new Map<string, number>();
  const outgoing = new Map<string, string[]>();
  for (const id of nodeIDs) {
    incoming.set(id, 0);
    outgoing.set(id, []);
  }
  for (const edge of definition.edges) {
    if (!nodeIDs.has(edge.from) || !nodeIDs.has(edge.to)) continue;
    outgoing.get(edge.from)?.push(edge.to);
    incoming.set(edge.to, (incoming.get(edge.to) ?? 0) + 1);
  }
  for (const targets of outgoing.values()) targets.sort();
  const ready = [...nodeIDs]
    .filter((id) => incoming.get(id) === 0)
    .sort();
  const ordered: string[] = [];
  while (ready.length) {
    const id = ready.shift()!;
    ordered.push(id);
    for (const target of outgoing.get(id) ?? []) {
      const next = (incoming.get(target) ?? 1) - 1;
      incoming.set(target, next);
      if (next === 0) {
        ready.push(target);
        ready.sort();
      }
    }
  }
  const unresolved = [...nodeIDs].filter((id) => !ordered.includes(id)).sort();
  return [...ordered, ...unresolved];
}

export function orderNodeExecutions(
  nodes: WorkflowNodeExecution[],
  definition: WorkflowDefinition,
) {
  const rank = new Map(
    topologicalNodeIDs(definition).map((id, index) => [id, index]),
  );
  return [...nodes].sort((left, right) => {
    const leftRank = rank.get(left.node_id) ?? Number.MAX_SAFE_INTEGER;
    const rightRank = rank.get(right.node_id) ?? Number.MAX_SAFE_INTEGER;
    return leftRank - rightRank || left.node_id.localeCompare(right.node_id);
  });
}

export function executionNodeSummary(
  execution: WorkflowNodeExecution,
  definitionNode?: WorkflowNode,
) {
  const config = definitionNode?.config as Record<string, unknown> | undefined;
  switch (definitionNode?.type ?? execution.node_type) {
    case "trigger":
      return "execution input received";
    case "task":
      return [
        config?.queue ? `queue ${config.queue}` : undefined,
        execution.queue_task_id ? `task ${execution.queue_task_id}` : undefined,
        `attempt ${execution.attempt}`,
      ]
        .filter(Boolean)
        .join(" · ");
    case "webhook":
      return execution.webhook_delivery_id
        ? `delivery ${execution.webhook_delivery_id}`
        : "EventForge delivery";
    case "delay":
      return [
        `${String(config?.duration_seconds ?? 0)}s delay`,
        execution.available_at
          ? `available ${formatTimestamp(execution.available_at)}`
          : undefined,
      ]
        .filter(Boolean)
        .join(" · ");
    case "condition":
      return `${String(config?.field ?? "input.…")} ${String(
        config?.operator ?? "equals",
      )}`;
  }
}

function trimDecimal(value: number) {
  return value.toFixed(value < 10 ? 1 : 0).replace(/\.0$/, "");
}
