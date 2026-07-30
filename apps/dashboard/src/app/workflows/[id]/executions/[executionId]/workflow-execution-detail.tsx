"use client";

import { useCallback, useEffect, useMemo, useRef, useState, useTransition } from "react";
import Link from "next/link";
import { StatusBadge } from "@/components/status-badge";
import {
  durationMilliseconds,
  executionNodeSummary,
  formatDuration,
  formatTimestamp,
  isExecutionTerminal,
  orderNodeExecutions,
} from "@/lib/workflow-execution-model";
import type {
  WorkflowExecution,
  WorkflowNodeExecution,
} from "@/lib/workflow-execution-types";
import type { WorkflowVersionDetail } from "@/lib/workflow-types";
import {
  cancelWorkflowExecutionAction,
  getWorkflowExecutionAction,
} from "../actions";

export function WorkflowExecutionDetail({
  initialExecution,
  version,
  workflowName,
}: {
  initialExecution: WorkflowExecution;
  version: WorkflowVersionDetail;
  workflowName: string;
}) {
  const [execution, setExecution] = useState(initialExecution);
  const [refreshError, setRefreshError] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [notice, setNotice] = useState("");
  const [pending, startTransition] = useTransition();
  const mounted = useRef(true);
  const refreshInFlight = useRef(false);
  const terminal = isExecutionTerminal(execution.status);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const refresh = useCallback(async () => {
    if (refreshInFlight.current) return;
    refreshInFlight.current = true;
    setRefreshing(true);
    const result = await getWorkflowExecutionAction(
      initialExecution.workflow_id,
      initialExecution.execution_id,
    );
    refreshInFlight.current = false;
    if (mounted.current) setRefreshing(false);
    if (!mounted.current) return;
    if (!result.ok) {
      setRefreshError(result.error);
      return;
    }
    setExecution(result.data);
    setRefreshError("");
  }, [initialExecution.execution_id, initialExecution.workflow_id]);

  useEffect(() => {
    if (terminal) return;
    const poll = () => {
      if (!document.hidden) void refresh();
    };
    const timer = window.setInterval(poll, 2000);
    const visibility = () => {
      if (!document.hidden) void refresh();
    };
    document.addEventListener("visibilitychange", visibility);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [refresh, terminal]);

  const definitionNodes = useMemo(
    () =>
      new Map(version.definition.nodes.map((node) => [node.id, node])),
    [version.definition.nodes],
  );
  const orderedNodes = useMemo(
    () => orderNodeExecutions(execution.nodes, version.definition),
    [execution.nodes, version.definition],
  );

  function cancelExecution() {
    if (
      !window.confirm(
        "Cancel this execution? External work already started may finish best-effort, but no new downstream nodes will be scheduled.",
      )
    ) {
      return;
    }
    startTransition(async () => {
      const result = await cancelWorkflowExecutionAction(
        execution.workflow_id,
        execution.execution_id,
      );
      if (!result.ok) {
        setRefreshError(result.error);
        return;
      }
      setNotice("execution cancelled; refreshing node state");
      await refresh();
    });
  }

  async function copy(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      setNotice(`${label} copied`);
    } catch {
      setRefreshError(`unable to copy ${label}`);
    }
  }

  return (
    <div>
      <div className="execution-breadcrumbs">
        <Link href={`/workflows/${execution.workflow_id}`}>workflow</Link>
        <span>/</span>
        <Link href={`/workflows/${execution.workflow_id}/executions`}>
          executions
        </Link>
        <span>/</span>
        <strong>{shortID(execution.execution_id)}</strong>
      </div>

      <div className="execution-detail-heading">
        <div>
          <p className="eyebrow">taskcanvas execution</p>
          <h1 className="task-id-title">{execution.execution_id}</h1>
          <div className="detail-subline">
            <StatusBadge status={execution.status} />
            <span>
              {workflowName} · immutable version {execution.workflow_version}
            </span>
          </div>
        </div>
        <div className="action-row">
          <button
            className="button button-quiet"
            onClick={() => void copy(execution.execution_id, "execution ID")}
            type="button"
          >
            copy execution ID
          </button>
          <button
            className="button button-secondary"
            disabled={pending || refreshing}
            onClick={() => void refresh()}
            type="button"
          >
            refresh
          </button>
          {!terminal && (
            <button
              className="button button-danger"
              disabled={pending}
              onClick={cancelExecution}
              type="button"
            >
              {pending ? "cancelling…" : "cancel execution"}
            </button>
          )}
        </div>
      </div>

      {(refreshError || notice) && (
        <p
          aria-live="polite"
          className={`editor-message ${refreshError ? "editor-message-error" : ""}`}
        >
          {refreshError || notice}
        </p>
      )}
      {!terminal && (
        <p className="execution-live-note" role="status">
          <span className="status-dot" />
          refreshing every 2 seconds while this execution is active
        </p>
      )}

      <section className="execution-metadata-grid">
        <MetadataCard
          items={[
            ["workflow", workflowName],
            ["workflow ID", execution.workflow_id],
            ["immutable version", `v${execution.workflow_version}`],
            ["status", execution.status],
          ]}
          title="execution"
        />
        <MetadataCard
          items={[
            ["created", formatTimestamp(execution.created_at)],
            ["started", formatTimestamp(execution.started_at)],
            ["completed", formatTimestamp(execution.completed_at)],
            [
              "duration",
              formatDuration(
                durationMilliseconds(
                  execution.started_at,
                  execution.completed_at,
                ),
              ),
            ],
          ]}
          title="timing"
        />
      </section>

      {(execution.error_code || execution.error_message) && (
        <section className="panel execution-error" role="alert">
          <p className="eyebrow">execution failure</p>
          <strong>{execution.error_code || "workflow_execution_failed"}</strong>
          <p>{execution.error_message || "execution failed"}</p>
        </section>
      )}

      <section className="execution-json-grid">
        <JSONViewer label="execution input" value={execution.input} />
        <JSONViewer label="execution output" value={execution.output} />
      </section>

      <section className="timeline-section">
        <div className="section-heading">
          <div>
            <p className="eyebrow">immutable version {version.version}</p>
            <h2>node execution timeline</h2>
          </div>
          <span className="tag">{orderedNodes.length} nodes</span>
        </div>
        <ol aria-label="node execution timeline" className="execution-timeline">
          {orderedNodes.map((node, index) => (
            <TimelineItem
              definitionNode={definitionNodes.get(node.node_id)}
              isLast={index === orderedNodes.length - 1}
              key={node.id || node.node_id}
              node={node}
            />
          ))}
        </ol>
      </section>

      <p className="cancellation-note">
        Cancellation prevents new downstream scheduling. QueueFlow tasks or
        EventForge deliveries already in flight may still finish externally.
      </p>
    </div>
  );
}

