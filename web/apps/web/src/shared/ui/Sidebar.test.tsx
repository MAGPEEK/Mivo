import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it } from "vitest";

import { isVisuallyHidden } from "../../test/visibility";
import { createI18n } from "../i18n";
import { Sidebar, type SidebarProps } from "./Sidebar";

// The sidebar links need a router; a single root route is enough to render it alone.
async function renderSidebar(
  footer?: SidebarProps["footer"],
  collapsed = false,
): Promise<HTMLElement> {
  // Omitted rather than undefined: the props use exactOptionalPropertyTypes.
  const footerProps = footer === undefined ? {} : { footer };
  const rootRoute = createRootRoute({
    component: () => (
      <Sidebar collapsed={collapsed} onToggleCollapsed={() => undefined} {...footerProps} />
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

function follows(earlier: Node, later: Node): boolean {
  return (earlier.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
}

describe("Sidebar footer slot", () => {
  it("renders the given footer inside the navigation, after the libraries", async () => {
    const sidebar = await renderSidebar(() => <p>Footer content</p>);

    const libraries = within(sidebar).getByText("No libraries yet");
    const footer = within(sidebar).getByText("Footer content");

    expect(follows(libraries, footer)).toBe(true);
  });

  it("keeps the footer visible while collapsed, unlike the labels", async () => {
    const sidebar = await renderSidebar(() => <p>Footer content</p>, true);

    const footer = within(sidebar).getByText("Footer content");

    expect(isVisuallyHidden(footer)).toBe(false);
    expect(isVisuallyHidden(within(sidebar).getByText("No libraries yet"))).toBe(true);
  });

  it("ends with the libraries section when no footer is given", async () => {
    const sidebar = await renderSidebar();

    expect(sidebar.textContent.endsWith("No libraries yet")).toBe(true);
  });

  it("passes the navigation id to the footer, for aria-controls", async () => {
    const sidebar = await renderSidebar((navigationId) => <p>Controls {navigationId}</p>);

    expect(sidebar.id).not.toBe("");
    expect(within(sidebar).getByText(`Controls ${sidebar.id}`)).toBeTruthy();
  });
});
