import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SidebarNav } from "./sidebar-nav";

vi.mock("next/navigation", () => ({
  usePathname: () => "/workflows/abc",
}));

describe("SidebarNav", () => {
  it("renders workflows and marks nested editor routes active", () => {
    render(<SidebarNav />);
    const link = screen.getByRole("link", { name: /workflows/ });
    expect(link).toHaveAttribute("href", "/workflows");
    expect(link).toHaveAttribute("aria-current", "page");
  });
});
