"use client";

import {
  memo,
  useCallback,
  useEffect,
  useMemo,
  useState,
  useTransition,
} from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Background,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  type Connection,
  type Edge,
  type Node,
  type NodeProps,
  useEdgesState,
  useNodesState,
} from "@xyflow/react";
import type { WebhookEndpoint } from "@/lib/webhook-types";
import {
  canConnect,
  configSummary,
  createWorkflowNode,
  definitionToEditor,
  editorToDefinition,
  NODE_LABELS,
  validateDraft,
} from "@/lib/workflow-editor-model";
import type {
  EditorEdge,
  EditorNodeData,
  Workflow,
  WorkflowNode,
  WorkflowNodeConfig,
  WorkflowNodeType,
  WorkflowPublishResult,
  WorkflowValidationResult,
  WorkflowVersionDetail,
  WorkflowVersionSummary,
} from "@/lib/workflow-types";
import {
  deleteWorkflowAction,
  getWorkflowVersionAction,
  publishWorkflowAction,
  saveWorkflowAction,
  validateWorkflowAction,
} from "../actions";

type FlowNode = Node<EditorNodeData, "workflow">;
type FlowEdgeData = {
  condition: { branch: boolean } | null;
  validationMessages?: string[];
};
type FlowEdge = Edge<FlowEdgeData>;

const nodeTypes = { workflow: memo(WorkflowNodeCard) };
const nodeKinds = Object.keys(NODE_LABELS) as WorkflowNodeType[];

