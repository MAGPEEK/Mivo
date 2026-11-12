import type { ItemDetail, ItemSummary } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { createFakeServer, jsonResponse, problemResponse } from "../../../test/fakeApi";
import { hierarchyServer } from "../test/fakeHierarchyServer";
import { renderItemPage } from "../test/renderItemPage";

const libraryId = "library-1";
const serverErrorStatus = 500;

const albumDetail: ItemDetail = {
  id: "album-1",
  libraryId,
  kind: "album",
  title: "Two Disc Symphony",
  year: 2019,
  childCount: 4,
  artwork: { square: { id: "cover-1", tag: "tag-1" } },
  ancestors: [{ id: "artist-1", kind: "artist", title: "Generated Orchestra" }],
  // The server names the album itself; Play must start a track instead.
  playItemId: "album-1",
  files: [],
};

function trackSummary(id: string, fields: Partial<ItemSummary>): ItemSummary {
  return {
    id,
    libraryId,
    kind: "track",
    title: id,
    parentId: albumDetail.id,
    media: { available: true, container: "flac", durationSeconds: 2, audioCodec: "flac" },
    ...fields,
  };
}

const twoDiscTracks: readonly ItemSummary[] = [
  trackSummary("overture", { title: "Overture", discNumber: 1, number: 1 }),
  trackSummary("allegro", { title: "Allegro", discNumber: 1, number: 2 }),
  trackSummary("adagio", { title: "Adagio", discNumber: 2, number: 1 }),
  trackSummary("finale", {
    title: "Finale",
    discNumber: 2,
    number: 2,
    media: { available: false },
  }),
];

function renderAlbum(tracks: readonly ItemSummary[], languages = ["en"]) {
  const server = hierarchyServer({
    details: [albumDetail],
    children: { [albumDetail.id]: tracks },
  });
  return renderItemPage(`/items/${albumDetail.id}`, { fetch: server.fetch, languages });
}