function TimelineItem({
  node,
  definitionNode,
  isLast,
}: {
  node: WorkflowNodeExecution;
  definitionNode: WorkflowVersionDetail["definition"]["nodes"][number] | undefined;
  isLast: boolean;
}) {
  const duration = formatDuration(
    durationMilliseconds(node.started_at, node.completed_at),
  );
  return (
    <li
      className={`timeline-item timeline-${node.status}`}
      tabIndex={0}
    >
      <div className="timeline-rail" aria-hidden="true">
        <span className="timeline-icon">{statusIcon(node.status)}</span>
        {!isLast && <span className="timeline-line" />}
      </div>
      <article className="panel timeline-card">
        <header>
          <div>
            <span className="timeline-type">{node.node_type}</span>
            <h3>{definitionNode?.name ?? node.node_id}</h3>
            <code>{node.node_id}</code>
          </div>
          <StatusBadge status={node.status} />
        </header>
        <p className="timeline-summary">
          {executionNodeSummary(node, definitionNode)}
        </p>
        <dl className="timeline-metadata">
          <div>
            <dt>attempt</dt>
            <dd>{node.attempt}</dd>
          </div>
          <div>
            <dt>started</dt>
            <dd>{formatTimestamp(node.started_at)}</dd>
          </div>
          <div>
            <dt>completed</dt>
            <dd>{formatTimestamp(node.completed_at)}</dd>
          </div>
          <div>
            <dt>duration</dt>
            <dd>{duration}</dd>
          </div>
        </dl>
        {node.queue_task_id && (
          <p className="timeline-reference">
            QueueFlow task <code>{node.queue_task_id}</code>
          </p>
        )}
        {node.webhook_delivery_id && (
          <p className="timeline-reference">
            EventForge delivery <code>{node.webhook_delivery_id}</code>
          </p>
        )}
        {(node.error_code || node.error_message) && (
          <div className="timeline-error" role="alert">
            <strong>{node.error_code || "node_execution_failed"}</strong>
            <p>{node.error_message || "node failed"}</p>
          </div>
        )}
        {(node.input !== undefined || node.output !== undefined) && (
          <details className="timeline-payload">
            <summary>node input / output</summary>
            {node.input !== undefined && (
              <pre>{JSON.stringify(node.input, null, 2)}</pre>
            )}
            {node.output !== undefined && (
              <pre>{JSON.stringify(node.output, null, 2)}</pre>
            )}
          </details>
        )}
      </article>
    </li>
  );
}

function MetadataCard({
  title,
  items,
}: {
  title: string;
  items: Array<[string, string]>;
}) {
  return (
    <article className="panel detail-card">
      <p className="eyebrow">{title}</p>
      <dl className="detail-list">
        {items.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </article>
  );
}

function JSONViewer({ label, value }: { label: string; value: unknown }) {
  const serialized =
    value === undefined ? "not available" : JSON.stringify(value, null, 2);
  return (
    <article className="panel code-panel execution-json">
      <header>
        <p className="eyebrow">{label}</p>
        <button
          aria-label={`copy ${label}`}
          className="text-button"
          onClick={() => void navigator.clipboard.writeText(serialized)}
          type="button"
        >
          copy
        </button>
      </header>
      <details open={serialized.length < 2000}>
        <summary>
          {serialized.length < 2000 ? "hide JSON" : "expand JSON"}
        </summary>
        <pre>{serialized}</pre>
      </details>
    </article>
  );
}

function statusIcon(status: WorkflowNodeExecution["status"]) {
  if (status === "succeeded") return "✓";
  if (status === "failed") return "!";
  if (status === "skipped") return "↷";
  if (status === "cancelled") return "×";
  if (status === "running") return "▶";
  if (status === "queued") return "↗";
  return "·";
}

function shortID(id: string) {
  return `${id.slice(0, 8)}…${id.slice(-4)}`;
}
