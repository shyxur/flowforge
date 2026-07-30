"use client";

import { FormEvent, useEffect, useRef, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type {
  WorkflowStatus,
  WorkflowVersionSummary,
} from "@/lib/workflow-types";
import { startWorkflowExecutionAction } from "./actions";

const maxInputBytes = 256 * 1024;

export function RunWorkflowDialog({
  workflowID,
  workflowStatus,
  versions,
  initialOpen = false,
}: {
  workflowID: string;
  workflowStatus: WorkflowStatus;
  versions: WorkflowVersionSummary[];
  initialOpen?: boolean;
}) {
  const router = useRouter();
  const [open, setOpen] = useState(initialOpen);
  const [version, setVersion] = useState("");
  const [input, setInput] = useState("{}");
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();
  const textarea = useRef<HTMLTextAreaElement>(null);
  const canRun = workflowStatus === "published" && versions.length > 0;

  useEffect(() => {
    if (open) textarea.current?.focus();
  }, [open]);

  function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    let parsed: unknown;
    try {
      parsed = JSON.parse(input);
    } catch {
      setError("execution input must be valid JSON");
      return;
    }
    if (new TextEncoder().encode(input).byteLength > maxInputBytes) {
      setError("execution input exceeds the 256 KiB limit");
      return;
    }
    const idempotencyKey = `dashboard-${crypto.randomUUID()}`;
    startTransition(async () => {
      const result = await startWorkflowExecutionAction({
        workflowID,
        idempotencyKey,
        request: {
          ...(version ? { version: Number(version) } : {}),
          input: parsed,
        },
      });
      if (!result.ok) {
        setError(result.error);
        return;
      }
      router.push(
        `/workflows/${workflowID}/executions/${result.data.execution_id}`,
      );
    });
  }

  return (
    <>
      <button
        className="button button-primary"
        disabled={!canRun}
        onClick={() => setOpen(true)}
        title={canRun ? undefined : "publish a workflow version first"}
        type="button"
      >
        run workflow
      </button>
      {open && canRun && (
        <div className="versions-backdrop">
          <section
            aria-labelledby="run-workflow-title"
            aria-modal="true"
            className="run-dialog"
            role="dialog"
          >
            <header>
              <div>
                <p className="eyebrow">immutable execution</p>
                <h2 id="run-workflow-title">run workflow</h2>
              </div>
              <button
                aria-label="close run workflow dialog"
                className="text-button"
                disabled={pending}
                onClick={() => setOpen(false)}
                type="button"
              >
                close
              </button>
            </header>
            <form className="config-form" onSubmit={submit}>
              <label>
                <span>published version</span>
                <select
                  disabled={pending}
                  onChange={(event) => setVersion(event.target.value)}
                  value={version}
                >
                  <option value="">
                    latest published · v{versions[0].version}
                  </option>
                  {versions.map((item) => (
                    <option key={item.version_id} value={item.version}>
                      version {item.version} · {item.name}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                <span>input JSON · max 256 KiB</span>
                <textarea
                  className="json-input run-input"
                  disabled={pending}
                  onChange={(event) => setInput(event.target.value)}
                  ref={textarea}
                  value={input}
                />
              </label>
              <p className="config-help">
                each deliberate run receives a new idempotency key and remains
                tied to the selected immutable version.
              </p>
              {error && (
                <p aria-live="polite" className="form-error">
                  {error}
                </p>
              )}
              <div className="run-dialog-actions">
                <button
                  className="button button-quiet"
                  disabled={pending}
                  onClick={() => setOpen(false)}
                  type="button"
                >
                  cancel
                </button>
                <button
                  className="button button-primary"
                  disabled={pending}
                  type="submit"
                >
                  {pending ? "starting…" : "run"}
                </button>
              </div>
            </form>
          </section>
        </div>
      )}
    </>
  );
}
