import type {
  EditorEdge,
  EditorNode,
  WorkflowDefinition,
  WorkflowEdge,
  WorkflowNode,
  WorkflowNodeConfig,
  WorkflowNodeType,
  WorkflowValidationError,
} from "./workflow-types";

export const NODE_LABELS: Record<WorkflowNodeType, string> = {
  trigger: "trigger",
  task: "task",
  webhook: "webhook",
  delay: "delay",
  condition: "condition",
};

export function fallbackPosition(index: number) {
  return { x: 64 + (index % 3) * 270, y: 64 + Math.floor(index / 3) * 170 };
}

export function definitionToEditor(
  definition: WorkflowDefinition,
  validationErrors: WorkflowValidationError[] = [],
): { nodes: EditorNode[]; edges: EditorEdge[] } {
  return {
    nodes: definition.nodes.map((workflowNode, index) => ({
      id: workflowNode.id,
      type: "workflow",
      position: workflowNode.position ?? fallbackPosition(index),
      data: {
        workflowNode: structuredClone(workflowNode),
        validationMessages: messagesForPath(validationErrors, `nodes[${index}]`),
      },
    })),
    edges: definition.edges.map((edge, index) => {
      const branch = edge.condition?.branch;
      return {
        id: edge.id,
        source: edge.from,
        target: edge.to,
        sourceHandle:
          typeof branch === "boolean" ? (branch ? "true" : "false") : undefined,
        label: typeof branch === "boolean" ? String(branch) : undefined,
        condition:
          typeof branch === "boolean" ? { branch } : null,
        data: {
          validationMessages: messagesForPath(
            validationErrors,
            `edges[${index}]`,
          ),
        },
      };
    }),
  };
}

export function editorToDefinition(
  nodes: EditorNode[],
  edges: EditorEdge[],
): WorkflowDefinition {
  return {
    nodes: nodes.map(({ position, data }) => ({
      ...structuredClone(data.workflowNode),
      position: { x: Math.round(position.x), y: Math.round(position.y) },
    })),
    edges: edges.map((edge): WorkflowEdge => ({
      id: edge.id,
      from: edge.source,
      to: edge.target,
      condition: edge.condition ? { branch: edge.condition.branch } : null,
    })),
  };
}

export function createWorkflowNode(
  type: WorkflowNodeType,
  position: { x: number; y: number },
  existingIDs: Set<string>,
): EditorNode {
  const base = type;
  let id = `${base}-${crypto.randomUUID().slice(0, 8)}`;
  while (existingIDs.has(id)) id = `${base}-${crypto.randomUUID().slice(0, 8)}`;
  const workflowNode: WorkflowNode = {
    id,
    type,
    name: defaultLabel(type),
    position,
    config: defaultConfig(type),
  };
  return {
    id,
    type: "workflow",
    position,
    data: { workflowNode, validationMessages: [] },
  };
}

export function canConnect(
  source: string,
  target: string,
  nodes: EditorNode[],
  edges: EditorEdge[],
): string | null {
  if (!source || !target) return "choose a source and target";
  if (source === target) return "self-loop connections are not allowed";
  if (edges.some((edge) => edge.source === source && edge.target === target)) {
    return "this connection already exists";
  }
  const targetNode = nodes.find((node) => node.id === target);
  if (targetNode?.data.workflowNode.type === "trigger") {
    return "trigger nodes cannot have incoming connections";
  }
  return null;
}

export function validateDraft(
  definition: WorkflowDefinition,
): Record<string, string> {
  const errors: Record<string, string> = {};
  const seen = new Set<string>();
  for (const node of definition.nodes) {
    if (!node.id.trim()) errors[node.id] = "node id is required";
    if (seen.has(node.id)) errors[node.id] = "node ids must be unique";
    seen.add(node.id);
    if (!node.name.trim()) errors[node.id] = "label is required";
    const configError = validateConfig(node.type, node.config);
    if (configError) errors[node.id] = configError;
  }
  const pairs = new Set<string>();
  for (const edge of definition.edges) {
    const pair = `${edge.from}\u0000${edge.to}`;
    if (edge.from === edge.to) errors[edge.id] = "self-loops are not allowed";
    if (pairs.has(pair)) errors[edge.id] = "duplicate connection";
    pairs.add(pair);
  }
  return errors;
}

export function configSummary(node: WorkflowNode) {
  const config = node.config as Record<string, unknown>;
  switch (node.type) {
    case "task":
      return String(config.queue || "queue not set");
    case "webhook":
      return config.endpoint_id ? "endpoint selected" : "endpoint not set";
    case "delay":
      return `${config.duration_seconds ?? 0}s delay`;
    case "condition":
      return `${config.field || "input.…"} ${config.operator || "equals"}`;
    default:
      return String(config.description || "workflow input");
  }
}

function validateConfig(type: WorkflowNodeType, config: WorkflowNodeConfig) {
  const value = config as Record<string, unknown>;
  if (type === "task" && !String(value.queue ?? "").trim()) {
    return "task queue is required";
  }
  if (type === "webhook" && !String(value.endpoint_id ?? "").trim()) {
    return "webhook endpoint is required";
  }
  if (type === "delay") {
    const seconds = Number(value.duration_seconds);
    if (!Number.isInteger(seconds) || seconds < 0 || seconds > 604800) {
      return "delay must be between 0 and 604800 seconds";
    }
  }
  if (type === "condition") {
    const field = String(value.field ?? "");
    const operator = String(value.operator ?? "");
    if (!field.startsWith("input.")) return "condition field must begin with input.";
    if (!["equals", "not_equals", "exists"].includes(operator)) {
      return "condition operator is invalid";
    }
    if (operator !== "exists" && value.value === undefined) {
      return "condition value is required";
    }
  }
  return "";
}

function defaultLabel(type: WorkflowNodeType) {
  return {
    trigger: "New trigger",
    task: "New task",
    webhook: "New webhook",
    delay: "New delay",
    condition: "New condition",
  }[type];
}

function defaultConfig(type: WorkflowNodeType): WorkflowNodeConfig {
  switch (type) {
    case "task":
      return { queue: "" };
    case "webhook":
      return { endpoint_id: "" };
    case "delay":
      return { duration_seconds: 60 };
    case "condition":
      return { field: "input.", operator: "equals", value: "" };
    default:
      return {};
  }
}

function messagesForPath(errors: WorkflowValidationError[], prefix: string) {
  return errors
    .filter((error) => error.path?.startsWith(prefix))
    .map((error) => error.message);
}
