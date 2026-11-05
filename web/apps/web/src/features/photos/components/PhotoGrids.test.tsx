import type { ItemSummary } from "@mivo/api-client";
import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { createFakeServer, type FakeServer } from "../../../test/fakeApi";
import { fakeLibrary, itemsRoute, libraryRoute } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";

const library = fakeLibrary({ name: "Pictures", kind: "photos", itemCount: 2 });
const libraryPath = `/libraries/${library.id}`;

function album(id: string, fields: Partial<ItemSummary> = {}): ItemSummary {
  return { id, libraryId: library.id, kind: "photo_album", title: `Album ${id}`, ...fields };
}

function photo(id: string, fields: Partial<ItemSummary> = {}): ItemSummary {
  return {
    id,
    libraryId: library.id,
    kind: "photo",
    title: `${id}.jpg`,
    thumbnailState: "ready",
    square: { id, tag: "tag-1" },
    ...fields,
  };
}

const holidays = album("album-1", {
  title: "Holidays 2024",
  leafCount: 3,
  square: { id: "photo-9", tag: "tag-9" },
});
const rawOnly = album("album-2", { title: "Raw Only", leafCount: 1 });

function renderPhotos(items: readonly ItemSummary[], path = libraryPath, languages = ["en"]) {
  const server: FakeServer = createFakeServer([
    libraryRoute(library),
    itemsRoute(library.id, items),
  ]);
  renderApp(path, { fetch: server.fetch, languages });
  return server;
}

describe("Photo library grids", () => {
  it("lists the albums with a square cover and their photo count", async () => {
    renderPhotos([holidays, rawOnly]);

    const list = await screen.findByRole("list", { name: "Photo albums" });
    const link = within(list).getByRole("link", { name: "Holidays 2024" });
    expect(link.getAttribute("href")).toBe("/items/album-1");
    expect(within(list).getByText("3 photos")).toBeTruthy();
    expect(within(list).getByText("1 photo")).toBeTruthy();
    const cover = list.querySelector("img");
    expect(cover?.getAttribute("src")).toContain("/api/v1/images/photo-9");
    expect(cover?.getAttribute("loading")).toBe("lazy");
    expect(cover?.parentElement?.className).toContain("aspect-square");
    // An album without a ready photo shows the photo placeholder.
    expect(list.querySelector('[data-icon="photos"]')).not.toBeNull();
    expect(screen.getByText("2 albums")).toBeTruthy();
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("shows every photo as a lazily loaded square tile named by title and date", async () => {
    renderPhotos(
      [
        photo("IMG_0101", { takenAt: "2024-07-14T09:30:00Z" }),
        photo("DSC_0300", { thumbnailState: "unsupported" }),
        photo("VID_0104", { media: { available: true, durationSeconds: 3 } }),
      ],
      `${libraryPath}?type=photo`,
    );

    const list = await screen.findByRole("list", { name: "Photos" });
    const tile = within(list).getByRole("link", { name: "IMG_0101.jpg, Jul 14, 2024" });
    expect(tile.getAttribute("href")).toBe("/items/IMG_0101");
    const image = tile.querySelector("img");
    expect(image?.getAttribute("src")).toContain("/api/v1/images/IMG_0101");
    expect(image?.getAttribute("loading")).toBe("lazy");

    const unsupported = within(list).getByRole("link", {
      name: "DSC_0300.jpg, cannot be shown",
    });
    expect(unsupported.querySelector("img")).toBeNull();
    expect(within(unsupported).getByText("Cannot be shown")).toBeTruthy();

    const video = within(list).getByRole("link", { name: "VID_0104.jpg, video" });
    expect(within(video).getByText("0:03")).toBeTruthy();
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("shows the empty state of a library without albums", async () => {
    renderPhotos([]);

    expect(await screen.findByText("No albums yet")).toBeTruthy();
  });

  it("speaks German", async () => {
    renderPhotos([holidays], libraryPath, ["de"]);

    const list = await screen.findByRole("list", { name: "Fotoalben" });
    expect(within(list).getByText("3 Fotos")).toBeTruthy();
  });
});
