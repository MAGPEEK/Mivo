import type { ItemSummary } from "@mivo/api-client";
import { fireEvent, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
  type FakeServer,
} from "../../../test/fakeApi";
import { fakeItem, fakeLibrary, itemsRoute, libraryRoute } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";

const library = fakeLibrary();
const matchedItem = fakeItem({ id: "matched", title: "Matched Film" });
const notImplementedStatus = 501;

function renderLibrary(itemsRouteOverride: FakeRoute): FakeServer {
  const server = createFakeServer([libraryRoute(library), itemsRouteOverride]);
  renderApp(`/libraries/${library.id}`, { fetch: server.fetch });
  return server;
}

// Answers the "Unmatched" filter with `unmatched` and every other request with one matched item.
function filteredItemsRoute(answerFiltered: () => Response): FakeRoute {
  const all = itemsRoute(library.id, [matchedItem]);
  return {
    ...all,
    respond: (request) =>
      request.url.searchParams.has("matchState") ? answerFiltered() : all.respond(request),
  };
}

function unmatchedRoute(unmatched: readonly ItemSummary[]): FakeRoute {
  return filteredItemsRoute(() => jsonResponse({ items: unmatched }));
}

// Opens the filter menu and chooses a quick filter; the menu button names the active one.
async function chooseQuick(name: string): Promise<void> {
  fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));
  fireEvent.click(await screen.findByRole("menuitemradio", { name }));
}

async function chooseUnmatched(): Promise<void> {
  await screen.findByText("Matched Film");
  await chooseQuick("Unmatched");
}

function filteredRequests(server: FakeServer): URL[] {
  return server.requests
    .map((request) => new URL(request.url))
    .filter((url) => url.searchParams.has("matchState"));
}

describe("LibraryPage unmatched filter", () => {
  it("lists only unmatched items with matchState=unmatched and badges them", async () => {
    const lost = fakeItem({ id: "lost", title: "Lost Film", matchState: "unmatched" });
    const server = renderLibrary(unmatchedRoute([lost]));

    await chooseUnmatched();

    // The filter goes through the URL, so the old list stays until the navigation has run.
    await screen.findByText("Lost Film");
    const list = screen.getByRole("list", { name: "Videos" });
    expect(within(list).queryByText("Matched Film")).toBeNull();
    expect(within(list).getByText("Unmatched")).toBeTruthy();
    expect(filteredRequests(server).map((url) => url.searchParams.get("matchState"))).toEqual([
      "unmatched",
    ]);
  });

  it("lists every item again under All", async () => {
    renderLibrary(unmatchedRoute([]));
    await chooseUnmatched();
    await screen.findByText("Everything is matched");

    await chooseQuick("All");

    expect(await screen.findByText("Matched Film")).toBeTruthy();
  });

  it("explains a server that cannot filter yet (501) without a retry", async () => {
    const server = renderLibrary(
      filteredItemsRoute(() => problemResponse(notImplementedStatus, "not_implemented")),
    );

    await chooseUnmatched();

    expect((await screen.findByRole("alert")).textContent).toContain(
      "This server cannot sort or filter the list this way yet.",
    );
    expect(filteredRequests(server)).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
  });
});
