import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { createFakeServer } from "../../../test/fakeApi";
import { fakeItems, fakeLibrary, itemsRoute, libraryRoute } from "../../../test/libraryFixtures";
import { renderApp, type RenderedApp } from "../../../test/renderApp";

const library = fakeLibrary();

// The router keeps arrays in the URL as JSON.
function listParam(name: string, values: readonly (string | number)[]): string {
  return `${name}=${encodeURIComponent(JSON.stringify(values))}`;
}

const filteredSearch = `?quick=hdr&${listParam("genre", ["Drama", "Comedy"])}&${listParam("decade", [1990])}`;

function renderChips(
  search: string,
  itemCount = 2,
  languages: readonly string[] = ["en"],
): RenderedApp {
  const server = createFakeServer([
    libraryRoute(library),
    itemsRoute(library.id, fakeItems(itemCount)),
  ]);
  return renderApp(`/libraries/${library.id}${search}`, { fetch: server.fetch, languages });
}

async function chips(): Promise<HTMLElement> {
  return screen.findByRole("group", { name: "Active filters" });
}

describe("FilterChips", () => {
  it("shows a chip per active filter", async () => {
    renderChips(filteredSearch);

    const group = await chips();
    expect(
      within(group)
        .getAllByRole("button")
        .map((button) => button.textContent),
    ).toEqual(["HDR", "Genre: Drama", "Genre: Comedy", "Decade: 1990s", "Clear all"]);
  });

  it("names decades and filters in German", async () => {
    renderChips(`?${listParam("decade", [1990])}`, 2, ["de"]);

    const group = await screen.findByRole("group", { name: "Aktive Filter" });
    expect(
      within(group).getByRole("button", { name: "Filter Jahrzehnt: 1990er entfernen" }),
    ).toBeTruthy();
  });

  it("removes a filter on click", async () => {
    const { router } = renderChips(filteredSearch);

    fireEvent.click(
      within(await chips()).getByRole("button", { name: "Remove filter Genre: Drama" }),
    );

    await waitFor(() => {
      expect(router.state.location.search).toEqual({
        quick: "hdr",
        genre: ["Comedy"],
        decade: ["1990"],
      });
    });
  });

  it("removes a filter with Delete and moves focus to the next chip", async () => {
    const { router } = renderChips(filteredSearch);
    const quick = within(await chips()).getByRole("button", { name: "Remove filter HDR" });

    quick.focus();
    fireEvent.keyDown(quick, { key: "Delete" });

    await waitFor(() => {
      expect(router.state.location.search).toEqual({
        genre: ["Drama", "Comedy"],
        decade: ["1990"],
      });
    });
    await waitFor(() => {
      expect(document.activeElement).toBe(
        within(screen.getByRole("group", { name: "Active filters" })).getByRole("button", {
          name: "Remove filter Genre: Drama",
        }),
      );
    });
  });

  it("clears every filter and keeps the sort", async () => {
    const { router } = renderChips(`${filteredSearch}&sort=year`);

    fireEvent.click(within(await chips()).getByRole("button", { name: "Clear all" }));

    await waitFor(() => {
      expect(router.state.location.search).toEqual({ sort: "year" });
    });
    expect(screen.queryByRole("group", { name: "Active filters" })).toBeNull();
  });

  it("has no chips without filters", async () => {
    renderChips("?sort=year");
    await screen.findByText("Item 1");

    expect(screen.queryByRole("group", { name: "Active filters" })).toBeNull();
  });

  it("explains an empty filtered result", async () => {
    renderChips(filteredSearch, 0);

    expect(await screen.findByText("Nothing matches these filters")).toBeTruthy();
  });

  it("has no serious or critical axe findings", async () => {
    renderChips(filteredSearch);
    await chips();
    await screen.findByText("Item 1");

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});
