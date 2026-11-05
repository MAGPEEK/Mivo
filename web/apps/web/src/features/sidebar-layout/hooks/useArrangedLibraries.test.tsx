import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it, vi } from "vitest";

import { createI18n } from "../../../shared/i18n";
import { Sidebar, type SidebarLibraryLink } from "../../../shared/ui";
import { blockingAxeViolations } from "../../../test/axe";
import { libraryLayoutStorageKey } from "../lib/layoutStore";
import { serializeLayout } from "../lib/libraryLayout";
import { useArrangedLibraries, useResetLibraryLayout } from "./useArrangedLibraries";

function link(id: string, name: string): SidebarLibraryLink {
  return { id, name, icon: <svg /> };
}

const threeLibraries = [link("m", "Movies"), link("s", "Shows"), link("a", "Archive")];

interface HarnessProps {
  readonly links: readonly SidebarLibraryLink[];
  readonly collapsed: boolean;
  readonly onExpand: () => void;
}

// The hook feeds the real sidebar, so both reorder paths run as a user meets them.
function Harness({ links, collapsed, onExpand }: HarnessProps) {
  const libraries = useArrangedLibraries({ state: "loaded", links });
  const reset = useResetLibraryLayout();
  return (
    <>
      <Sidebar collapsed={collapsed} onToggleCollapsed={onExpand} libraries={libraries} />
      <button type="button" onClick={reset}>
        External reset
      </button>
    </>
  );
}

interface RenderOptions {
  readonly links?: readonly SidebarLibraryLink[];
  readonly collapsed?: boolean;
  readonly onExpand?: () => void;
}

