import type { Library } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { createFakeServer, type FakeServer } from "../../../test/fakeApi";
import { fakeItems, fakeLibrary, itemsRoute, libraryRoute } from "../../../test/libraryFixtures";
import { stubViewportWidth } from "../../../test/mediaQuery";
import { renderApp, type RenderedApp } from "../../../test/renderApp";

const phoneWidth = 375;

interface RenderedBrowse extends RenderedApp {
  readonly server: FakeServer;
}

function renderBrowse(
  search = "",
  options: { library?: Library; languages?: readonly string[]; itemCount?: number } = {},
): RenderedBrowse {
  const library = options.library ?? fakeLibrary();
  const server = createFakeServer([
    libraryRoute(library),
    itemsRoute(library.id, fakeItems(options.itemCount ?? 2)),
  ]);
  const languages = options.languages ?? ["en"];
  return {
    ...renderApp(`/libraries/${library.id}${search}`, { fetch: server.fetch, languages }),
    server,
  };
}

// The listing requests, without the paging parameters.
function listQueries(server: FakeServer): Record<string, string>[] {
  return server.requests
    .map((request) => new URL(request.url))
    .filter((url) => url.pathname.endsWith("/items"))
    .map((url) => {
      const query = Object.fromEntries(url.searchParams);
      delete query["limit"];
      delete query["cursor"];
      return query;
    });
}

async function openToolbar(): Promise<HTMLElement> {
  return screen.findByRole("group", { name: "Browse options" });
}

describe("BrowseToolbar", () => {
  it("chooses a sort with the keyboard and keeps it in the URL", async () => {
    const { router, server } = renderBrowse();
    const toolbar = await openToolbar();
    const trigger = within(toolbar).getByRole("button", { name: "Sort: Title" });

    trigger.focus();
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    const menu = await screen.findByRole("menu");
    const year = within(menu).getByRole("menuitemradio", { name: "Year" });
    fireEvent.keyDown(document.activeElement ?? menu, { key: "ArrowDown" });
    expect(document.activeElement).toBe(year);
    // Enter on a focused menu item is the native button click (jsdom does not synthesise it).
    fireEvent.click(year);

    await waitFor(() => {
      expect(router.state.location.search).toEqual({ sort: "year" });
    });
    await waitFor(() => {
      expect(listQueries(server).at(-1)).toEqual({ sort: "year" });
    });
    expect(within(toolbar).getByRole("button", { name: "Sort: Year" })).toBeTruthy();
  });

  it("toggles the direction and leaves ascending out of the URL", async () => {
    const { router, server } = renderBrowse("?sort=dateAdded");
    const toolbar = await openToolbar();

    fireEvent.click(within(toolbar).getByRole("button", { name: "Sort direction: Ascending" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ sort: "dateAdded", order: "desc" });
    });
    await waitFor(() => {
      expect(listQueries(server).at(-1)).toEqual({ sort: "dateAdded", order: "desc" });
    });

    fireEvent.click(within(toolbar).getByRole("button", { name: "Sort direction: Descending" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ sort: "dateAdded" });
    });
  });

  it("keeps the seed of the random sort, so a reload shows the same order", async () => {
    const { router, server } = renderBrowse("?sort=random");
    const toolbar = await openToolbar();

    await waitFor(() => {
      expect(typeof router.state.location.search.seed).toBe("number");
    });
    const { sort, seed } = router.state.location.search;
    expect(sort).toBe("random");
    await waitFor(() => {
      expect(listQueries(server).at(-1)).toEqual({ sort: "random", seed: String(seed) });
    });
    expect(within(toolbar).queryByRole("button", { name: /^Sort direction/ })).toBeNull();
  });

  it("drops unknown values from the URL", async () => {
    const { router, server } = renderBrowse("?sort=rating&order=up&type=episode");
    await openToolbar();

    await waitFor(() => {
      expect(listQueries(server)).toEqual([{}]);
    });
    expect(router.state.location.search).toEqual({});
  });

  it("offers the types of the library kind and switches between them", async () => {
    const music = fakeLibrary({ kind: "music", name: "Music" });
    const { router, server } = renderBrowse("", { library: music });
    const toolbar = await openToolbar();

    fireEvent.click(within(toolbar).getByRole("button", { name: "Show: Artists" }));
    fireEvent.click(await screen.findByRole("menuitemradio", { name: "Albums" }));

    await waitFor(() => {
      expect(router.state.location.search).toEqual({ type: "album" });
    });
    await waitFor(() => {
      expect(listQueries(server).at(-1)).toEqual({ type: "album" });
    });
    expect(await within(toolbar).findByText("2 albums")).toBeTruthy();
  });

  it("has no type switch in a library with a single type", async () => {
    renderBrowse();
    const toolbar = await openToolbar();

    expect(within(toolbar).queryByRole("button", { name: /^Show:/ })).toBeNull();
  });

  it.each([
    { languages: ["en"], itemCount: 1, text: "1 movie" },
    { languages: ["en"], itemCount: 367, text: "367 movies" },
    { languages: ["de"], itemCount: 1, text: "1 Film" },
    { languages: ["de"], itemCount: 2, text: "2 Filme" },
  ])("counts $itemCount items in $languages", async ({ languages, itemCount, text }) => {
    renderBrowse("", { languages, itemCount });

    expect(await within(await screen.findByRole("group")).findByText(text)).toBeTruthy();
  });

  it("puts the controls into a sheet on phones", async () => {
    stubViewportWidth(phoneWidth);
    const { router } = renderBrowse();
    const toolbar = await openToolbar();
    expect(within(toolbar).queryByRole("button", { name: "Sort: Title" })).toBeNull();

    fireEvent.click(within(toolbar).getByRole("button", { name: "Filter and sort" }));
    const sheet = await screen.findByRole("dialog", { name: "Filter and sort" });
    fireEvent.click(within(sheet).getByRole("button", { name: "Sort: Title" }));
    fireEvent.click(await within(sheet).findByRole("menuitemradio", { name: "File size" }));

    await waitFor(() => {
      expect(router.state.location.search).toEqual({ sort: "fileSize" });
    });
  });

  it("restores the unmatched filter from the URL", async () => {
    const { server } = renderBrowse("?matchState=unmatched");
    const toolbar = await openToolbar();

    expect(within(toolbar).getByRole("button", { name: "Filter: Unmatched" })).toBeTruthy();
    await waitFor(() => {
      expect(listQueries(server)).toEqual([{ matchState: "unmatched" }]);
    });
  });

  it("has no serious or critical axe findings", async () => {
    renderBrowse("?sort=year&order=desc");
    await openToolbar();
    await screen.findByText("Item 1");

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});
