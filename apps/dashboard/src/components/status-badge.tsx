import type { TaskStatus } from "@/lib/queueflow";

export function StatusBadge({ status }: { status: TaskStatus }) {
  return (
    <span className={`task-status status-${status}`}>
      <span />
      {status.replace("_", " ")}
    </span>
  );
}
