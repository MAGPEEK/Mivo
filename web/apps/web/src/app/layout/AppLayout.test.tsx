import type { LibraryKind, Problem } from "@mivo/api-client";
import { productId, productName } from "@mivo/ui-tokens";
import { act, fireEvent, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  createFakeServer,
  fakeSystemInfo,
  respondNever,
  respondWithJson,
  respondWithProblem,
} from "../../test/fakeApi";
import { fakeLibrary, librariesRoute, libraryRoute } from "../../test/libraryFixtures";
import { stubMediaQueries } from "../../test/mediaQuery";
import { renderApp } from "../../test/renderApp";
import { isVisuallyHidden } from "../../test/visibility";
import { sidebarCollapsedStorageKey } from "./useSidebarState";

const internalError: Problem = {
  type: "about:blank",
  title: "Internal Server Error",
  status: 500,
  code: "internal",
};

async function findSidebar(): Promise<HTMLElement> {
  return screen.findByRole("navigation", { name: "Main navigation" });
}

function sidebarToggle(): HTMLElement {
  return screen.getByRole("button", { name: "Toggle sidebar" });
}

interface CollapsedUnreachable {
  readonly sidebar: HTMLElement;
  readonly indicator: HTMLElement;
}

async function renderCollapsedUnreachable(): Promise<CollapsedUnreachable> {
  window.localStorage.setItem(sidebarCollapsedStorageKey, "true");
  renderApp("/", { fetch: respondWithProblem(internalError) });
  const sidebar = await findSidebar();
  const status = within(sidebar).getByRole("status");
  const indicator = await within(status).findByRole("button", { name: "Server unreachable" });
  return { sidebar, indicator };
}

function tooltipOf(indicator: HTMLElement): HTMLElement {
  return within(indicator).getByText("Server unreachable");
}

// jsdom applies no Tailwind CSS; the state attribute is what the styles key on.
function tooltipState(indicator: HTMLElement): string | undefined {
  return tooltipOf(indicator).dataset["state"];
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

  it("shows the empty libraries section with the configuration hint", async () => {
    renderApp("/");

    const sidebar = await findSidebar();

    expect(within(sidebar).getByRole("heading", { level: 2, name: "Libraries" })).toBeTruthy();
    expect(await within(sidebar).findByText("No libraries yet")).toBeTruthy();
    expect(
      within(sidebar).getByText(
        `Add folders under libraries in config.yaml or set ${productId.toUpperCase()}_LIBRARIES, then restart ${productName}.`,
      ),
    ).toBeTruthy();
  });

  it("links every configured library with the icon of its kind", async () => {
    const movies = fakeLibrary({ id: "lib-movies", name: "Movies", kind: "movies" });
    const shows = fakeLibrary({ id: "lib-shows", name: "Shows", kind: "shows", available: false });
    renderApp("/", { fetch: createFakeServer([librariesRoute([movies, shows])]).fetch });

    const sidebar = await findSidebar();
    const moviesLink = await within(sidebar).findByRole("link", { name: "Movies" });
    const showsLink = within(sidebar).getByRole("link", { name: "Shows, Unavailable" });

    expect(moviesLink.getAttribute("href")).toBe("/libraries/lib-movies");
    expect(showsLink.getAttribute("href")).toBe("/libraries/lib-shows");
    expect(moviesLink.querySelector("[data-icon]")?.getAttribute("data-icon")).toBe("movies");
    expect(showsLink.querySelector("[data-icon]")?.getAttribute("data-icon")).toBe("shows");
  });

  it("shows the generic library icon for a kind this client does not know", async () => {
    const music = fakeLibrary({ name: "Music", kind: "music" as LibraryKind });
    renderApp("/", { fetch: createFakeServer([librariesRoute([music])]).fetch });

    const link = await within(await findSidebar()).findByRole("link", { name: "Music" });

    expect(link.querySelector("svg")).not.toBeNull();
    expect(link.querySelector("[data-icon]")).toBeNull();
  });

  it("keeps the layout when the library list has the wrong shape", async () => {
    renderApp("/", { fetch: respondWithJson(fakeSystemInfo, 200) });

    const sidebar = await findSidebar();

    expect(await within(sidebar).findByText("No libraries yet")).toBeTruthy();
    expect(screen.getByRole("heading", { level: 1, name: "Home" })).toBeTruthy();
  });

  it("says when the libraries cannot be loaded", async () => {
    renderApp("/", { fetch: respondWithProblem(internalError) });

    const sidebar = await findSidebar();

    expect(await within(sidebar).findByText("Libraries could not be loaded")).toBeTruthy();
  });

  it("translates the libraries section into German", async () => {
    renderApp("/", { languages: ["de"] });

    const sidebar = await screen.findByRole("navigation", { name: "Hauptnavigation" });

    expect(within(sidebar).getByText("Bibliotheken werden geladen")).toBeTruthy();
    expect(await within(sidebar).findByText("Noch keine Bibliotheken")).toBeTruthy();
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
    expect(await within(sidebar).findByText("No libraries yet")).toBeTruthy();
    const version = await within(sidebar).findByText(`Server version ${fakeSystemInfo.version}`);
    expect(isVisuallyHidden(version)).toBe(true);
  });

  it("keeps a visible, focusable unreachable indicator while collapsed", async () => {
    const { indicator } = await renderCollapsedUnreachable();

    expect(isVisuallyHidden(indicator)).toBe(false);
    act(() => {
      indicator.focus();
    });
    expect(document.activeElement).toBe(indicator);
  });

  it("announces that the unreachable indicator expands the sidebar", async () => {
    const { sidebar, indicator } = await renderCollapsedUnreachable();

    expect(indicator.getAttribute("aria-expanded")).toBe("false");
    expect(indicator.getAttribute("aria-controls")).toBe(sidebar.id);
  });

  it("expands the sidebar from the unreachable indicator and keeps keyboard focus", async () => {
    const { sidebar, indicator } = await renderCollapsedUnreachable();
    const status = within(sidebar).getByRole("status");

    fireEvent.click(indicator);

    expect(sidebarToggle().getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(sidebarToggle());
    expect(window.localStorage.getItem(sidebarCollapsedStorageKey)).toBe("false");
    const message = within(status).getByText("Server unreachable");
    expect(isVisuallyHidden(message)).toBe(false);
    expect(within(status).queryByRole("button")).toBeNull();
  });

  it("announces the loading version while collapsed", async () => {
    window.localStorage.setItem(sidebarCollapsedStorageKey, "true");

    renderApp("/", { fetch: respondNever() });
    const status = within(await findSidebar()).getByRole("status");

    expect(status.textContent).toBe("Loading server version");
    expect(within(status).queryByRole("button")).toBeNull();
  });
});