export function WorkflowEditor({
  initialWorkflow,
  webhookEndpoints,
  initialVersions,
  created,
}: {
  initialWorkflow: Workflow;
  webhookEndpoints: WebhookEndpoint[];
  initialVersions: WorkflowVersionSummary[];
  created: boolean;
}) {
  const router = useRouter();
  const mapped = useMemo(
    () => definitionToEditor(initialWorkflow.definition),
    [initialWorkflow.definition],
  );
  const [nodes, setNodes, onNodesChangeBase] = useNodesState<FlowNode>(
    mapped.nodes as FlowNode[],
  );
  const [edges, setEdges, onEdgesChangeBase] = useEdgesState<FlowEdge>(
    mapped.edges.map((edge) => toFlowEdge(edge)),
  );
  const [workflow, setWorkflow] = useState(initialWorkflow);
  const [name, setName] = useState(initialWorkflow.name);
  const [description, setDescription] = useState(
    initialWorkflow.description ?? "",
  );
  const [selectedNodeID, setSelectedNodeID] = useState<string>();
  const [selectedEdgeID, setSelectedEdgeID] = useState<string>();
  const [dirty, setDirty] = useState(false);
  const [validation, setValidation] = useState<WorkflowValidationResult>();
  const [notice, setNotice] = useState(
    created ? "workflow created — add an action to make it publishable" : "",
  );
  const [error, setError] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [jsonErrors, setJSONErrors] = useState<Record<string, string>>({});
  const [versions, setVersions] = useState(initialVersions);
  const [versionDetail, setVersionDetail] = useState<WorkflowVersionDetail>();
  const [versionsOpen, setVersionsOpen] = useState(false);
  const [pending, startTransition] = useTransition();
  const readOnly = workflow.status === "archived";

  useEffect(() => {
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (!dirty) return;
      event.preventDefault();
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => window.removeEventListener("beforeunload", beforeUnload);
  }, [dirty]);

  const markDirty = useCallback(() => {
    setDirty(true);
    setValidation(undefined);
    setNotice("");
  }, []);

  const onNodesChange = useCallback(
    (changes: Parameters<typeof onNodesChangeBase>[0]) => {
      if (
        changes.some(
          (change) =>
            change.type === "remove" ||
            (change.type === "position" && change.dragging !== undefined),
        )
      ) {
        markDirty();
      }
      onNodesChangeBase(changes);
    },
    [markDirty, onNodesChangeBase],
  );

  const onEdgesChange = useCallback(
    (changes: Parameters<typeof onEdgesChangeBase>[0]) => {
      if (changes.some((change) => change.type === "remove")) markDirty();
      onEdgesChangeBase(changes);
    },
    [markDirty, onEdgesChangeBase],
  );

  const onConnect = useCallback(
    (connection: Connection) => {
      if (readOnly || !connection.source || !connection.target) return;
      const connectionError = canConnect(
        connection.source,
        connection.target,
        nodes as never,
        edges.map(fromFlowEdge),
      );
      if (connectionError) {
        setError(connectionError);
        return;
      }
      const source = nodes.find((node) => node.id === connection.source);
      const isCondition = source?.data.workflowNode.type === "condition";
      const branch = isCondition ? connection.sourceHandle === "true" : undefined;
      const edge: FlowEdge = {
        id: `edge-${crypto.randomUUID().slice(0, 12)}`,
        source: connection.source,
        target: connection.target,
        sourceHandle: connection.sourceHandle ?? undefined,
        label: isCondition ? String(branch) : undefined,
        markerEnd: { type: MarkerType.ArrowClosed },
        data: {
          condition:
            typeof branch === "boolean" ? { branch } : null,
        },
      };
      setEdges((current) => [...current, edge]);
      setError("");
      markDirty();
    },
    [edges, markDirty, nodes, readOnly, setEdges],
  );

  function addNode(type: WorkflowNodeType) {
    if (readOnly) return;
    const node = createWorkflowNode(
      type,
      { x: 100 + (nodes.length % 3) * 240, y: 100 + nodes.length * 36 },
      new Set(nodes.map(({ id }) => id)),
    ) as FlowNode;
    setNodes((current) => [...current, node]);
    setSelectedNodeID(node.id);
    setSelectedEdgeID(undefined);
    markDirty();
  }

  function updateNode(next: WorkflowNode) {
    setNodes((current) =>
      current.map((node) =>
        node.id === next.id
          ? { ...node, data: { ...node.data, workflowNode: next } }
          : node,
      ),
    );
    setFieldErrors((current) => {
      const nextErrors = { ...current };
      delete nextErrors[next.id];
      return nextErrors;
    });
    markDirty();
  }

  function deleteSelectedNode() {
    if (!selectedNodeID || readOnly) return;
    if (!window.confirm("Delete this node and its connected edges?")) return;
    setNodes((current) => current.filter((node) => node.id !== selectedNodeID));
    setEdges((current) =>
      current.filter(
        (edge) =>
          edge.source !== selectedNodeID && edge.target !== selectedNodeID,
      ),
    );
    setSelectedNodeID(undefined);
    markDirty();
  }

  function deleteSelectedEdge() {
    if (!selectedEdgeID || readOnly) return;
    setEdges((current) => current.filter((edge) => edge.id !== selectedEdgeID));
    setSelectedEdgeID(undefined);
    markDirty();
  }

  function updateSelectedEdgeBranch(branch: boolean) {
    setEdges((current) =>
      current.map((edge) =>
        edge.id === selectedEdgeID
          ? {
              ...edge,
              sourceHandle: branch ? "true" : "false",
              label: String(branch),
              data: { ...edge.data, condition: { branch } },
            }
          : edge,
      ),
    );
    markDirty();
  }

  const buildDefinition = useCallback(
    () =>
      editorToDefinition(
        nodes as never,
        edges.map(fromFlowEdge),
      ),
    [edges, nodes],
  );

  async function save() {
    if (readOnly || !dirty) return true;
    if (Object.keys(jsonErrors).length) {
      setError("fix node configuration errors before saving");
      const first = Object.keys(jsonErrors)[0];
      if (nodes.some((node) => node.id === first)) setSelectedNodeID(first);
      return false;
    }
    const definition = buildDefinition();
    const localErrors = validateDraft(definition);
    if (Object.keys(localErrors).length) {
      setFieldErrors(localErrors);
      setError("fix node configuration errors before saving");
      const first = Object.keys(localErrors)[0];
      if (nodes.some((node) => node.id === first)) setSelectedNodeID(first);
      return false;
    }
    const result = await saveWorkflowAction({
      id: workflow.id,
      name,
      description,
      definition,
    });
    if (!result.ok) {
      setError(result.error);
      return false;
    }
    setWorkflow(result.data);
    setDirty(false);
    setError("");
    setNotice(
      initialWorkflow.status === "published"
        ? "saved as a new draft; published versions remain immutable"
        : "draft saved",
    );
    return true;
  }

  function runSave() {
    startTransition(() => void save());
  }

  function runValidation() {
    startTransition(async () => {
      if (!(await save())) return;
      const result = await validateWorkflowAction(workflow.id);
      if (!result.ok) {
        setError(result.error);
        return;
      }
      applyValidation(result.data);
      setNotice(result.data.valid ? "workflow is valid and ready to publish" : "");
    });
  }

  function runPublish() {
    if (!window.confirm("Publish an immutable workflow version?")) return;
    startTransition(async () => {
      if (!(await save())) return;
      const checked = await validateWorkflowAction(workflow.id);
      if (!checked.ok) {
        setError(checked.error);
        return;
      }
      applyValidation(checked.data);
      if (!checked.data.valid) return;
      const result = await publishWorkflowAction(workflow.id);
      if (!result.ok) {
        setError(result.error);
        return;
      }
      if (isValidationResult(result.data)) {
        applyValidation(result.data);
        return;
      }
      const published = result.data as WorkflowPublishResult;
      setWorkflow((current) => ({ ...current, status: "published" }));
      setVersions((current) => [
        {
          version: published.version,
          version_id: published.version_id,
          status: published.status,
          published_at: published.published_at,
          name,
          slug: workflow.slug,
        },
        ...current,
      ]);
      setNotice(
        `published version ${published.version} at ${formatDate(published.published_at)}`,
      );
      setError("");
    });
  }

  function applyValidation(result: WorkflowValidationResult) {
    setValidation(result);
    const next = definitionToEditor(buildDefinition(), result.errors);
    setNodes(next.nodes as FlowNode[]);
    setEdges(next.edges.map(toFlowEdge));
    setError(result.valid ? "" : "workflow needs attention before publishing");
  }

  function removeWorkflow() {
    if (!window.confirm("Delete this workflow? Published snapshots will no longer be accessible.")) {
      return;
    }
    startTransition(async () => {
      const result = await deleteWorkflowAction(workflow.id);
      if (!result.ok) {
        setError(result.error);
        return;
      }
      router.push("/workflows");
      router.refresh();
    });
  }

  function leaveEditor() {
    if (dirty && !window.confirm("Leave without saving your draft changes?")) return;
    router.push("/workflows");
  }

  const selectedNode = nodes.find((node) => node.id === selectedNodeID);
  const selectedEdge = edges.find((edge) => edge.id === selectedEdgeID);
  const selectedEdgeSource = nodes.find(
    (node) => node.id === selectedEdge?.source,
  );

  return (
    <div className="workflow-editor">
      <header className="workflow-toolbar">
        <div className="workflow-toolbar-title">
          <button className="text-button" onClick={leaveEditor} type="button">
            ← workflows
          </button>
          <div>
            <div className="workflow-title-line">
              <h1>{name}</h1>
              <span className={`task-status status-${workflow.status}`}>
                <span />
                {workflow.status}
              </span>
              {dirty && <span className="dirty-note">unsaved</span>}
            </div>
            <small>{workflow.slug}</small>
          </div>
        </div>
        <div className="workflow-toolbar-actions">
          <button
            className="button button-quiet"
            onClick={() => setVersionsOpen((open) => !open)}
            type="button"
          >
            versions {versions.length > 0 ? `(${versions.length})` : ""}
          </button>
          <button
            className="button button-secondary"
            disabled={pending || readOnly || !dirty}
            onClick={runSave}
            type="button"
          >
            {pending && dirty ? "saving…" : "save"}
          </button>
          <button
            className="button button-secondary"
            disabled={pending || readOnly}
            onClick={runValidation}
            type="button"
          >
            validate
          </button>
          <button
            className="button button-primary"
            disabled={pending || readOnly}
            onClick={runPublish}
            type="button"
          >
            {pending ? "working…" : "publish"}
          </button>
        </div>
      </header>

      {workflow.status === "published" && (
        <p className="editor-banner">
          version {versions[0]?.version ?? "—"} is published. Saving changes
          creates a new draft while the published snapshot stays immutable.
        </p>
      )}
      {readOnly && (
        <p className="editor-banner">
          archived workflows are read-only and cannot be published.
        </p>
      )}
      {(notice || error) && (
        <div
          aria-live="polite"
          className={`editor-message ${error ? "editor-message-error" : ""}`}
        >
          {error || notice}
        </div>
      )}

      <div className="workflow-editor-grid">
        <section className="workflow-canvas-region">
          <div aria-label="add workflow node" className="node-palette">
            <span>add node</span>
            {nodeKinds.map((kind) => (
              <button
                disabled={readOnly}
                key={kind}
                onClick={() => addNode(kind)}
                type="button"
              >
                <NodeIcon type={kind} />
                {NODE_LABELS[kind]}
              </button>
            ))}
          </div>
          <div className="workflow-canvas">
            <ReactFlow
              colorMode="dark"
              deleteKeyCode={readOnly ? null : ["Backspace", "Delete"]}
              edges={edges}
              fitView
              nodeTypes={nodeTypes}
              nodes={nodes}
              nodesDraggable={!readOnly}
              nodesConnectable={!readOnly}
              onConnect={onConnect}
              onEdgesChange={onEdgesChange}
              onEdgeClick={(_, edge) => {
                setSelectedEdgeID(edge.id);
                setSelectedNodeID(undefined);
              }}
              onNodesChange={onNodesChange}
              onNodeClick={(_, node) => {
                setSelectedNodeID(node.id);
                setSelectedEdgeID(undefined);
              }}
              onPaneClick={() => {
                setSelectedNodeID(undefined);
                setSelectedEdgeID(undefined);
              }}
              proOptions={{ hideAttribution: true }}
            >
              <Background color="#404040" gap={24} size={1} />
              <Controls showInteractive={!readOnly} />
              <MiniMap
                maskColor="rgba(13,13,13,.75)"
                nodeColor="#bfbfbf"
                pannable
                zoomable
              />
            </ReactFlow>
          </div>
        </section>

        <aside aria-label="workflow configuration" className="workflow-config">
          {selectedNode ? (
            <NodeConfiguration
              error={fieldErrors[selectedNode.id]}
              node={selectedNode.data.workflowNode}
              onDelete={deleteSelectedNode}
              onError={(message) =>
                setJSONErrors((current) => {
                  const next = { ...current };
                  if (message) next[selectedNode.id] = message;
                  else delete next[selectedNode.id];
                  return next;
                })
              }
              onUpdate={updateNode}
              readOnly={readOnly}
              webhookEndpoints={webhookEndpoints}
            />
          ) : selectedEdge ? (
            <EdgeConfiguration
              edge={selectedEdge}
              isCondition={
                selectedEdgeSource?.data.workflowNode.type === "condition"
              }
              onBranchChange={updateSelectedEdgeBranch}
              onDelete={deleteSelectedEdge}
              readOnly={readOnly}
            />
          ) : (
            <WorkflowConfiguration
              description={description}
              name={name}
              onChange={(nextName, nextDescription) => {
                setName(nextName);
                setDescription(nextDescription);
                markDirty();
              }}
              onDelete={removeWorkflow}
              readOnly={readOnly}
            />
          )}

          {validation && (
            <ValidationPanel
              onSelect={(path) => {
                const nodeMatch = path.match(/^nodes\[(\d+)]/);
                const edgeMatch = path.match(/^edges\[(\d+)]/);
                if (nodeMatch) {
                  setSelectedNodeID(nodes[Number(nodeMatch[1])]?.id);
                  setSelectedEdgeID(undefined);
                } else if (edgeMatch) {
                  setSelectedEdgeID(edges[Number(edgeMatch[1])]?.id);
                  setSelectedNodeID(undefined);
                }
              }}
              result={validation}
            />
          )}
        </aside>
      </div>

      {versionsOpen && (
        <VersionsPanel
          detail={versionDetail}
          onClose={() => setVersionsOpen(false)}
          onSelect={(version) =>
            startTransition(async () => {
              const result = await getWorkflowVersionAction(workflow.id, version);
              if (result.ok) setVersionDetail(result.data);
              else setError(result.error);
            })
          }
          pending={pending}
          versions={versions}
        />
      )}
    </div>
  );
}

