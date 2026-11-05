import type { FacetList } from "@mivo/api-client";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
  type FakeServer,
} from "../../../test/fakeApi";
import { fakeItems, fakeLibrary, itemsRoute, libraryRoute } from "../../../test/libraryFixtures";
import { renderApp, type RenderedApp } from "../../../test/renderApp";

const library = fakeLibrary();
const facetsPath = `/api/v1/libraries/${library.id}/facets`;
const notImplementedStatus = 501;

const facetList: FacetList = {
  facets: [
    {
      field: "genre",
      values: [
        { value: "Comedy", count: 3 },
        { value: "Drama", count: 5 },
      ],
      truncated: false,
    },
    { field: "year", values: [], truncated: false },
    { field: "audioLanguage", values: [{ value: "ger", count: 2 }], truncated: false },
  ],
};

function facetsRoute(respond: FakeRoute["respond"]): FakeRoute {
  return { method: "GET", path: facetsPath, respond };
}

interface RenderedFilters extends RenderedApp {
  readonly server: FakeServer;
}

function renderFilters(
  facets: FakeRoute = facetsRoute(() => jsonResponse(facetList)),
  search = "",
): RenderedFilters {
  const server = createFakeServer([
    libraryRoute(library),
    itemsRoute(library.id, fakeItems(2)),
    facets,
  ]);
  return { ...renderApp(`/libraries/${library.id}${search}`, { fetch: server.fetch }), server };
}

function facetRequests(server: FakeServer): URL[] {
  return server.requests
    .map((request) => new URL(request.url))
    .filter((url) => url.pathname === facetsPath);
}

async function filterTrigger(): Promise<HTMLElement> {
  const toolbar = await screen.findByRole("group", { name: "Browse options" });
  return within(toolbar).getByRole("button", { name: /^Filter/ });
}

async function openFilters(): Promise<HTMLElement> {
  fireEvent.click(await filterTrigger());
  return screen.findByRole("menu", { name: "Filters" });
}

function keyDown(key: string): void {
  fireEvent.keyDown(document.activeElement ?? document.body, { key });
}

