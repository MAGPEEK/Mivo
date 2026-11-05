import type { LibraryRows, Row } from "@mivo/api-client";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
} from "../../../test/fakeApi";
import {
  fakeItem,
  fakeLibrary,
  fakeShow,
  itemsRoute,
  libraryRoute,
} from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";
import { pivotStorageKey } from "../lib/pivotMemory";

const library = fakeLibrary();
const libraryPath = `/libraries/${library.id}`;
const rowsApiPath = `/api/v1/libraries/${library.id}/rows`;

const recentlyAddedRow: Row = {
  id: `recentlyAdded:${library.id}`,
  kind: "recentlyAdded",
  titleKey: "rows.recentlyAdded",
  libraryId: library.id,
  cardFormat: "poster",
  seeAllQuery: { sort: "dateAdded", order: "desc" },
  items: [
    fakeItem({ id: "movie-1", title: "Sintel", year: 2010 }),
    fakeItem({ id: "movie-2", title: "Tears of Steel", year: 2012 }),
  ],
};

const genreRow: Row = {
  id: `moreGenre:${library.id}:Comedy`,
  kind: "moreGenre",
  titleKey: "rows.moreGenre",
  titleParams: { genre: "Comedy" },
  libraryId: library.id,
  cardFormat: "poster",
  items: [fakeShow({ id: "show-1", title: "Cosmos Laundromat", newEpisodeCount: 2 })],
};

function rowsRoute(rows: Omit<LibraryRows, "libraryId">): FakeRoute {
  return {
    method: "GET",
    path: rowsApiPath,
    respond: () => jsonResponse({ libraryId: library.id, ...rows } satisfies LibraryRows),
  };
}

function renderPivots(
  path: string,
  rows: FakeRoute = rowsRoute({ rows: [recentlyAddedRow, genreRow] }),
  languages: readonly string[] = ["en"],
) {
  const server = createFakeServer([
    libraryRoute(library),
    itemsRoute(library.id, [fakeItem()]),
    rows,
  ]);
  return { ...renderApp(path, { fetch: server.fetch, languages }), server };
}

function tab(name: string): HTMLElement {
  return screen.getByRole("tab", { name });
}

describe("LibraryPage pivots", () => {
  it("opens a library without a remembered pivot on Browse and loads no rows", async () => {
    const { server } = renderPivots(libraryPath);

    expect(await screen.findByRole("tab", { name: "Browse", selected: true })).toBeTruthy();
    expect(tab("Overview").getAttribute("aria-selected")).toBe("false");
    expect(await screen.findByRole("list", { name: "Videos" })).toBeTruthy();
    expect(server.countRequests("GET", rowsApiPath)).toBe(0);
    expect(window.localStorage.getItem(pivotStorageKey(library.id))).toBe("browse");
  });

  it("renders the server rows on Overview with their cards and See all into Browse", async () => {
    renderPivots(`${libraryPath}?view=overview`);

    const recent = await screen.findByRole("region", { name: "Recently added" });
    expect(within(recent).getByRole("link", { name: "Sintel" })).toBeTruthy();
    const genre = screen.getByRole("region", { name: "More Comedy" });
    expect(within(genre).getByText("2 new episodes")).toBeTruthy();
    expect(screen.queryByRole("list", { name: "Videos" })).toBeNull();

    const seeAll = within(recent).getByRole("link", { name: "See all: Recently added" });
    const url = new URL(seeAll.getAttribute("href") ?? "", "http://app.test");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      view: "browse",
      sort: "dateAdded",
      order: "desc",
    });
    fireEvent.click(seeAll);

    expect(await screen.findByRole("list", { name: "Videos" })).toBeTruthy();
    expect(tab("Browse").getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("button", { name: "Sort: Date added" })).toBeTruthy();
  });

  it("changes the pivot in the URL, keeps the Browse state and remembers the choice", async () => {
    const { router } = renderPivots(`${libraryPath}?sort=year`);
    await screen.findByRole("list", { name: "Videos" });

    fireEvent.click(tab("Overview"));

    await screen.findByRole("region", { name: "Recently added" });
    expect(router.state.location.search).toMatchObject({ view: "overview", sort: "year" });
    expect(window.localStorage.getItem(pivotStorageKey(library.id))).toBe("overview");

    fireEvent.click(tab("Browse"));

    expect(await screen.findByRole("button", { name: "Sort: Year" })).toBeTruthy();
    expect(router.state.location.search).toMatchObject({ view: "browse", sort: "year" });
  });

  it("keeps the pivot in the URL when the Browse toolbar changes the listing", async () => {
    const { router } = renderPivots(`${libraryPath}?view=browse`);
    await screen.findByRole("list", { name: "Videos" });

    fireEvent.click(screen.getByRole("button", { name: "Sort direction: Ascending" }));

    await waitFor(() => {
      expect(router.state.location.search).toMatchObject({ view: "browse", order: "desc" });
    });
  });

  it("opens the remembered pivot for a plain or invalid view", async () => {
    window.localStorage.setItem(pivotStorageKey(library.id), "overview");
    const { router } = renderPivots(`${libraryPath}?view=grid`);

    expect(await screen.findByRole("tab", { name: "Overview", selected: true })).toBeTruthy();
    expect(await screen.findByRole("region", { name: "Recently added" })).toBeTruthy();
    expect(router.state.location.search).not.toHaveProperty("view", "grid");
  });

  it("switches pivots with the arrow keys", async () => {
    renderPivots(libraryPath);
    const browse = await screen.findByRole("tab", { name: "Browse", selected: true });

    act(() => {
      browse.focus();
    });
    fireEvent.keyDown(browse, { key: "ArrowLeft" });

    expect(await screen.findByRole("tab", { name: "Overview", selected: true })).toBeTruthy();
  });
});

