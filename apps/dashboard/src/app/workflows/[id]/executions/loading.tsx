export default function WorkflowExecutionsLoading() {
  return (
    <div aria-busy="true" aria-label="loading workflow executions">
      <div className="skeleton skeleton-heading" />
      <div className="panel workflow-list-skeleton">
        {Array.from({ length: 6 }, (_, index) => (
          <div className="skeleton skeleton-row" key={index} />
        ))}
      </div>
    </div>
  );
}
