export function StatusBadge({ status }: { status: string }) {
  return (
    <span className={`task-status status-${status}`}>
      <span />
      {status.replace("_", " ")}
    </span>
  );
}
