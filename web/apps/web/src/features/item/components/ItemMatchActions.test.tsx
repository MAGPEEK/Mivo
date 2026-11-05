import type { ItemDetail } from "@mivo/api-client";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { libraryItemsQueryKey } from "../../library";
import { itemQueryKey } from "../api/itemQuery";
import { hdrMovie, itemServer } from "../test/fakeItemServer";
import { renderItemPage } from "../test/renderItemPage";

const episode: ItemDetail = { ...hdrMovie, id: "episode-1", kind: "episode", title: "Pilot" };
const show: ItemDetail = {
  id: "show-1",
  libraryId: "library-1",
  kind: "show",
  title: "Show",
  files: [],
};
const album: ItemDetail = { ...show, id: "album-1", kind: "album", title: "Album" };
const artist: ItemDetail = { ...show, id: "artist-1", kind: "artist", title: "Artist" };
const track: ItemDetail = { ...hdrMovie, id: "track-1", kind: "track", title: "Track" };

// Serves `item` until the remove-match call, `removed` from then on, like the real server.
function serverWithRemove(item: ItemDetail, removed: ItemDetail) {
  let current = item;
  return (request: Request) => {
    const path = new URL(request.url).pathname;
    if (request.method === "DELETE" && path === `/api/v1/items/${item.id}/match`) {
      current = removed;
      return Promise.resolve(
        new Response(JSON.stringify(removed), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
    return itemServer([current])(request);
  };
}

describe("ItemPage match actions", () => {
  it.each([hdrMovie, show, album, artist])("offers the overflow menu on a $kind", async (item) => {
    renderItemPage(`/items/${item.id}`, { fetch: itemServer([item]) });

    await screen.findByRole("heading", { level: 1, name: item.title });

    expect(screen.getByRole("button", { name: "More actions" })).toBeTruthy();
  });

  it.each([episode, track])("offers no match actions on a $kind", async (item) => {
    renderItemPage(`/items/${item.id}`, { fetch: itemServer([item]) });

    await screen.findByRole("heading", { level: 1, name: item.title });

    expect(screen.queryByRole("button", { name: "More actions" })).toBeNull();
  });

  it("shows the item the server answered after removing the match", async () => {
    const removed: ItemDetail = { ...hdrMovie, title: "big.buck.bunny", matchState: "disabled" };
    const { queryClient } = renderItemPage(`/items/${hdrMovie.id}`, {
      fetch: serverWithRemove(hdrMovie, removed),
    });
    await screen.findByRole("heading", { level: 1, name: hdrMovie.title });
    // A cached page of the library grid, as left behind by a visit to the library.
    const libraryPageKey = [...libraryItemsQueryKey(hdrMovie.libraryId), "all"];
    queryClient.setQueryData(libraryPageKey, { pages: [], pageParams: [] });

    fireEvent.click(screen.getByRole("button", { name: "More actions" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Remove match" }));
    fireEvent.click(await screen.findByRole("button", { name: "Remove match" }));

    await waitFor(() => {
      expect(queryClient.getQueryData(itemQueryKey(hdrMovie.id))).toEqual(removed);
    });
    expect(await screen.findByRole("heading", { level: 1, name: "big.buck.bunny" })).toBeTruthy();
    expect(queryClient.getQueryState(libraryPageKey)?.isInvalidated).toBe(true);
  });
});
