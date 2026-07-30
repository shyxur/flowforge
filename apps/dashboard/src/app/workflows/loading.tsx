export default function WorkflowsLoading() {
  return (
    <div aria-busy="true" aria-label="loading workflows">
      <div className="skeleton skeleton-heading" />
      <div className="panel workflow-list-skeleton">
        {Array.from({ length: 5 }, (_, index) => (
          <div className="skeleton skeleton-row" key={index} />
        ))}
      </div>
    </div>
  );
}
