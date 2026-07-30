export type WorkflowStatus = "draft" | "published" | "archived";
export type WorkflowNodeType =
  | "trigger"
  | "task"
  | "webhook"
  | "delay"
  | "condition";

export type WorkflowPosition = { x: number; y: number };

export type TriggerConfig = { description?: string };
export type TaskConfig = {
  queue: string;
  payload?: unknown;
  priority?: number;
  max_retries?: number;
  timeout_seconds?: number;
};
export type WebhookConfig = { endpoint_id: string; payload?: unknown };
export type DelayConfig = { duration_seconds: number };
export type ConditionConfig = {
  field: string;
  operator: "equals" | "not_equals" | "exists";
  value?: unknown;
};

export type WorkflowNodeConfig =
  | TriggerConfig
  | TaskConfig
  | WebhookConfig
  | DelayConfig
  | ConditionConfig;

export type WorkflowNode = {
  id: string;
  type: WorkflowNodeType;
  name: string;
  position?: WorkflowPosition;
  config: WorkflowNodeConfig;
};

export type WorkflowEdge = {
  id: string;
  from: string;
  to: string;
  condition: { branch: boolean } | null;
};

export type WorkflowDefinition = {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
};

export type Workflow = {
  id: string;
  org_id: string;
  name: string;
  slug: string;
  description: string | null;
  status: WorkflowStatus;
  definition: WorkflowDefinition;
  created_at: string;
  updated_at: string;
};

export type WorkflowPage = {
  items: Workflow[];
  next_cursor?: string;
};

export type WorkflowValidationError = {
  code: string;
  message: string;
  path?: string;
};

export type WorkflowValidationResult = {
  valid: boolean;
  errors: WorkflowValidationError[];
};

export type WorkflowVersionSummary = {
  version: number;
  version_id: string;
  status: "published" | "deprecated";
  published_at: string;
  name: string;
  slug: string;
};

export type WorkflowVersionDetail = WorkflowVersionSummary & {
  workflow_id: string;
  description: string | null;
  definition: WorkflowDefinition;
  created_at: string;
};

export type WorkflowPublishResult = {
  workflow_id: string;
  version: number;
  version_id: string;
  status: "published";
  published_at: string;
};

export type EditorNodeData = {
  workflowNode: WorkflowNode;
  validationMessages: string[];
};

export type EditorNode = {
  id: string;
  type: "workflow";
  position: WorkflowPosition;
  data: EditorNodeData;
};

export type EditorEdge = {
  id: string;
  source: string;
  target: string;
  sourceHandle?: "true" | "false";
  label?: string;
  condition: { branch: boolean } | null;
  data?: { validationMessages?: string[] };
};