describe("LibraryPage Overview states", () => {
  it("shows the server's hint and a way to Browse when there is too little data", async () => {
    renderPivots(`${libraryPath}?view=overview`, rowsRoute({ rows: [], hint: "notEnoughData" }));

    expect(
      await screen.findByText(
        "Some rows need more data, for example metadata. Everything is under Browse.",
      ),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Open Browse" }));

    expect(await screen.findByRole("list", { name: "Videos" })).toBeTruthy();
  });

  it("shows the hint below the rows it accompanies", async () => {
    renderPivots(
      `${libraryPath}?view=overview`,
      rowsRoute({ rows: [recentlyAddedRow], hint: "notEnoughData" }),
    );

    expect(await screen.findByRole("region", { name: "Recently added" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open Browse" })).toBeTruthy();
  });

  it("explains an empty library instead of showing an empty page", async () => {
    renderPivots(`${libraryPath}?view=overview`, rowsRoute({ rows: [], hint: "noItems" }));

    expect(
      await screen.findByText(
        "This library has no titles yet. They appear here after the first scan.",
      ),
    ).toBeTruthy();
  });

  it("shows a generic hint without rows and without a reason", async () => {
    renderPivots(`${libraryPath}?view=overview`, rowsRoute({ rows: [] }));

    expect(
      await screen.findByText("Nothing to show here yet. Everything is under Browse."),
    ).toBeTruthy();
  });

  it("points to Browse when the server cannot build rows yet", async () => {
    const notImplemented: FakeRoute = {
      method: "GET",
      path: rowsApiPath,
      respond: () => problemResponse(501, "not_implemented"),
    };
    const { server } = renderPivots(`${libraryPath}?view=overview`, notImplemented);

    expect(
      await screen.findByText(
        "This server cannot show an overview yet. Everything is under Browse.",
      ),
    ).toBeTruthy();
    expect(server.countRequests("GET", rowsApiPath)).toBe(1);
  });

  it("offers a retry when the rows cannot be loaded", async () => {
    let failing = true;
    const flaky: FakeRoute = {
      method: "GET",
      path: rowsApiPath,
      respond: () =>
        failing
          ? problemResponse(500, "internal")
          : jsonResponse({ libraryId: library.id, rows: [recentlyAddedRow] }),
    };
    renderPivots(`${libraryPath}?view=overview`, flaky);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("The overview could not be loaded.");
    failing = false;
    fireEvent.click(within(alert).getByRole("button", { name: "Try again" }));

    expect(await screen.findByRole("region", { name: "Recently added" })).toBeTruthy();
  });

  it("shows loading rows while the rows load", async () => {
    const pending: FakeRoute = {
      method: "GET",
      path: rowsApiPath,
      respond: () => new Promise<Response>(() => undefined),
    };
    renderPivots(`${libraryPath}?view=overview`, pending);

    const panel = await screen.findByRole("tabpanel");
    expect((await within(panel).findByRole("status")).textContent).toBe("Loading rows");
  });

  it("renders the Overview in German", async () => {
    renderPivots(`${libraryPath}?view=overview`, undefined, ["de"]);

    expect(await screen.findByRole("region", { name: "Kürzlich hinzugefügt" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Übersicht", selected: true })).toBeTruthy();
  });

  it("has no serious or critical axe findings with rows and with a hint", async () => {
    renderPivots(`${libraryPath}?view=overview`, rowsRoute({ rows: [genreRow], hint: "scanning" }));
    await screen.findByRole("region", { name: "More Comedy" });

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});
