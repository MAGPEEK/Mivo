import type { Row } from "@mivo/api-client";
import { fireEvent, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { createFakeServer } from "../../../test/fakeApi";
import { fakeItem, fakeLibrary, librariesRoute } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";
import { libraryLayoutStorageKey } from "../../sidebar-layout";
import {
  failingHomeRowsRoute,
  homeRowsRoute,
  moviesLibrary,
  showsLibrary,
  twoLibraryRows,
} from "../test/homeFixtures";

function serverWithRows(rows = twoLibraryRows) {
  return createFakeServer([librariesRoute([moviesLibrary, showsLibrary]), homeRowsRoute(rows)]);
}

async function rowRegion(name: string): Promise<HTMLElement> {
  return screen.findByRole("region", { name });
}

describe("HomePage", () => {
  it("leads to Settings › Libraries when no library exists", async () => {
    renderApp("/");

    const main = await screen.findByRole("main");

    expect(
      await within(main).findByRole("heading", { level: 2, name: "No libraries yet" }),
    ).toBeTruthy();
    const add = within(main).getByRole("link", { name: "Add library" });
    expect(add.getAttribute("href")).toContain("/admin/manage/libraries");
  });

  it("shows the empty state in German when the language is de", async () => {
    renderApp("/", { languages: ["de"] });

    const main = await screen.findByRole("main");

    expect(await within(main).findByRole("heading", { level: 1, name: "Startseite" })).toBeTruthy();
    expect(
      await within(main).findByRole("heading", { level: 2, name: "Noch keine Bibliotheken" }),
    ).toBeTruthy();
    expect(within(main).getByRole("link", { name: "Bibliothek hinzufügen" })).toBeTruthy();
  });

  it("renders one row per server row in server order with translated titles", async () => {
    renderApp("/", { fetch: serverWithRows().fetch });

    await rowRegion("Recently added in Shows");
    const headings = within(screen.getByRole("main"))
      .getAllByRole("heading", { level: 2 })
      .map((heading) => heading.textContent);
    expect(headings).toEqual(["Recently added in Movies", "Recently added in Shows"]);
    const movies = await rowRegion("Recently added in Movies");
    expect(within(movies).getByRole("link", { name: "Sintel" }).getAttribute("href")).toBe(
      "/items/movie-1",
    );
  });

  it("titles the rows in German", async () => {
    renderApp("/", { languages: ["de"], fetch: serverWithRows().fetch });

    expect(await rowRegion("Kürzlich hinzugefügt in Movies")).toBeTruthy();
  });

  it("shows the new-episodes count on grouped show and season cards", async () => {
    renderApp("/", { fetch: serverWithRows().fetch });

    const shows = await rowRegion("Recently added in Shows");
    const show = within(shows).getByRole("link", { name: "Cosmos Laundromat" });
    expect(show.getAttribute("aria-describedby")).not.toBeNull();
    expect(within(shows).getByText("3 new episodes")).toBeTruthy();
    expect(within(shows).getByRole("link", { name: "Elephants Dream" })).toBeTruthy();
    expect(within(shows).getByText("Season 2")).toBeTruthy();
    expect(within(shows).getByText("4 new episodes").className).toContain("sr-only");
  });

  it("labels episode cards with show, episode code and title, and names unknown rows", async () => {
    const episodesRow: Row = {
      id: "future:1",
      kind: "upNext" as Row["kind"],
      titleKey: "rows.upNext",
      cardFormat: "banner" as Row["cardFormat"],
      items: [
        fakeItem({
          id: "episode-1",
          kind: "episode",
          title: "The Pilot",
          number: 9,
          seasonNumber: 3,
          showTitle: "Elephants Dream",
        }),
      ],
    };
    renderApp("/", { fetch: serverWithRows({ rows: [episodesRow], hero: [] }).fetch });

    const row = await rowRegion("More to explore");
    expect(within(row).getByRole("link", { name: "Elephants Dream" })).toBeTruthy();
    expect(within(row).getByText("S3 · E9")).toBeTruthy();
    expect(within(row).getByText("The Pilot")).toBeTruthy();
    expect(within(row).queryByRole("link", { name: /See all/ })).toBeNull();
  });

  it("opens the Browse pivot with the row's sort from See all", async () => {
    renderApp("/", { fetch: serverWithRows().fetch });

    const shows = await rowRegion("Recently added in Shows");
    const seeAll = within(shows).getByRole("link", { name: "See all: Recently added in Shows" });
    const url = new URL(seeAll.getAttribute("href") ?? "", "http://app.test");
    expect(url.pathname).toBe(`/libraries/${showsLibrary.id}`);
    expect(Object.fromEntries(url.searchParams)).toEqual({
      view: "browse",
      sort: "dateAdded",
      order: "desc",
      type: "show",
    });
  });

  it("hides the rows of libraries unpinned in the sidebar", async () => {
    window.localStorage.setItem(
      libraryLayoutStorageKey,
      JSON.stringify({ version: 1, order: [], unpinned: [moviesLibrary.id] }),
    );
    renderApp("/", { fetch: serverWithRows().fetch });

    expect(await rowRegion("Recently added in Shows")).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Recently added in Movies" })).toBeNull();
  });

  it("shows the server's hint when the libraries have no items yet", async () => {
    const server = createFakeServer([
      librariesRoute([fakeLibrary()]),
      homeRowsRoute({ rows: [], hero: [], hint: "noItems" }),
    ]);
    renderApp("/", { fetch: server.fetch });

    expect(await screen.findByText(/have no titles yet/)).toBeTruthy();
  });

  it("shows loading skeletons while the rows load", async () => {
    const server = createFakeServer([
      librariesRoute([fakeLibrary()]),
      // Never answers, so the rows stay loading.
      { method: "GET", path: "/api/v1/home/rows", respond: () => new Promise(() => undefined) },
    ]);
    renderApp("/", { fetch: server.fetch });

    const loading = await screen.findByText("Loading rows");
    expect(loading.closest("[role=status]")).not.toBeNull();
  });

  it("offers a retry when the rows cannot be loaded", async () => {
    let healed = false;
    const server = createFakeServer([
      librariesRoute([moviesLibrary, showsLibrary]),
      failingHomeRowsRoute(twoLibraryRows, () => healed),
    ]);
    renderApp("/", { fetch: server.fetch });

    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Home could not be loaded")).toBeTruthy();
    healed = true;
    fireEvent.click(within(alert).getByRole("button", { name: "Try again" }));

    expect(await rowRegion("Recently added in Movies")).toBeTruthy();
  });

  it("has no blocking accessibility violations", async () => {
    renderApp("/", { fetch: serverWithRows().fetch });

    await rowRegion("Recently added in Shows");

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});
