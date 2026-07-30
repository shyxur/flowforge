import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CreateWorkflowForm } from "./workflow-create-form";

const push = vi.fn();
const createWorkflowAction = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
}));
vi.mock("../actions", () => ({
  createWorkflowAction: (...args: unknown[]) => createWorkflowAction(...args),
}));

describe("CreateWorkflowForm", () => {
  beforeEach(() => {
    push.mockReset();
    createWorkflowAction.mockReset();
  });

  it("shows required name validation", () => {
    render(<CreateWorkflowForm />);
    fireEvent.submit(screen.getByRole("button", { name: "create workflow" }).closest("form")!);
    expect(screen.getByText("workflow name is required")).toBeInTheDocument();
  });

  it("creates and redirects to the editor", async () => {
    createWorkflowAction.mockResolvedValue({
      ok: true,
      data: { id: "workflow-1" },
    });
    render(<CreateWorkflowForm />);
    fireEvent.change(screen.getByLabelText("name"), {
      target: { value: "Order lifecycle" },
    });
    fireEvent.click(screen.getByRole("button", { name: "create workflow" }));
    await waitFor(() =>
      expect(push).toHaveBeenCalledWith("/workflows/workflow-1?created=1"),
    );
  });

  it("preserves and displays API failures", async () => {
    createWorkflowAction.mockResolvedValue({
      ok: false,
      error: "workflow slug already exists",
    });
    render(<CreateWorkflowForm />);
    fireEvent.change(screen.getByLabelText("name"), {
      target: { value: "Existing" },
    });
    fireEvent.click(screen.getByRole("button", { name: "create workflow" }));
    expect(await screen.findByText("workflow slug already exists")).toBeInTheDocument();
    expect(screen.getByLabelText("name")).toHaveValue("Existing");
  });
});
