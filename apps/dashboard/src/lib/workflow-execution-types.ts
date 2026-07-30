export type WorkflowExecutionStatus =
  | "pending"
  | "running"
  | "succeeded"
  | "failed"
  | "cancelled";

export type WorkflowNodeExecutionStatus =
  | "pending"
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "skipped"
  | "cancelled";

export type WorkflowExecutionSummary = {
  execution_id: string;
  org_id: string;
  workflow_id: string;
  workflow_version_id: string;
  workflow_version: number;
  status: WorkflowExecutionStatus;
  input?: unknown;
  output?: unknown;
  error_code?: string;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
};

export type WorkflowNodeExecution = {
  id: string;
  org_id: string;
  workflow_execution_id: string;
  node_id: string;
  node_type: "trigger" | "task" | "webhook" | "delay" | "condition";
  status: WorkflowNodeExecutionStatus;
  attempt: number;
  input?: unknown;
  output?: unknown;
  error_code?: string;
  error_message?: string;
  queue_task_id?: string;
  webhook_delivery_id?: string;
  available_at?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
};

export type WorkflowExecution = WorkflowExecutionSummary & {
  nodes: WorkflowNodeExecution[];
};

export type WorkflowExecutionPage = {
  items: WorkflowExecutionSummary[];
  next_cursor?: string;
};

export type StartWorkflowExecutionRequest = {
  version?: number;
  input?: unknown;
};

export type StartWorkflowExecutionResponse = {
  execution_id: string;
  workflow_id: string;
  workflow_version: number;
  status: WorkflowExecutionStatus;
  created_at: string;
};