function WorkflowNodeCard({ data, selected }: NodeProps<FlowNode>) {
  const node = data.workflowNode;
  const condition = node.type === "condition";
  return (
    <div
      className={`canvas-node canvas-node-${node.type} ${selected ? "is-selected" : ""} ${
        data.validationMessages.length ? "has-error" : ""
      }`}
    >
      {node.type !== "trigger" && (
        <Handle aria-label="input connection" position={Position.Left} type="target" />
      )}
      <div className="canvas-node-heading">
        <NodeIcon type={node.type} />
        <span>{node.type}</span>
      </div>
      <strong>{node.name}</strong>
      <small>{configSummary(node)}</small>
      {data.validationMessages.length > 0 && (
        <span className="node-error-count">
          {data.validationMessages.length} issue
          {data.validationMessages.length === 1 ? "" : "s"}
        </span>
      )}
      {condition ? (
        <>
          <Handle
            aria-label="false branch"
            id="false"
            position={Position.Right}
            style={{ top: "38%" }}
            type="source"
          />
          <span className="branch-label branch-label-false">false</span>
          <Handle
            aria-label="true branch"
            id="true"
            position={Position.Right}
            style={{ top: "72%" }}
            type="source"
          />
          <span className="branch-label branch-label-true">true</span>
        </>
      ) : (
        <Handle aria-label="output connection" position={Position.Right} type="source" />
      )}
    </div>
  );
}

