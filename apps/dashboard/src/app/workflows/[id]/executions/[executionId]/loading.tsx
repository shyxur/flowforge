export default function WorkflowExecutionDetailLoading() {
  return (
    <div aria-busy="true" aria-label="loading workflow execution">
      <div className="skeleton skeleton-heading" />
      <div className="execution-detail-skeleton">
        <div className="skeleton" />
        <div className="skeleton" />
        <div className="skeleton" />
      </div>
    </div>
  );
}
