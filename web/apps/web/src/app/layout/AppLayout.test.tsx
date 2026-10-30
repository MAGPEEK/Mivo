import { productName } from "@mivo/ui-tokens";
import { fireEvent, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { fakeSystemInfo } from "../../test/fakeApi";
import { renderApp } from "../../test/renderApp";
import { sidebarCollapsedStorageKey } from "./useSidebarState";

async function findSidebar(): Promise<HTMLElement> {
  return screen.findByRole("navigation", { name: "Main navigation" });
}

function sidebarToggle(): HTMLElement {
  return screen.getByRole("button", { name: "Toggle sidebar" });
}

function searchField(): HTMLElement {
  return screen.getByRole("searchbox", { name: "Search" });
}

describe("AppLayout landmarks", () => {
  it("renders a labelled navigation, a header with search and the main content", async () => {
    renderApp("/");

    const sidebar = await findSidebar();
    const header = screen.getByRole("banner");
    const main = screen.getByRole("main");

    expect(within(sidebar).getByText(productName)).toBeTruthy();
    expect(within(header).getByRole("search")).toBeTruthy();
    expect(within(main).getByRole("heading", { level: 1, name: "Home" })).toBeTruthy();
  });

  it("offers a skip link to the main content", async () => {
    renderApp("/");

    const skipLink = await screen.findByRole("link", { name: "Skip to content" });

    expect(skipLink.getAttribute("href")).toBe(`#${screen.getByRole("main").id}`);
  });
});

describe("Sidebar navigation", () => {
  it("marks the home link as the current page on /", async () => {
    renderApp("/");

    const homeLink = within(await findSidebar()).getByRole("link", { name: "Home" });

    expect(homeLink.getAttribute("aria-current")).toBe("page");
  });

  it("does not mark the home link on another page", async () => {
    renderApp("/does-not-exist");
    await screen.findByRole("heading", { level: 1, name: "Page not found" });

    const homeLink = within(await findSidebar()).getByRole("link", { name: "Home" });

    expect(homeLink.hasAttribute("aria-current")).toBe(false);
  });

  it("shows the empty libraries section", async () => {
    renderApp("/");

    const sidebar = await findSidebar();

    expect(within(sidebar).getByRole("heading", { level: 2, name: "Libraries" })).toBeTruthy();
    expect(within(sidebar).getByText("No libraries yet")).toBeTruthy();
  });
});

describe("Sidebar collapse toggle", () => {
  it("controls the sidebar and starts expanded", async () => {
    renderApp("/");
    const sidebar = await findSidebar();

    expect(sidebarToggle().getAttribute("aria-controls")).toBe(sidebar.id);
    expect(sidebarToggle().getAttribute("aria-expanded")).toBe("true");
  });

  it("flips aria-expanded and persists the collapsed state", async () => {
    renderApp("/");
    await findSidebar();

    fireEvent.click(sidebarToggle());

    expect(sidebarToggle().getAttribute("aria-expanded")).toBe("false");
    expect(window.localStorage.getItem(sidebarCollapsedStorageKey)).toBe("true");

    fireEvent.click(sidebarToggle());

    expect(sidebarToggle().getAttribute("aria-expanded")).toBe("true");
    expect(window.localStorage.getItem(sidebarCollapsedStorageKey)).toBe("false");
  });

  it("restores the collapsed state on the next visit", async () => {
    window.localStorage.setItem(sidebarCollapsedStorageKey, "true");

    renderApp("/");
    await findSidebar();

    expect(sidebarToggle().getAttribute("aria-expanded")).toBe("false");
  });

  it("keeps every label available to screen readers while collapsed", async () => {
    window.localStorage.setItem(sidebarCollapsedStorageKey, "true");

    renderApp("/");
    const sidebar = await findSidebar();

    expect(within(sidebar).getByRole("link", { name: "Home" })).toBeTruthy();
    expect(within(sidebar).getByText("No libraries yet")).toBeTruthy();
    expect(
      await within(sidebar).findByText(`Server version ${fakeSystemInfo.version}`),
    ).toBeTruthy();
  });
});

describe("Search shortcut", () => {
  it.each([
    { name: "Ctrl+K", modifiers: { ctrlKey: true } },
    { name: "Cmd+K", modifiers: { metaKey: true } },
  ])("$name focuses the search field and prevents the browser default", async (shortcut) => {
    renderApp("/");
    await findSidebar();
    expect(document.activeElement).not.toBe(searchField());

    const notPrevented = fireEvent.keyDown(document.body, { key: "k", ...shortcut.modifiers });

    expect(notPrevented).toBe(false);
    expect(document.activeElement).toBe(searchField());
  });

  it("selects the previous query so typing replaces it", async () => {
    renderApp("/");
    await findSidebar();
    const field = searchField() as HTMLInputElement;
    fireEvent.change(field, { target: { value: "matrix" } });

    fireEvent.keyDown(document.body, { key: "K", ctrlKey: true });

    expect([field.selectionStart, field.selectionEnd]).toEqual([0, "matrix".length]);
  });

  it("ignores K without the shortcut modifier", async () => {
    renderApp("/");
    await findSidebar();

    const notPrevented = fireEvent.keyDown(document.body, { key: "k" });

    expect(notPrevented).toBe(true);
    expect(document.activeElement).not.toBe(searchField());
  });

  it("announces the shortcut on the search field", async () => {
    renderApp("/");
    await findSidebar();

    expect(searchField().getAttribute("aria-keyshortcuts")).toBe("Control+K Meta+K");
    expect(searchField().getAttribute("placeholder")).toBe("Search movies, shows and music");
  });

  it("keeps the app on the page when the search is submitted", async () => {
    const { router } = renderApp("/");
    await findSidebar();

    const notPrevented = fireEvent.submit(screen.getByRole("search"));

    expect(notPrevented).toBe(false);
    expect(router.state.location.pathname).toBe("/");
  });
});
