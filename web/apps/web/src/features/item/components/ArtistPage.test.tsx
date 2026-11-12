import type { ItemDetail, ItemSummary } from "@mivo/api-client";
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { createFakeServer, jsonResponse, problemResponse } from "../../../test/fakeApi";
import { hierarchyServer } from "../test/fakeHierarchyServer";
import { renderItemPage } from "../test/renderItemPage";

const libraryId = "library-1";
const serverErrorStatus = 500;

const artistDetail: ItemDetail = {
  id: "artist-1",
  libraryId,
  kind: "artist",
  title: "Generated Orchestra",
  childCount: 3,
  files: [],
};

function albumSummary(id: string, fields: Partial<ItemSummary>): ItemSummary {
  return {
    id,
    libraryId,
    kind: "album",
    title: id,
    parentId: artistDetail.id,
    ...fields,
  };
}

const albums: readonly ItemSummary[] = [
  albumSummary("late", { title: "Late Work", year: 2020, square: { id: "cover-late", tag: "t" } }),
  albumSummary("undated", { title: "Undated" }),
  albumSummary("early", { title: "Early Work", year: 2001 }),
];

function renderArtist(children: readonly ItemSummary[], languages = ["en"]) {
  const server = hierarchyServer({
    details: [artistDetail],
    children: { [artistDetail.id]: children },
  });
  return renderItemPage(`/items/${artistDetail.id}`, { fetch: server.fetch, languages });
}

describe("Artist page", () => {
  it("lists the albums oldest first, undated last, with a placeholder for missing covers", async () => {
    renderArtist(albums);

    expect(
      await screen.findByRole("heading", { level: 1, name: "Generated Orchestra" }),
    ).toBeTruthy();
    const links = (await screen.findAllByRole("link")).filter((link) =>
      (link.getAttribute("href") ?? "").startsWith("/items/"),
    );
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "/items/early",
      "/items/late",
      "/items/undated",
    ]);
    const section = screen.getByRole("region", { name: "Albums" });
    // Only the album with a cover loads an image; the others show the 1:1 placeholder.
    expect(section.querySelectorAll("img")).toHaveLength(1);
    expect(section.querySelector("img")?.getAttribute("src")).toContain("cover-late");
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("offers no Play button for the whole artist", async () => {
    renderArtist(albums);
    await screen.findByRole("region", { name: "Albums" });

    expect(screen.queryByRole("link", { name: "Play" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Play" })).toBeNull();
  });

  it("shows a loading state while the albums load", async () => {
    const server = createFakeServer([
      { method: "GET", path: "/api/v1/items/artist-1", respond: () => jsonResponse(artistDetail) },
      {
        method: "GET",
        path: "/api/v1/items/artist-1/children",
        respond: () => new Promise<Response>(() => undefined),
      },
    ]);
    renderItemPage("/items/artist-1", { fetch: server.fetch });

    expect(await screen.findByText("Loading albums")).toBeTruthy();
  });

  it("loads further album pages with Load more", async () => {
    const server = createFakeServer([
      { method: "GET", path: "/api/v1/items/artist-1", respond: () => jsonResponse(artistDetail) },
      {
        method: "GET",
        path: "/api/v1/items/artist-1/children",
        respond: ({ url }) =>
          url.searchParams.has("cursor")
            ? jsonResponse({ items: [albums[2]] })
            : jsonResponse({ items: [albums[0]], nextCursor: "1" }),
      },
    ]);
    renderItemPage("/items/artist-1", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Load more" }));

    expect(await screen.findByRole("link", { name: /Early Work/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("says so when the artist has no albums", async () => {
    renderArtist([]);

    expect(await screen.findByText("This artist has no albums yet.")).toBeTruthy();
  });

  it("offers a retry when the albums cannot be loaded", async () => {
    let failing = true;
    const server = createFakeServer([
      { method: "GET", path: "/api/v1/items/artist-1", respond: () => jsonResponse(artistDetail) },
      {
        method: "GET",
        path: "/api/v1/items/artist-1/children",
        respond: () =>
          failing
            ? problemResponse(serverErrorStatus, "internal")
            : jsonResponse({ items: [albums[2]] }),
      },
    ]);
    renderItemPage("/items/artist-1", { fetch: server.fetch });

    expect(await screen.findByText("The albums could not be loaded.")).toBeTruthy();
    failing = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(await screen.findByRole("link", { name: /Early Work/ })).toBeTruthy();
  });

  it("speaks German", async () => {
    renderArtist([], ["de"]);

    expect(await screen.findByText("Dieser Künstler hat noch keine Alben.")).toBeTruthy();
  });
});