describe("Unreachable indicator tooltip", () => {
  it("is closed until the indicator is hovered or focused", async () => {
    const { indicator } = await renderCollapsedUnreachable();

    expect(tooltipState(indicator)).toBe("closed");
  });

  it("opens on focus and closes on Escape without moving focus", async () => {
    const { indicator } = await renderCollapsedUnreachable();

    act(() => {
      indicator.focus();
    });
    expect(tooltipState(indicator)).toBe("open");
    fireEvent.keyDown(indicator, { key: "Escape" });

    expect(tooltipState(indicator)).toBe("closed");
    expect(document.activeElement).toBe(indicator);
    expect(sidebarToggle().getAttribute("aria-expanded")).toBe("false");
  });

  it("stays open while the pointer rests on the tooltip text", async () => {
    const { indicator } = await renderCollapsedUnreachable();

    fireEvent.mouseEnter(tooltipOf(indicator));

    expect(tooltipState(indicator)).toBe("open");
  });

  it("closes on Escape while hovered, even when focus is elsewhere", async () => {
    const { indicator } = await renderCollapsedUnreachable();
    fireEvent.mouseEnter(indicator);

    fireEvent.keyDown(document.body, { key: "Escape" });

    expect(tooltipState(indicator)).toBe("closed");
    fireEvent.mouseLeave(indicator);
    fireEvent.mouseEnter(indicator);
    expect(tooltipState(indicator)).toBe("open");
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

describe("Sidebar on a narrow screen", () => {
  function expanded(): boolean {
    return sidebarToggle().getAttribute("aria-expanded") === "true";
  }

  async function renderNarrowWithOpenSidebar(path = "/") {
    stubMediaQueries(true);
    const movies = fakeLibrary({ id: "lib-movies", name: "Movies" });
    const rendered = renderApp(path, {
      fetch: createFakeServer([librariesRoute([movies]), libraryRoute(movies)]).fetch,
    });
    const sidebar = await findSidebar();
    await within(sidebar).findByRole("link", { name: "Movies" });
    fireEvent.click(sidebarToggle());
    expect(expanded()).toBe(true);
    return { ...rendered, sidebar };
  }

  it("starts collapsed, even when the wide-screen preference is expanded", async () => {
    stubMediaQueries(true);
    window.localStorage.setItem(sidebarCollapsedStorageKey, "false");

    renderApp("/");
    await findSidebar();

    expect(expanded()).toBe(false);
  });

  it("opens as an overlay without saving it as the preference", async () => {
    await renderNarrowWithOpenSidebar();

    expect(window.localStorage.getItem(sidebarCollapsedStorageKey)).toBeNull();
    fireEvent.click(sidebarToggle());
    expect(expanded()).toBe(false);
    expect(window.localStorage.getItem(sidebarCollapsedStorageKey)).toBeNull();
  });

  it("closes on Escape and returns focus to the toggle", async () => {
    await renderNarrowWithOpenSidebar();
    act(() => {
      screen.getByRole("link", { name: "Home" }).focus();
    });

    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });

    expect(expanded()).toBe(false);
    expect(document.activeElement).toBe(sidebarToggle());
  });

  it("leaves an Escape that another control already handled alone", async () => {
    await renderNarrowWithOpenSidebar();
    const claimEscape = (event: KeyboardEvent) => {
      event.preventDefault();
    };
    document.body.addEventListener("keydown", claimEscape);

    fireEvent.keyDown(document.body, { key: "Escape" });

    document.body.removeEventListener("keydown", claimEscape);
    expect(expanded()).toBe(true);
  });

  it("closes on a press outside, but not on a press inside", async () => {
    const { sidebar } = await renderNarrowWithOpenSidebar();

    fireEvent.pointerDown(within(sidebar).getByText("Libraries"));
    expect(expanded()).toBe(true);

    fireEvent.pointerDown(screen.getByRole("main"));
    expect(expanded()).toBe(false);
  });

  it("closes when keyboard focus moves out of the navigation", async () => {
    const { sidebar } = await renderNarrowWithOpenSidebar();
    act(() => {
      within(sidebar).getByRole("link", { name: "Home" }).focus();
    });
    act(() => {
      within(sidebar).getByRole("link", { name: "Movies" }).focus();
    });
    expect(expanded()).toBe(true);

    act(() => {
      screen.getByRole("searchbox", { name: "Search" }).focus();
    });

    expect(expanded()).toBe(false);
  });

  it("stays open when the window loses focus", async () => {
    const { sidebar } = await renderNarrowWithOpenSidebar();
    const homeLink = within(sidebar).getByRole("link", { name: "Home" });
    act(() => {
      homeLink.focus();
    });

    fireEvent.focusOut(homeLink, { relatedTarget: null });

    expect(expanded()).toBe(true);
  });

  it("closes when a link in it opens another page", async () => {
    const { sidebar, router } = await renderNarrowWithOpenSidebar();

    fireEvent.click(within(sidebar).getByRole("link", { name: "Movies" }));

    await screen.findByRole("heading", { level: 1, name: "Movies" });
    expect(router.state.location.pathname).toBe("/libraries/lib-movies");
    expect(expanded()).toBe(false);
  });

  it("does not react to Escape or outside presses on a wide screen", async () => {
    renderApp("/");
    await findSidebar();

    fireEvent.keyDown(document.body, { key: "Escape" });
    fireEvent.pointerDown(screen.getByRole("main"));

    expect(expanded()).toBe(true);
  });

  it("switches between overlay and preference when the window is resized", async () => {
    const screenWidth = stubMediaQueries(false);
    renderApp("/");
    await findSidebar();
    expect(expanded()).toBe(true);

    act(() => {
      screenWidth.setMatches(true);
    });
    expect(expanded()).toBe(false);
    fireEvent.click(sidebarToggle());
    expect(expanded()).toBe(true);

    act(() => {
      screenWidth.setMatches(false);
    });
    expect(expanded()).toBe(true);
    act(() => {
      screenWidth.setMatches(true);
    });
    expect(expanded()).toBe(false);
    expect(window.localStorage.getItem(sidebarCollapsedStorageKey)).toBeNull();
  });

  it("opens the overlay from the unreachable indicator without saving it", async () => {
    stubMediaQueries(true);
    renderApp("/", { fetch: respondWithProblem(internalError) });
    const sidebar = await findSidebar();
    const indicator = await within(within(sidebar).getByRole("status")).findByRole("button", {
      name: "Server unreachable",
    });

    fireEvent.click(indicator);

    expect(expanded()).toBe(true);
    expect(document.activeElement).toBe(sidebarToggle());
    expect(window.localStorage.getItem(sidebarCollapsedStorageKey)).toBeNull();
  });
});
