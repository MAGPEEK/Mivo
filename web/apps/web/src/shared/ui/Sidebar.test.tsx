import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it } from "vitest";

import { createI18n } from "../i18n";
import { Sidebar } from "./Sidebar";

// The sidebar links need a router; a single root route is enough to render it alone.
async function renderSidebar(footer?: ReactNode): Promise<HTMLElement> {
  const rootRoute = createRootRoute({
    component: () => (
      <Sidebar collapsed={false} onToggleCollapsed={() => undefined} footer={footer} />
    ),
  });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(
    <I18nextProvider i18n={createI18n(["en"])}>
      <RouterProvider router={router} />
    </I18nextProvider>,
  );
  return screen.findByRole("navigation", { name: "Main navigation" });
}

describe("Sidebar footer slot", () => {
  it("renders the given footer at the bottom of the navigation", async () => {
    const sidebar = await renderSidebar(<p>Footer content</p>);

    const footer = within(sidebar).getByText("Footer content");

    expect(sidebar.lastElementChild?.contains(footer)).toBe(true);
  });

  it("leaves the footer area out when no footer is given", async () => {
    const sidebar = await renderSidebar();

    expect(within(sidebar).getByText("No libraries yet")).toBeTruthy();
    expect(sidebar.lastElementChild?.textContent).toBe("LibrariesNo libraries yet");
  });
});