function NodeConfiguration({
  node,
  webhookEndpoints,
  readOnly,
  error,
  onUpdate,
  onError,
  onDelete,
}: {
  node: WorkflowNode;
  webhookEndpoints: WebhookEndpoint[];
  readOnly: boolean;
  error?: string;
  onUpdate: (node: WorkflowNode) => void;
  onError: (message: string) => void;
  onDelete: () => void;
}) {
  const [jsonError, setJSONError] = useState("");
  const [durationUnit, setDurationUnit] = useState("seconds");
  const config = node.config as Record<string, unknown>;
  const updateConfig = (patch: Record<string, unknown>) =>
    onUpdate({ ...node, config: { ...config, ...patch } as WorkflowNodeConfig });

  function updateJSON(value: string) {
    if (!value.trim()) {
      setJSONError("");
      onError("");
      const next = { ...config };
      delete next.payload;
      onUpdate({ ...node, config: next as WorkflowNodeConfig });
      return;
    }
    try {
      updateConfig({ payload: JSON.parse(value) });
      setJSONError("");
      onError("");
    } catch {
      setJSONError("payload must be valid JSON");
      onError("payload must be valid JSON");
    }
  }

  const multiplier =
    durationUnit === "days"
      ? 86400
      : durationUnit === "hours"
        ? 3600
        : durationUnit === "minutes"
          ? 60
          : 1;

  return (
    <div>
      <p className="eyebrow">selected node</p>
      <h2>{node.type} settings</h2>
      <div className="config-form">
        <label>
          <span>label</span>
          <input
            disabled={readOnly}
            onChange={(event) => onUpdate({ ...node, name: event.target.value })}
            value={node.name}
          />
        </label>
        {node.type === "trigger" && (
          <label>
            <span>description · optional</span>
            <textarea
              disabled={readOnly}
              onChange={(event) => updateConfig({ description: event.target.value })}
              value={String(config.description ?? "")}
            />
          </label>
        )}
        {node.type === "task" && (
          <>
            <label>
              <span>queue</span>
              <input
                disabled={readOnly}
                onChange={(event) => updateConfig({ queue: event.target.value })}
                required
                value={String(config.queue ?? "")}
              />
            </label>
            <JSONField
              key={`${node.id}-task-payload`}
              disabled={readOnly}
              error={jsonError}
              onChange={updateJSON}
              value={config.payload}
            />
            <NumberField
              label="priority · optional"
              max={100}
              min={-100}
              onChange={(value) => updateConfig({ priority: value })}
              readOnly={readOnly}
              value={config.priority}
            />
            <NumberField
              label="max retries · optional"
              min={0}
              onChange={(value) => updateConfig({ max_retries: value })}
              readOnly={readOnly}
              value={config.max_retries}
            />
            <NumberField
              label="timeout seconds · optional"
              min={1}
              onChange={(value) => updateConfig({ timeout_seconds: value })}
              readOnly={readOnly}
              value={config.timeout_seconds}
            />
          </>
        )}
        {node.type === "webhook" && (
          <>
            <label>
              <span>active endpoint</span>
              <select
                disabled={readOnly}
                onChange={(event) =>
                  updateConfig({ endpoint_id: event.target.value })
                }
                required
                value={String(config.endpoint_id ?? "")}
              >
                <option value="">select an endpoint</option>
                {webhookEndpoints.map((endpoint) => (
                  <option key={endpoint.id} value={endpoint.id}>
                    {endpoint.name} · {safeDestination(endpoint.url)}
                  </option>
                ))}
              </select>
            </label>
            {webhookEndpoints.length === 0 && (
              <p className="config-help">
                no active endpoints. <Link href="/webhooks/new">create a webhook</Link>.
              </p>
            )}
            <JSONField
              key={`${node.id}-webhook-payload`}
              disabled={readOnly}
              error={jsonError}
              onChange={updateJSON}
              value={config.payload}
            />
          </>
        )}
        {node.type === "delay" && (
          <>
            <div className="duration-fields">
              <label>
                <span>duration</span>
                <input
                  disabled={readOnly}
                  min={0}
                  onChange={(event) =>
                    updateConfig({
                      duration_seconds: Number(event.target.value) * multiplier,
                    })
                  }
                  type="number"
                  value={Number(config.duration_seconds ?? 0) / multiplier}
                />
              </label>
              <label>
                <span>unit</span>
                <select
                  disabled={readOnly}
                  onChange={(event) => setDurationUnit(event.target.value)}
                  value={durationUnit}
                >
                  <option value="seconds">seconds</option>
                  <option value="minutes">minutes</option>
                  <option value="hours">hours</option>
                  <option value="days">days</option>
                </select>
              </label>
            </div>
            <p className="config-help">
              persisted as {String(config.duration_seconds ?? 0)} seconds · max 7
              days
            </p>
          </>
        )}
        {node.type === "condition" && (
          <>
            <label>
              <span>input field</span>
              <input
                disabled={readOnly}
                onChange={(event) => updateConfig({ field: event.target.value })}
                placeholder="input.status"
                value={String(config.field ?? "")}
              />
            </label>
            <label>
              <span>operator</span>
              <select
                disabled={readOnly}
                onChange={(event) => {
                  const operator = event.target.value;
                  const next: Record<string, unknown> = { ...config, operator };
                  if (operator === "exists") delete next.value;
                  onUpdate({ ...node, config: next as WorkflowNodeConfig });
                }}
                value={String(config.operator ?? "equals")}
              >
                <option value="equals">equals</option>
                <option value="not_equals">not equals</option>
                <option value="exists">exists</option>
              </select>
            </label>
            {config.operator !== "exists" && (
              <label>
                <span>value</span>
                <input
                  disabled={readOnly}
                  onChange={(event) => updateConfig({ value: event.target.value })}
                  value={String(config.value ?? "")}
                />
              </label>
            )}
            <p className="config-help">
              connect the explicit true and false handles to persist branch
              metadata.
            </p>
          </>
        )}
        {(error || jsonError) && (
          <p aria-live="polite" className="form-error">
            {jsonError || error}
          </p>
        )}
        <button
          className="button button-danger"
          disabled={readOnly}
          onClick={onDelete}
          type="button"
        >
          delete node
        </button>
      </div>
    </div>
  );
}

