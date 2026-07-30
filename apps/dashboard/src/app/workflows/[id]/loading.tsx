export default function WorkflowEditorLoading() {
  return (
    <div aria-busy="true" aria-label="loading workflow editor">
      <div className="skeleton skeleton-heading" />
      <div className="workflow-editor-skeleton">
        <div className="skeleton" />
        <div className="skeleton" />
      </div>
    </div>
  );
}