async function renderArranged(options: RenderOptions = {}): Promise<HTMLElement> {
  const { links = threeLibraries, collapsed = false, onExpand = () => undefined } = options;
  const rootRoute = createRootRoute({
    component: () => (
      <>
        <Harness links={links} collapsed={collapsed} onExpand={onExpand} />
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
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(
    <I18nextProvider i18n={createI18n(["en"])}>
      <RouterProvider router={router} />
    </I18nextProvider>,
  );
  return screen.findByRole("navigation", { name: "Main navigation" });
}

function libraryNames(sidebar: HTMLElement): string[] {
  return within(sidebar)
    .getAllByRole("link")
    .filter((element) => element.getAttribute("href")?.startsWith("/libraries/") === true)
    .map((element) => element.textContent);
}

function liveRegion(sidebar: HTMLElement): HTMLElement {
  const region = sidebar.querySelector("[aria-live]");
  if (!(region instanceof HTMLElement)) {
    throw new Error("live region missing");
  }
  return region;
}

function openAllLibraries(sidebar: HTMLElement): void {
  fireEvent.click(within(sidebar).getByRole("button", { name: "All libraries" }));
}

function storeLayout(order: string[], unpinned: string[]): void {
  window.localStorage.setItem(libraryLayoutStorageKey, serializeLayout({ order, unpinned }));
}

describe("useArrangedLibraries with the sidebar", () => {
  it("shows every library pinned in server order by default", async () => {
    const sidebar = await renderArranged();

    expect(libraryNames(sidebar)).toEqual(["Movies", "Shows", "Archive"]);
    expect(within(sidebar).getByRole("button", { name: "All libraries" })).toBeTruthy();
  });

  it("applies the saved order, hides unpinned and appends new libraries pinned", async () => {
    storeLayout(["a", "gone", "m", "s"], ["s"]);

    const sidebar = await renderArranged({
      links: [...threeLibraries, link("n", "New")],
    });

    expect(libraryNames(sidebar)).toEqual(["Archive", "Movies", "New"]);
  });

  it("falls back to the default layout for corrupt storage", async () => {
    window.localStorage.setItem(libraryLayoutStorageKey, "{corrupt");

    const sidebar = await renderArranged();

    expect(libraryNames(sidebar)).toEqual(["Movies", "Shows", "Archive"]);
  });

  it("reorders with the move buttons, keeps focus and announces the move", async () => {
    const sidebar = await renderArranged();
    openAllLibraries(sidebar);
    const done = within(sidebar).getByRole("button", { name: "Done" });
    expect(document.activeElement).toBe(done);

    fireEvent.click(within(sidebar).getByRole("button", { name: "Move down: Movies" }));
    fireEvent.click(within(sidebar).getByRole("button", { name: "Move down: Movies" }));

    expect(libraryNames(sidebar)).toEqual(["Shows", "Archive", "Movies"]);
    // Movies reached the end, so focus moves to its enabled counterpart.
    expect(document.activeElement).toBe(
      within(sidebar).getByRole("button", { name: "Move up: Movies" }),
    );
    expect(liveRegion(sidebar).textContent).toBe("Movies moved to position 3 of 3");
    expect(window.localStorage.getItem(libraryLayoutStorageKey)).toContain('"order":["s","a","m"]');
  });

  it("disables moving past either end", async () => {
    const sidebar = await renderArranged();
    openAllLibraries(sidebar);

    const up = within(sidebar).getByRole("button", { name: "Move up: Movies" });
    const down = within(sidebar).getByRole("button", { name: "Move down: Archive" });

    expect((up as HTMLButtonElement).disabled).toBe(true);
    expect((down as HTMLButtonElement).disabled).toBe(true);
  });

  it("unpins a library, which then shows only in All libraries", async () => {
    const sidebar = await renderArranged();
    openAllLibraries(sidebar);
    const pin = within(sidebar).getByRole("button", { name: "Pin Shows" });

    fireEvent.click(pin);

    expect(pin.getAttribute("aria-pressed")).toBe("false");
    expect(liveRegion(sidebar).textContent).toBe("Shows unpinned");
    fireEvent.click(within(sidebar).getByRole("button", { name: "Done" }));
    expect(libraryNames(sidebar)).toEqual(["Movies", "Archive"]);
    expect(document.activeElement).toBe(
      within(sidebar).getByRole("button", { name: "All libraries" }),
    );
  });

  it("reorders pinned libraries by drag and drop", async () => {
    storeLayout(["m", "s", "a"], ["s"]);
    const sidebar = await renderArranged();
    const rows = within(sidebar)
      .getAllByRole("link")
      .filter((row) => row.getAttribute("draggable") === "true");
    const [moviesRow, archiveRow] = rows;
    if (moviesRow === undefined || archiveRow === undefined) {
      throw new Error("rows missing");
    }
    const dataTransfer = { setData: vi.fn(), effectAllowed: "", dropEffect: "" };

    fireEvent.dragStart(moviesRow, { dataTransfer });
    fireEvent.dragOver(archiveRow, { dataTransfer });
    fireEvent.drop(archiveRow, { dataTransfer });

    expect(libraryNames(sidebar)).toEqual(["Archive", "Movies"]);
    expect(liveRegion(sidebar).textContent).toBe("Movies moved to position 2 of 2");
    expect(window.localStorage.getItem(libraryLayoutStorageKey)).toContain('"order":["s","a","m"]');
  });

  it("highlights the drop target only while the drag is over it", async () => {
    const sidebar = await renderArranged();
    const [moviesRow, showsRow] = within(sidebar)
      .getAllByRole("link")
      .filter((row) => row.getAttribute("draggable") === "true");
    if (moviesRow === undefined || showsRow === undefined) {
      throw new Error("rows missing");
    }
    const dataTransfer = { setData: vi.fn(), effectAllowed: "", dropEffect: "" };
    const showsItem = showsRow.closest("li");

    fireEvent.dragStart(moviesRow, { dataTransfer });
    fireEvent.dragOver(showsRow, { dataTransfer });
    const highlighted = showsItem?.className.includes("ring-accent");
    fireEvent.dragLeave(showsRow, { dataTransfer });

    expect(highlighted).toBe(true);
    expect(showsItem?.className.includes("ring-accent")).toBe(false);
  });

  it("announces a repeated message again", async () => {
    const sidebar = await renderArranged();
    openAllLibraries(sidebar);
    const reset = within(sidebar).getByRole("button", { name: "Reset layout" });

    fireEvent.click(reset);
    const first = liveRegion(sidebar).firstElementChild;
    fireEvent.click(reset);
    const second = liveRegion(sidebar).firstElementChild;

    expect(second?.textContent).toBe("Layout reset");
    expect(second).not.toBe(first);
  });

  it("restores the server order with everything pinned on reset", async () => {
    storeLayout(["a", "s", "m"], ["s"]);
    const sidebar = await renderArranged();
    openAllLibraries(sidebar);

    fireEvent.click(within(sidebar).getByRole("button", { name: "Reset layout" }));

    expect(liveRegion(sidebar).textContent).toBe("Layout reset");
    fireEvent.click(within(sidebar).getByRole("button", { name: "Done" }));
    expect(libraryNames(sidebar)).toEqual(["Movies", "Shows", "Archive"]);
    expect(window.localStorage.getItem(libraryLayoutStorageKey)).toBeNull();
  });

  it("follows a reset triggered elsewhere, e.g. in the device settings", async () => {
    storeLayout(["a", "s", "m"], []);
    const sidebar = await renderArranged();

    act(() => {
      screen.getByRole("button", { name: "External reset" }).click();
    });

    expect(libraryNames(sidebar)).toEqual(["Movies", "Shows", "Archive"]);
  });

  it("expands a collapsed sidebar when All libraries is pressed", async () => {
    const onExpand = vi.fn();
    const sidebar = await renderArranged({ collapsed: true, onExpand });

    openAllLibraries(sidebar);

    expect(onExpand).toHaveBeenCalledTimes(1);
  });

  it.each([false, true])(
    "has no serious or critical axe findings (all libraries: %s)",
    async (all) => {
      const sidebar = await renderArranged();
      if (all) {
        openAllLibraries(sidebar);
      }

      expect(await blockingAxeViolations(sidebar)).toEqual([]);
    },
  );
});