function WorkflowConfiguration({
  name,
  description,
  readOnly,
  onChange,
  onDelete,
}: {
  name: string;
  description: string;
  readOnly: boolean;
  onChange: (name: string, description: string) => void;
  onDelete: () => void;
}) {
  return (
    <div>
      <p className="eyebrow">workflow</p>
      <h2>draft settings</h2>
      <div className="config-form">
        <label>
          <span>name</span>
          <input
            disabled={readOnly}
            onChange={(event) => onChange(event.target.value, description)}
            value={name}
          />
        </label>
        <label>
          <span>description · optional</span>
          <textarea
            disabled={readOnly}
            onChange={(event) => onChange(name, event.target.value)}
            value={description}
          />
        </label>
        <p className="config-help">
          select a node or edge to configure it. positions are saved in the
          workflow definition.
        </p>
        <button
          className="button button-danger"
          onClick={onDelete}
          type="button"
        >
          delete workflow
        </button>
      </div>
    </div>
  );
}

function EdgeConfiguration({
  edge,
  isCondition,
  readOnly,
  onDelete,
  onBranchChange,
}: {
  edge: FlowEdge;
  isCondition: boolean;
  readOnly: boolean;
  onDelete: () => void;
  onBranchChange: (branch: boolean) => void;
}) {
  return (
    <div>
      <p className="eyebrow">selected edge</p>
      <h2>connection settings</h2>
      <dl className="connection-summary">
        <div>
          <dt>from</dt>
          <dd>{edge.source}</dd>
        </div>
        <div>
          <dt>to</dt>
          <dd>{edge.target}</dd>
        </div>
      </dl>
      {isCondition && (
        <fieldset className="branch-options">
          <legend>condition branch</legend>
          {[true, false].map((branch) => (
            <label key={String(branch)}>
              <input
                checked={edge.data?.condition?.branch === branch}
                disabled={readOnly}
                name="branch"
                onChange={() => onBranchChange(branch)}
                type="radio"
              />
              {String(branch)}
            </label>
          ))}
        </fieldset>
      )}
      <button
        className="button button-danger"
        disabled={readOnly}
        onClick={onDelete}
        type="button"
      >
        delete connection
      </button>
    </div>
  );
}