describe("Album page", () => {
  it("shows cover, artist link, facts and the tracks grouped by disc", async () => {
    renderAlbum(twoDiscTracks);

    const heading = await screen.findByRole("heading", { level: 1, name: "Two Disc Symphony" });
    const header = heading.closest("article");
    expect(header?.querySelector("img")?.getAttribute("src")).toContain("/api/v1/images/cover-1");
    expect(screen.getByRole("link", { name: "Generated Orchestra" }).getAttribute("href")).toBe(
      "/items/artist-1",
    );
    expect(screen.getByText("2019 · 4 tracks")).toBeTruthy();

    const discOne = await screen.findByRole("list", { name: "Disc 1" });
    const discTwo = screen.getByRole("list", { name: "Disc 2" });
    expect(
      within(discOne)
        .getAllByRole("listitem")
        .map((row) => row.textContent),
    ).toEqual(["1Overture0:02", "2Allegro0:02"]);
    expect(within(discTwo).getByRole("link", { name: "Play Adagio" }).getAttribute("href")).toBe(
      "/items/adagio/play",
    );
    expect(within(discTwo).queryByRole("link", { name: "Play Finale" })).toBeNull();
    expect(within(discTwo).getByText("Unavailable")).toBeTruthy();
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("starts the first playable track from Play", async () => {
    const { router } = renderAlbum([
      trackSummary("broken", { title: "Broken", number: 1, media: { available: false } }),
      trackSummary("second", { title: "Second", number: 2 }),
    ]);

    const play = await screen.findByRole("link", { name: "Play" });
    expect(play.getAttribute("href")).toBe("/items/second/play");
    fireEvent.click(play);

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/items/second/play");
    });
  });

  it("orders by disc and number whatever order the server lists, unnumbered tracks last", async () => {
    renderAlbum([
      trackSummary("bonus", { title: "Bonus", discNumber: 1 }),
      trackSummary("adagio", { title: "Adagio", discNumber: 2, number: 1 }),
      trackSummary("allegro", { title: "Allegro", discNumber: 1, number: 2 }),
      trackSummary("overture", { title: "Overture", discNumber: 1, number: 1 }),
    ]);

    const discOne = await screen.findByRole("list", { name: "Disc 1" });
    expect(
      within(discOne)
        .getAllByRole("listitem")
        .map((row) => row.textContent),
    ).toEqual(["1Overture0:02", "2Allegro0:02", "Bonus0:02"]);
  });

  it("plays the album from a clicked track and queues the playable tracks", async () => {
    const { router } = renderAlbum(twoDiscTracks);

    fireEvent.click(await screen.findByRole("link", { name: "Play Allegro" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/items/allegro/play");
    });
    expect(router.state.location.state).toMatchObject({
      mivoQueue: { ids: ["overture", "allegro", "adagio"], shuffle: false },
    });
  });

  it("hands the queue to the player from Play", async () => {
    const { router } = renderAlbum(twoDiscTracks);

    fireEvent.click(await screen.findByRole("link", { name: "Play" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/items/overture/play");
    });
    expect(router.state.location.state).toMatchObject({
      mivoQueue: { ids: ["overture", "allegro", "adagio"], shuffle: false },
    });
  });

  it("shuffles the playable tracks from Shuffle", async () => {
    const { router } = renderAlbum(twoDiscTracks);
    await screen.findByRole("link", { name: "Play Overture" });

    fireEvent.click(screen.getByRole("button", { name: "Shuffle" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toMatch(/^\/items\/(overture|allegro|adagio)\/play$/);
    });
    expect(router.state.location.state).toMatchObject({
      mivoQueue: { ids: ["overture", "allegro", "adagio"], shuffle: true },
    });
  });

  it("keeps Shuffle off for an album without tracks", async () => {
    renderAlbum([]);
    await screen.findByText("This album has no tracks yet.");

    expect(screen.getByRole("button", { name: "Shuffle" }).getAttribute("aria-disabled")).toBe(
      "true",
    );
  });

  it("lists a one-disc album without disc headings", async () => {
    renderAlbum([trackSummary("only", { title: "Only", discNumber: 1, number: 1 })]);

    expect(await screen.findByRole("link", { name: "Play Only" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Disc 1" })).toBeNull();
  });

  it("explains a disabled Play for an album without tracks", async () => {
    renderAlbum([]);

    expect(await screen.findByText("This album has no tracks yet.")).toBeTruthy();
    const play = screen.getByRole("button", { name: "Play" });
    const descriptionId = play.getAttribute("aria-describedby") ?? "";
    expect(document.getElementById(descriptionId)?.textContent).toBe(
      "There is no track to play yet.",
    );
  });

  it("offers a retry when the tracks cannot be loaded", async () => {
    let failing = true;
    const server = createFakeServer([
      { method: "GET", path: "/api/v1/items/album-1", respond: () => jsonResponse(albumDetail) },
      {
        method: "GET",
        path: "/api/v1/items/album-1/children",
        respond: () =>
          failing
            ? problemResponse(serverErrorStatus, "internal")
            : jsonResponse({ items: [twoDiscTracks[0]] }),
      },
    ]);
    renderItemPage("/items/album-1", { fetch: server.fetch });

    expect(await screen.findByText("The tracks could not be loaded.")).toBeTruthy();
    const play = screen.getByRole("button", { name: "Play" });
    const descriptionId = play.getAttribute("aria-describedby") ?? "";
    expect(document.getElementById(descriptionId)?.textContent).toBe(
      "The tracks to play could not be loaded.",
    );
    failing = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(await screen.findByRole("link", { name: "Play Overture" })).toBeTruthy();
  });

  it("speaks German", async () => {
    renderAlbum(twoDiscTracks, ["de"]);

    expect(await screen.findByRole("list", { name: "Disc 2" })).toBeTruthy();
    expect(screen.getByText("2019 · 4 Titel")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Adagio abspielen" })).toBeTruthy();
  });
});
