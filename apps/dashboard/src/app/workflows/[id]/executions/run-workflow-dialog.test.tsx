import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RunWorkflowDialog } from "./run-workflow-dialog";

const push = vi.fn();
const start = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
}));
vi.mock("./actions", () => ({
  startWorkflowExecutionAction: (...args: unknown[]) => start(...args),
}));

const versions = [
  {
    version: 2,
    version_id: "version-2",
    status: "published" as const,
    published_at: "2026-07-30T10:00:00Z",
    name: "Flow",
    slug: "flow",
  },
  {
    version: 1,
    version_id: "version-1",
    status: "published" as const,
    published_at: "2026-07-30T09:00:00Z",
    name: "Flow",
    slug: "flow",
  },
];

describe("RunWorkflowDialog", () => {
  beforeEach(() => {
    push.mockReset();
    start.mockReset();
  });

  it("disables running for an unpublished workflow", () => {
    render(
      <RunWorkflowDialog
        versions={[]}
        workflowID="workflow-1"
        workflowStatus="draft"
      />,
    );
    expect(screen.getByRole("button", { name: "run workflow" })).toBeDisabled();
  });

  it("opens with latest published version selected", () => {
    render(
      <RunWorkflowDialog
        versions={versions}
        workflowID="workflow-1"
        workflowStatus="published"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "run workflow" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText("published version")).toHaveValue("");
    expect(screen.getByText("latest published · v2")).toBeInTheDocument();
  });

  it("rejects invalid input JSON before starting", () => {
    render(
      <RunWorkflowDialog
        initialOpen
        versions={versions}
        workflowID="workflow-1"
        workflowStatus="published"
      />,
    );
    fireEvent.change(screen.getByLabelText(/input JSON/), {
      target: { value: "{" },
    });
    fireEvent.click(screen.getByRole("button", { name: "run" }));
    expect(screen.getByText("execution input must be valid JSON")).toBeInTheDocument();
    expect(start).not.toHaveBeenCalled();
  });

  it("serializes explicit version and redirects after success", async () => {
    vi.spyOn(crypto, "randomUUID").mockReturnValue(
      "11111111-1111-4111-8111-111111111111",
    );
    start.mockResolvedValue({
      ok: true,
      data: { execution_id: "execution-1" },
    });
    render(
      <RunWorkflowDialog
        initialOpen
        versions={versions}
        workflowID="workflow-1"
        workflowStatus="published"
      />,
    );
    fireEvent.change(screen.getByLabelText("published version"), {
      target: { value: "1" },
    });
    fireEvent.change(screen.getByLabelText(/input JSON/), {
      target: { value: '{"customer":"one"}' },
    });
    fireEvent.click(screen.getByRole("button", { name: "run" }));
    await waitFor(() =>
      expect(start).toHaveBeenCalledWith({
        workflowID: "workflow-1",
        idempotencyKey:
          "dashboard-11111111-1111-4111-8111-111111111111",
        request: { version: 1, input: { customer: "one" } },
      }),
    );
    expect(push).toHaveBeenCalledWith(
      "/workflows/workflow-1/executions/execution-1",
    );
  });

  it("preserves safe API errors and input", async () => {
    start.mockResolvedValue({
      ok: false,
      error: "rate limit reached; wait briefly and retry",
      status: 429,
    });
    render(
      <RunWorkflowDialog
        initialOpen
        versions={versions}
        workflowID="workflow-1"
        workflowStatus="published"
      />,
    );
    fireEvent.change(screen.getByLabelText(/input JSON/), {
      target: { value: '{"keep":true}' },
    });
    fireEvent.click(screen.getByRole("button", { name: "run" }));
    expect(
      await screen.findByText("rate limit reached; wait briefly and retry"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/input JSON/)).toHaveValue('{"keep":true}');
  });
});
