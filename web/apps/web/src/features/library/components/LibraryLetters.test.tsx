import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
  type FakeServer,
} from "../../../test/fakeApi";
import { fakeItems, fakeLibrary, itemsRoute, libraryRoute } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";

const library = fakeLibrary();
const libraryPath = `/libraries/${library.id}`;
const lettersApiPath = `/api/v1/libraries/${library.id}/letters`;
const barName = "Jump to letter";

const letterIndex = {
  totalCount: 3,
  letters: [
    { letter: "#", offset: 0 },
    { letter: "B", offset: 2 },
  ],
};

function lettersRoute(): FakeRoute {
  return { method: "GET", path: lettersApiPath, respond: () => jsonResponse(letterIndex) };
}

function renderAt(search: string, lettersRoutes: readonly FakeRoute[]): FakeServer {
  const server = createFakeServer([
    libraryRoute(library),
    itemsRoute(library.id, fakeItems(3)),
    ...lettersRoutes,
  ]);
  renderApp(`${libraryPath}${search}`, { fetch: server.fetch });
  return server;
}

describe("LibraryPage A-Z bar", () => {
  it("shows the bar for the title sort, with the letters the server reported", async () => {
    const server = renderAt("", [lettersRoute()]);

    const bar = await screen.findByRole("navigation", { name: barName });
    expect(within(bar).getByRole("button", { name: "B" }).hasAttribute("disabled")).toBe(false);
    expect(within(bar).getByRole("button", { name: "C" }).hasAttribute("disabled")).toBe(true);
    expect(server.countRequests("GET", lettersApiPath)).toBe(1);
  });

  it("passes the order and filters of the listing, not its sort", async () => {
    const server = renderAt("?order=desc&genre=Drama", [lettersRoute()]);

    await screen.findByRole("navigation", { name: barName });
    const request = new URL(server.requests.find((r) => r.url.includes("/letters"))?.url ?? "");
    expect(request.searchParams.get("order")).toBe("desc");
    expect(request.searchParams.get("genre")).toBe("Drama");
    expect(request.searchParams.has("sort")).toBe(false);
  });

  it("has no bar for another sort and does not ask for letters", async () => {
    const server = renderAt("?sort=year", [lettersRoute()]);

    await screen.findByRole("link", { name: "Item 1" });

    expect(screen.queryByRole("navigation", { name: barName })).toBeNull();
    expect(server.countRequests("GET", lettersApiPath)).toBe(0);
  });

  it("has no bar when the server cannot index letters yet", async () => {
    const server = renderAt("", [
      {
        method: "GET",
        path: lettersApiPath,
        respond: () => problemResponse(501, "not_implemented"),
      },
    ]);

    await screen.findByRole("link", { name: "Item 1" });

    expect(screen.queryByRole("navigation", { name: barName })).toBeNull();
    expect(server.countRequests("GET", lettersApiPath)).toBe(1);
  });

  it("speaks German", async () => {
    const server = createFakeServer([
      libraryRoute(library),
      itemsRoute(library.id, fakeItems(3)),
      lettersRoute(),
    ]);
    renderApp(libraryPath, { fetch: server.fetch, languages: ["de"] });

    const bar = await screen.findByRole("navigation", { name: "Zu Buchstabe springen" });
    expect(within(bar).getByRole("button", { name: "Zahlen und Sonderzeichen" })).toBeTruthy();
  });
});
