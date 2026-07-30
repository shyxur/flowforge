import Link from "next/link";

export default function WorkflowNotFound() {
  return (
    <section className="panel empty-state">
      <span className="empty-state-mark">404</span>
      <h2>workflow not found</h2>
      <p>it may have been removed or belongs to another organization.</p>
      <Link className="button button-primary empty-action" href="/workflows">
        back to workflows
      </Link>
    </section>
  );
}
