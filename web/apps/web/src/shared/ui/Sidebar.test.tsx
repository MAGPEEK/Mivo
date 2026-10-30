import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../test/axe";
import { isVisuallyHidden } from "../../test/visibility";
import { createI18n } from "../i18n";
import { Sidebar, type SidebarLibraries, type SidebarProps } from "./Sidebar";

const emptyLibraries: SidebarLibraries = {
  state: "loaded",
  links: [],
  emptyHint: "Add a library to the configuration.",
};

const twoLibraries: SidebarLibraries = {
  state: "loaded",
  links: [
    { id: "lib-1", name: "Movies", icon: <svg data-testid="icon-lib-1" /> },
    { id: "lib-2", name: "Archive", icon: <svg data-testid="icon-lib-2" />, note: "Unavailable" },
  ],
  emptyHint: "unused",
};

interface RenderOptions {
  readonly footer?: SidebarProps["footer"];
  readonly collapsed?: boolean;
  readonly libraries?: SidebarLibraries;
  readonly path?: string;
}

// The sidebar links need a router; a root route plus the library route is enough to render
// it alone and to mark the current library.
async function renderSidebar(options: RenderOptions = {}): Promise<HTMLElement> {
  const { footer, collapsed = false, libraries = emptyLibraries, path = "/" } = options;
  // Omitted rather than undefined: the props use exactOptionalPropertyTypes.
  const footerProps = footer === undefined ? {} : { footer };
  const rootRoute = createRootRoute({
    component: () => (
      <>
        <Sidebar
          collapsed={collapsed}
          onToggleCollapsed={() => undefined}
          libraries={libraries}
          {...footerProps}
        />
        <Outlet />
      </>
    ),
  });
  const libraryRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/libraries/$libraryId",
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([libraryRoute]),
    history: createMemoryHistory({ initialEntries: [path] }),
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
    const sidebar = await renderSidebar({ footer: () => <p>Footer content</p> });

    const libraries = within(sidebar).getByText("No libraries yet");
    const footer = within(sidebar).getByText("Footer content");

    expect(follows(libraries, footer)).toBe(true);
  });

  it("keeps the footer visible while collapsed, unlike the labels", async () => {
    const sidebar = await renderSidebar({
      footer: () => <p>Footer content</p>,
      collapsed: true,
    });

    const footer = within(sidebar).getByText("Footer content");

    expect(isVisuallyHidden(footer)).toBe(false);
    expect(isVisuallyHidden(within(sidebar).getByText("No libraries yet"))).toBe(true);
  });

  it("ends with the libraries section when no footer is given", async () => {
    const sidebar = await renderSidebar();

    expect(sidebar.textContent.endsWith(emptyLibraries.emptyHint)).toBe(true);
  });

  it("passes the navigation id to the footer, for aria-controls", async () => {
    const sidebar = await renderSidebar({
      footer: (navigationId) => <p>Controls {navigationId}</p>,
    });

    expect(sidebar.id).not.toBe("");
    expect(within(sidebar).getByText(`Controls ${sidebar.id}`)).toBeTruthy();
  });
});

describe("Sidebar libraries section", () => {
  function librariesSection(sidebar: HTMLElement): HTMLElement {
    return within(sidebar).getByRole("region", { name: "Libraries" });
  }

  it("shows the empty text and the hint when no library is configured", async () => {
    const section = librariesSection(await renderSidebar());

    expect(within(section).getByText("No libraries yet")).toBeTruthy();
    expect(within(section).getByText(emptyLibraries.emptyHint)).toBeTruthy();
    expect(within(section).queryByRole("list")).toBeNull();
  });

  it.each([
    { state: "loading", message: "Loading libraries" },
    { state: "error", message: "Libraries could not be loaded" },
  ] as const)("shows the $state message", async (libraries) => {
    const section = librariesSection(await renderSidebar({ libraries }));

    expect(within(section).getByText(libraries.message)).toBeTruthy();
    expect(within(section).queryByText("No libraries yet")).toBeNull();
  });

  it("lists one link per library with its icon", async () => {
    const section = librariesSection(await renderSidebar({ libraries: twoLibraries }));

    const links = within(section).getAllByRole("link");

    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "/libraries/lib-1",
      "/libraries/lib-2",
    ]);
    expect(within(links[0] ?? section).getByTestId("icon-lib-1")).toBeTruthy();
  });

  it("names an unavailable library with its note", async () => {
    const sidebar = await renderSidebar({ libraries: twoLibraries });

    expect(within(sidebar).getByRole("link", { name: "Movies" })).toBeTruthy();
    expect(within(sidebar).getByRole("link", { name: "Archive, Unavailable" })).toBeTruthy();
  });

  it("marks the library of the current page", async () => {
    const sidebar = await renderSidebar({ libraries: twoLibraries, path: "/libraries/lib-2" });

    const current = within(sidebar).getByRole("link", { name: "Archive, Unavailable" });

    expect(current.getAttribute("aria-current")).toBe("page");
    expect(within(sidebar).getByRole("link", { name: "Movies" }).hasAttribute("aria-current")).toBe(
      false,
    );
  });

  it("keeps library names for screen readers while collapsed", async () => {
    const sidebar = await renderSidebar({ libraries: twoLibraries, collapsed: true });

    const link = within(sidebar).getByRole("link", { name: "Movies" });

    expect(isVisuallyHidden(within(link).getByText("Movies"))).toBe(true);
  });

  it.each([false, true])(
    "has no serious or critical axe findings (collapsed: %s)",
    async (collapsed) => {
      const sidebar = await renderSidebar({ libraries: twoLibraries, collapsed });

      expect(await blockingAxeViolations(sidebar)).toEqual([]);
    },
  );
});