describe("FilterMenu", () => {
  it("is operable with the keyboard: open, drill into a field, toggle, back and close", async () => {
    const { router } = renderFilters();
    const trigger = await filterTrigger();

    trigger.focus();
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    const menu = await screen.findByRole("menu", { name: "Filters" });
    expect(document.activeElement).toBe(within(menu).getByRole("menuitemradio", { name: "All" }));

    // Type-ahead to the first field, then ArrowRight opens its values.
    const genre = await within(menu).findByRole("menuitem", { name: "Genre" });
    keyDown("G");
    expect(document.activeElement).toBe(genre);
    keyDown("ArrowRight");
    const values = await screen.findByRole("menu", { name: "Genre" });
    const comedy = within(values).getByRole("menuitemcheckbox", { name: "Comedy, 3 items" });
    expect(document.activeElement).toBe(comedy);

    // Enter on a focused item is the native button click; the menu stays open for more values.
    fireEvent.click(comedy);
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ genre: ["Comedy"] });
    });
    expect(comedy.getAttribute("aria-checked")).toBe("true");
    expect(document.activeElement).toBe(comedy);

    keyDown("ArrowLeft");
    const root = await screen.findByRole("menu", { name: "Filters" });
    expect(document.activeElement).toBe(
      within(root).getByRole("menuitem", { name: "Genre, 1 chosen" }),
    );

    keyDown("Escape");
    expect(screen.queryByRole("menu")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("chooses a quick filter, closes and lists with it", async () => {
    const { router, server } = renderFilters();
    const menu = await openFilters();

    fireEvent.click(within(menu).getByRole("menuitemradio", { name: "Dolby Atmos" }));

    await waitFor(() => {
      expect(router.state.location.search).toEqual({ quick: "atmos" });
    });
    expect(screen.queryByRole("menu")).toBeNull();
    expect(await screen.findByRole("button", { name: "Filter: Dolby Atmos" })).toBeTruthy();
    const listed = server.requests
      .map((request) => new URL(request.url))
      .filter((url) => url.pathname.endsWith("/items"))
      .at(-1);
    await waitFor(() => {
      expect(listed?.searchParams.get("quick") ?? "").toBe("atmos");
    });
  });

  it("offers only fields with values and names languages", async () => {
    renderFilters();
    const menu = await openFilters();

    await within(menu).findByRole("menuitem", { name: "Audio language" });
    expect(within(menu).queryByRole("menuitem", { name: "Year" })).toBeNull();
    expect(within(menu).queryByRole("menuitem", { name: "Container" })).toBeNull();

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Audio language" }));
    const values = await screen.findByRole("menu", { name: "Audio language" });
    expect(within(values).getByRole("menuitemcheckbox", { name: "German, 2 items" })).toBeTruthy();
  });

  it("loads the counts only once the menu is used and with the current filters", async () => {
    const { server } = renderFilters(
      undefined,
      `?quick=hdr&genre=${encodeURIComponent('["Drama"]')}`,
    );
    await filterTrigger();
    await screen.findByText("Item 1");
    expect(facetRequests(server)).toHaveLength(0);

    await openFilters();

    await waitFor(() => {
      expect(facetRequests(server)).toHaveLength(1);
    });
    const query = facetRequests(server)[0]?.searchParams;
    expect(query?.get("quick")).toBe("hdr");
    expect(query?.getAll("genre")).toEqual(["Drama"]);
    expect(query?.getAll("field")).toContain("subtitleLanguage");
  });

  it("refetches when another filter changes and ignores the older answer", async () => {
    const pending: ((list: FacetList) => void)[] = [];
    const { router, server } = renderFilters(
      facetsRoute(
        () =>
          new Promise<Response>((resolve) => {
            pending.push((list) => {
              resolve(jsonResponse(list));
            });
          }),
      ),
    );
    await openFilters();
    await waitFor(() => {
      expect(pending).toHaveLength(1);
    });

    await act(async () => {
      await router.navigate({ to: ".", search: { quick: "uhd" } });
    });
    await waitFor(() => {
      expect(pending).toHaveLength(2);
    });
    expect(facetRequests(server).at(-1)?.searchParams.get("quick")).toBe("uhd");

    const newer: FacetList = {
      facets: [{ field: "genre", values: [{ value: "Drama", count: 1 }], truncated: false }],
    };
    act(() => {
      pending[1]?.(newer);
    });
    fireEvent.click(await screen.findByRole("menuitem", { name: "Genre" }));
    const values = await screen.findByRole("menu", { name: "Genre" });
    expect(within(values).getByRole("menuitemcheckbox", { name: "Drama, 1 item" })).toBeTruthy();

    // The answer to the first query arrives last and must not replace the current counts.
    await act(async () => {
      pending[0]?.(facetList);
      await Promise.resolve();
    });
    expect(within(values).getByRole("menuitemcheckbox", { name: "Drama, 1 item" })).toBeTruthy();
    expect(within(values).queryByRole("menuitemcheckbox", { name: /^Comedy/ })).toBeNull();
  });

  it("explains a server that cannot count yet (501)", async () => {
    renderFilters(facetsRoute(() => problemResponse(notImplementedStatus, "not_implemented")));
    const menu = await openFilters();

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Genre" }));

    const values = await screen.findByRole("menu", { name: "Genre" });
    const status = await within(values).findByRole("menuitem", {
      name: "This server cannot count values yet.",
    });
    expect(status).toHaveProperty("disabled", true);
  });

  it("has no serious or critical axe findings in either view", async () => {
    renderFilters();
    const menu = await openFilters();
    await within(menu).findByRole("menuitem", { name: "Audio language" });

    expect(await blockingAxeViolations(document.body)).toEqual([]);

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Genre" }));
    await screen.findByRole("menu", { name: "Genre" });
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});
