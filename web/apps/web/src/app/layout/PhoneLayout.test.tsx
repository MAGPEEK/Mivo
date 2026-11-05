import type { Library } from "@mivo/api-client";
import { act, fireEvent, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../test/axe";
import { createFakeServer, respondNever } from "../../test/fakeApi";
import { fakeLibrary, librariesRoute, libraryRoute } from "../../test/libraryFixtures";
import { stubViewportWidth } from "../../test/mediaQuery";
import { renderApp } from "../../test/renderApp";

// Representative widths: a phone below `sm`, a tablet between `sm` and `md`, a desktop.
const phoneWidth = 375;
const tabletWidth = 700;
const desktopWidth = 1280;

const movies = fakeLibrary({ id: "lib-movies", name: "Movies" });
const shows = fakeLibrary({ id: "lib-shows", name: "Shows", kind: "shows" });

function renderPhone(path = "/", libraries: readonly Library[] = [movies, shows]) {
  const viewport = stubViewportWidth(phoneWidth);
  const routes = [librariesRoute(libraries), ...libraries.map((library) => libraryRoute(library))];
  const rendered = renderApp(path, { fetch: createFakeServer(routes).fetch });
  return { ...rendered, viewport };
}

async function findBottomNav(): Promise<HTMLElement> {
  const navigation = await screen.findByRole("navigation", { name: "Main navigation" });
  await within(navigation).findByRole("button", { name: "Libraries" });
  return navigation;
}

function librariesTab(navigation: HTMLElement): HTMLElement {
  return within(navigation).getByRole("button", { name: "Libraries" });
}

async function openLibrariesSheet(): Promise<HTMLElement> {
  const navigation = await findBottomNav();
  fireEvent.click(librariesTab(navigation));
  return screen.getByRole("dialog", { name: "Libraries" });
}

describe("Phone layout", () => {
  it("replaces the sidebar with a bottom navigation of four tabs", async () => {
    renderPhone();
    const navigation = await findBottomNav();

    expect(screen.queryByRole("button", { name: "Toggle sidebar" })).toBeNull();
    expect(within(navigation).getByRole("link", { name: "Home" })).toBeTruthy();
    expect(within(navigation).getByRole("button", { name: "Search" })).toBeTruthy();
    expect(within(navigation).getByRole("link", { name: "Settings" })).toBeTruthy();
  });

  it.each([
    { name: "tablet", width: tabletWidth },
    { name: "desktop", width: desktopWidth },
  ])("keeps the sidebar and shows no bottom navigation on a $name", async ({ width }) => {
    stubViewportWidth(width);
    renderApp("/");

    await screen.findByRole("button", { name: "Toggle sidebar" });

    expect(screen.queryByRole("button", { name: "Libraries" })).toBeNull();
  });

  it("switches between sidebar and bottom navigation when the window is resized", async () => {
    const { viewport } = renderPhone();
    await findBottomNav();

    act(() => {
      viewport.setWidth(desktopWidth);
    });
    expect(screen.getByRole("button", { name: "Toggle sidebar" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Libraries" })).toBeNull();

    act(() => {
      viewport.setWidth(phoneWidth);
    });
    expect(screen.queryByRole("button", { name: "Toggle sidebar" })).toBeNull();
    expect(screen.getByRole("button", { name: "Libraries" })).toBeTruthy();
  });

  it("marks Home as the current page on the home route", async () => {
    renderPhone("/");
    const navigation = await findBottomNav();

    expect(
      within(navigation).getByRole("link", { name: "Home" }).getAttribute("aria-current"),
    ).toBe("page");
    expect(librariesTab(navigation).hasAttribute("aria-current")).toBe(false);
  });

  it("marks Libraries as current on a library page", async () => {
    renderPhone("/libraries/lib-movies");
    const navigation = await findBottomNav();
    await screen.findByRole("heading", { level: 1, name: "Movies" });

    expect(librariesTab(navigation).getAttribute("aria-current")).toBe("page");
    expect(
      within(navigation).getByRole("link", { name: "Home" }).hasAttribute("aria-current"),
    ).toBe(false);
  });

  it("marks Settings as current on the settings page", async () => {
    renderPhone("/admin/manage/libraries");
    const navigation = await findBottomNav();

    expect(
      within(navigation).getByRole("link", { name: "Settings" }).getAttribute("aria-current"),
    ).toBe("page");
  });

  it("gives every tab a touch target of at least 44 px", async () => {
    renderPhone();
    const navigation = await findBottomNav();

    // jsdom has no layout; min-h-11/min-w-11 are 2.75rem = 44 px in Tailwind's spacing scale.
    for (const tab of within(navigation).getAllByRole("listitem")) {
      const control = tab.firstElementChild;
      expect(control?.className).toContain("min-h-11");
      expect(control?.className).toContain("min-w-11");
    }
  });

  it("exposes the navigation height as a CSS variable and pads the content by it", async () => {
    renderPhone();
    await findBottomNav();

    const main = screen.getByRole("main");
    const shell = main.closest("[data-bottom-nav]");

    expect(shell?.className).toContain("[--bottom-nav-height:");
    expect(main.className).toContain("var(--bottom-nav-height)");
  });

  it("focuses the search field from the Search tab", async () => {
    renderPhone();
    const navigation = await findBottomNav();

    fireEvent.click(within(navigation).getByRole("button", { name: "Search" }));

    expect(document.activeElement).toBe(screen.getByRole("searchbox", { name: "Search" }));
  });

  it("has no serious or critical axe findings", async () => {
    renderPhone();
    await findBottomNav();

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

describe("Phone libraries sheet", () => {
  it("lists the libraries and links to all libraries", async () => {
    renderPhone();
    const navigation = await findBottomNav();
    const tab = librariesTab(navigation);
    expect(tab.getAttribute("aria-haspopup")).toBe("dialog");
    expect(tab.getAttribute("aria-expanded")).toBe("false");

    const sheet = await openLibrariesSheet();

    expect(tab.getAttribute("aria-expanded")).toBe("true");
    const movieLink = await within(sheet).findByRole("link", { name: "Movies" });
    expect(movieLink.getAttribute("href")).toBe("/libraries/lib-movies");
    expect(within(sheet).getByRole("link", { name: "Shows" })).toBeTruthy();
    expect(within(sheet).getByRole("link", { name: "All libraries" })).toBeTruthy();
  });

  it("opens a library and closes", async () => {
    const { router } = renderPhone();
    const sheet = await openLibrariesSheet();

    fireEvent.click(await within(sheet).findByRole("link", { name: "Shows" }));

    await screen.findByRole("heading", { level: 1, name: "Shows" });
    expect(router.state.location.pathname).toBe("/libraries/lib-shows");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("closes with its close button and returns focus to the tab", async () => {
    renderPhone();
    const navigation = await findBottomNav();
    act(() => {
      librariesTab(navigation).focus();
    });
    fireEvent.click(librariesTab(navigation));

    fireEvent.click(screen.getByRole("button", { name: "Close libraries" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(librariesTab(navigation));
  });

  it("says so when no library exists yet", async () => {
    renderPhone("/", []);
    const sheet = await openLibrariesSheet();

    expect(await within(sheet).findByText("No libraries yet")).toBeTruthy();
    expect(within(sheet).getByRole("link", { name: "All libraries" })).toBeTruthy();
  });

  it("shows the loading message while the libraries load", async () => {
    stubViewportWidth(phoneWidth);
    renderApp("/", { fetch: respondNever() });
    const sheet = await openLibrariesSheet();

    expect(within(sheet).getByText("Loading libraries")).toBeTruthy();
  });

  it("has no serious or critical axe findings while open", async () => {
    renderPhone();
    const sheet = await openLibrariesSheet();
    await within(sheet).findByRole("link", { name: "Movies" });

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("is translated into German", async () => {
    stubViewportWidth(phoneWidth);
    renderApp("/", {
      languages: ["de"],
      fetch: createFakeServer([librariesRoute([movies])]).fetch,
    });
    const navigation = await screen.findByRole("navigation", { name: "Hauptnavigation" });
    fireEvent.click(await within(navigation).findByRole("button", { name: "Bibliotheken" }));
    const sheet = screen.getByRole("dialog", { name: "Bibliotheken" });

    expect(within(sheet).getByRole("link", { name: "Alle Bibliotheken" })).toBeTruthy();
    expect(within(sheet).getByRole("button", { name: "Bibliotheken schließen" })).toBeTruthy();
    expect(within(navigation).getByRole("button", { name: "Suche" })).toBeTruthy();
  });
});
