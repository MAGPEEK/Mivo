import type { ItemDetail, ItemSummary } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
  type FakeServer,
} from "../../../test/fakeApi";
import { fakeLibrary, itemsRoute, libraryRoute } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";
import { resetWindowScroll, scrollWindowToEnd } from "../../../test/scrollWindow";

const library = fakeLibrary({ name: "Music", kind: "music" });
const libraryPath = `/libraries/${library.id}`;
const albumsPath = `${libraryPath}?type=album`;
// More than one page of the Browse listing.
const manyAlbums = 150;
const itemsApiPath = `/api/v1/libraries/${library.id}/items`;
const serverErrorStatus = 500;

const artist: ItemDetail = {
  id: "artist-1",
  libraryId: library.id,
  kind: "artist",
  title: "Generated Orchestra",
  files: [],
};

function album(id: string, fields: Partial<ItemSummary> = {}): ItemSummary {
  return {
    id,
    libraryId: library.id,
    kind: "album",
    title: `Album ${id}`,
    parentId: artist.id,
    ...fields,
  };
}

const symphony = album("album-1", {
  title: "Two Disc Symphony",
  year: 2019,
  square: { id: "cover-1", tag: "tag-1" },
});
const sketches = album("album-2", { title: "Sketches" });

const artistRoute: FakeRoute = {
  method: "GET",
  path: "/api/v1/items/:itemId",
  respond: ({ params }) =>
    params["itemId"] === artist.id ? jsonResponse(artist) : problemResponse(404, "not_found"),
};

function renderMusicLibrary(
  routes: readonly FakeRoute[],
  languages = ["en"],
  path = albumsPath,
): FakeServer {
  const server = createFakeServer([libraryRoute(library), artistRoute, ...routes]);
  renderApp(path, { fetch: server.fetch, languages });
  return server;
}

async function findAlbumList(name = "Albums"): Promise<HTMLElement> {
  return screen.findByRole("list", { name });
}

afterEach(resetWindowScroll);

describe("Music library grid", () => {
  it("lists albums with square covers, artist and year, and asks for the artist once", async () => {
    const server = renderMusicLibrary([itemsRoute(library.id, [symphony, sketches])]);

    const list = await findAlbumList();
    const card = within(list).getByRole("link", { name: "Two Disc Symphony" });

    expect(card.getAttribute("href")).toBe("/items/album-1");
    await waitFor(() => {
      expect(within(list).getAllByText("Generated Orchestra")).toHaveLength(2);
    });
    expect(within(list).getByText("2019")).toBeTruthy();
    const cover = list.querySelector("img");
    expect(cover?.getAttribute("src")).toContain("/api/v1/images/cover-1");
    expect(cover?.parentElement?.className).toContain("aspect-square");
    expect(server.countRequests("GET", `/api/v1/items/${artist.id}`)).toBe(1);
    expect(server.requests.map(typeOf).filter((type) => type !== undefined)).toEqual(["album"]);
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("shows the empty state of a library without albums", async () => {
    renderMusicLibrary([itemsRoute(library.id, [])]);

    expect(await screen.findByText("No albums yet")).toBeTruthy();
    expect(screen.queryByRole("list", { name: "Videos" })).toBeNull();
  });

  it("tells an empty Unmatched result from a library without albums", async () => {
    renderMusicLibrary([itemsRoute(library.id, [])], ["en"], `${albumsPath}&matchState=unmatched`);

    expect(await screen.findByText("Everything is matched")).toBeTruthy();
    expect(screen.queryByText("No albums yet")).toBeNull();
  });

  it("offers a retry after a failed load and then shows the albums", async () => {
    let failing = true;
    renderMusicLibrary([
      {
        method: "GET",
        path: itemsApiPath,
        respond: () =>
          failing
            ? problemResponse(serverErrorStatus, "internal")
            : jsonResponse({ items: [symphony] }),
      },
    ]);

    expect(await screen.findByText("The albums could not be loaded.")).toBeTruthy();
    failing = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(await findAlbumList()).toBeTruthy();
  });

  it("loads more albums as the window scrolls to the end", async () => {
    const many = Array.from({ length: manyAlbums }, (_, index) => album(`album-${String(index)}`));
    renderMusicLibrary([itemsRoute(library.id, many)]);

    const list = await findAlbumList();
    expect(within(list).getAllByRole("listitem").length).toBeLessThan(manyAlbums);
    scrollWindowToEnd();

    expect(await within(list).findByText(`Album album-${String(manyAlbums - 1)}`)).toBeTruthy();
  });

  it("lists artists in the poster grid and switches to the albums with the type menu", async () => {
    const server = renderMusicLibrary([itemsRoute(library.id, [symphony])], ["en"], libraryPath);

    expect(await screen.findByRole("list", { name: "Videos" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Show: Artists" }));
    fireEvent.click(await screen.findByRole("menuitemradio", { name: "Albums" }));

    expect(await findAlbumList()).toBeTruthy();
    expect(server.requests.some((request) => typeOf(request) === "album")).toBe(true);
  });

  it("speaks German", async () => {
    renderMusicLibrary([itemsRoute(library.id, [])], ["de"]);

    expect(await screen.findByText("Noch keine Alben")).toBeTruthy();
  });
});

// The `type` of a library listing request; `undefined` for other requests.
function typeOf(request: Request): string | null | undefined {
  const url = new URL(request.url);
  return url.pathname === itemsApiPath ? url.searchParams.get("type") : undefined;
}
