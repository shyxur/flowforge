import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import WorkflowsLoading from "./loading";
import WorkflowsPage from "./page";

const api = vi.hoisted(() => ({
  listWorkflows: vi.fn(),
  listWorkflowVersions: vi.fn(),
}));

vi.mock("@/lib/queueflow", () => ({
  ...api,
  QueueFlowAPIError: class QueueFlowAPIError extends Error {
    constructor(
      message: string,
      readonly status: number,
    ) {
      super(message);
    }
  },
}));

describe("WorkflowsPage", () => {
  beforeEach(() => {
    api.listWorkflows.mockReset();
    api.listWorkflowVersions.mockReset();
  });

  it("renders its loading skeleton", () => {
    render(<WorkflowsLoading />);
    expect(screen.getByLabelText("loading workflows")).toHaveAttribute(
      "aria-busy",
      "true",
    );
  });

  it("renders the concise empty state and create navigation", async () => {
    api.listWorkflows.mockResolvedValue({ items: [] });
    render(await WorkflowsPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("no workflows yet")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "create workflow" }),
    ).toHaveAttribute("href", "/workflows/new");
  });

  it("renders workflows, status, latest version, and editor navigation", async () => {
    api.listWorkflows.mockResolvedValue({
      items: [
        {
          id: "workflow-1",
          name: "Order flow",
          slug: "order-flow",
          description: "Routes new orders",
          status: "published",
          updated_at: "2026-07-30T10:00:00Z",
        },
      ],
    });
    api.listWorkflowVersions.mockResolvedValue({
      items: [{ version: 3 }],
    });
    render(await WorkflowsPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("Order flow")).toBeInTheDocument();
    expect(screen.getByText("published")).toBeInTheDocument();
    expect(screen.getByText("v3")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "open editor" })).toHaveAttribute(
      "href",
      "/workflows/workflow-1",
    );
  });

  it("renders an API error with retry navigation", async () => {
    api.listWorkflows.mockRejectedValue(new Error("offline"));
    render(await WorkflowsPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("workflows unavailable")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "retry" })).toHaveAttribute(
      "href",
      "/workflows",
    );
  });
});