function ValidationPanel({
  result,
  onSelect,
}: {
  result: WorkflowValidationResult;
  onSelect: (path: string) => void;
}) {
  return (
    <section
      aria-live="polite"
      className={`validation-panel ${result.valid ? "is-valid" : ""}`}
    >
      <strong>{result.valid ? "valid workflow" : `${result.errors.length} issues`}</strong>
      {!result.valid && (
        <ul>
          {result.errors.map((item, index) => (
            <li key={`${item.code}-${index}`}>
              <button onClick={() => onSelect(item.path ?? "")} type="button">
                <span>{item.message}</span>
                {item.path && <small>{item.path}</small>}
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function VersionsPanel({
  versions,
  detail,
  pending,
  onClose,
  onSelect,
}: {
  versions: WorkflowVersionSummary[];
  detail?: WorkflowVersionDetail;
  pending: boolean;
  onClose: () => void;
  onSelect: (version: number) => void;
}) {
  return (
    <div aria-modal="true" className="versions-backdrop" role="dialog">
      <section aria-label="immutable workflow versions" className="versions-panel">
        <header>
          <div>
            <p className="eyebrow">immutable snapshots</p>
            <h2>published versions</h2>
          </div>
          <button aria-label="close versions" className="text-button" onClick={onClose}>
            close
          </button>
        </header>
        {versions.length === 0 ? (
          <div className="empty-state">
            <h2>no published versions</h2>
            <p>validate and publish the draft to create version 1.</p>
          </div>
        ) : (
          <div className="versions-layout">
            <ol className="version-list">
              {versions.map((version) => (
                <li key={version.version_id}>
                  <button
                    disabled={pending}
                    onClick={() => onSelect(version.version)}
                    type="button"
                  >
                    <strong>version {version.version}</strong>
                    <span>{formatDate(version.published_at)}</span>
                    <small>{version.status} · immutable</small>
                  </button>
                </li>
              ))}
            </ol>
            <div className="version-detail">
              {pending ? (
                <p>loading snapshot…</p>
              ) : detail ? (
                <>
                  <strong>version {detail.version}</strong>
                  <p>{detail.name} · {detail.slug}</p>
                  <pre>{JSON.stringify(detail.definition, null, 2)}</pre>
                </>
              ) : (
                <p>select a version to inspect its immutable definition.</p>
              )}
            </div>
          </div>
        )}
      </section>
    </div>
  );
}

function JSONField({
  value,
  disabled,
  error,
  onChange,
}: {
  value: unknown;
  disabled: boolean;
  error: string;
  onChange: (value: string) => void;
}) {
  return (
    <label>
      <span>payload JSON · optional</span>
      <textarea
        className="json-input"
        defaultValue={value === undefined ? "" : JSON.stringify(value, null, 2)}
        disabled={disabled}
        onBlur={(event) => onChange(event.target.value)}
      />
      {error && <small className="input-error">{error}</small>}
    </label>
  );
}

function NumberField({
  label,
  value,
  min,
  max,
  readOnly,
  onChange,
}: {
  label: string;
  value: unknown;
  min: number;
  max?: number;
  readOnly: boolean;
  onChange: (value: number | undefined) => void;
}) {
  return (
    <label>
      <span>{label}</span>
      <input
        disabled={readOnly}
        max={max}
        min={min}
        onChange={(event) =>
          onChange(event.target.value === "" ? undefined : Number(event.target.value))
        }
        type="number"
        value={value === undefined ? "" : Number(value)}
      />
    </label>
  );
}

function NodeIcon({ type }: { type: WorkflowNodeType }) {
  return (
    <span aria-hidden="true" className={`node-icon node-icon-${type}`}>
      {type === "trigger"
        ? "▶"
        : type === "task"
          ? "□"
          : type === "webhook"
            ? "↗"
            : type === "delay"
              ? "◷"
              : "◇"}
    </span>
  );
}

function toFlowEdge(edge: EditorEdge): FlowEdge {
  return {
    id: edge.id,
    source: edge.source,
    target: edge.target,
    sourceHandle: edge.sourceHandle,
    label: edge.label,
    markerEnd: { type: MarkerType.ArrowClosed },
    data: {
      condition: edge.condition,
      validationMessages: (
        edge as EditorEdge & { data?: { validationMessages?: string[] } }
      ).data?.validationMessages,
    },
  };
}

function fromFlowEdge(edge: FlowEdge): EditorEdge {
  return {
    id: edge.id,
    source: edge.source,
    target: edge.target,
    sourceHandle:
      edge.sourceHandle === "true" || edge.sourceHandle === "false"
        ? edge.sourceHandle
        : undefined,
    label: typeof edge.label === "string" ? edge.label : undefined,
    condition: edge.data?.condition ?? null,
  };
}

function safeDestination(value: string) {
  try {
    const url = new URL(value);
    return url.host;
  } catch {
    return "configured destination";
  }
}

function isValidationResult(
  value: WorkflowValidationResult | WorkflowPublishResult,
): value is WorkflowValidationResult {
  return "valid" in value;
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
