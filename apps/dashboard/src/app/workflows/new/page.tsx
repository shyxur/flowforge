import type { Metadata } from "next";
import Link from "next/link";
import { CreateWorkflowForm } from "./workflow-create-form";

export const metadata: Metadata = { title: "new workflow" };

export default function NewWorkflowPage() {
  return (
    <div className="narrow-page">
      <Link className="back-link" href="/workflows">
        ← back to workflows
      </Link>
      <p className="eyebrow">taskcanvas</p>
      <h1 className="page-title">new workflow</h1>
      <p className="page-copy">
        start with one trigger, then build the graph in the visual editor.
      </p>
      <CreateWorkflowForm />
    </div>
  );
}
