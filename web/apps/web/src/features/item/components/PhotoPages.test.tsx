import type { ItemDetail, ItemSummary } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { hierarchyServer } from "../test/fakeHierarchyServer";
import { mediaFile } from "../test/fakeItemServer";
import { renderItemPage } from "../test/renderItemPage";
import { resetWindowScroll, scrollWindowToEnd } from "../../../test/scrollWindow";

const libraryId = "library-1";
// More than one page of an album's contents.
const manyPhotos = 130;

const holidays: ItemDetail = {
  id: "album-1",
  libraryId,
  kind: "photo_album",
  title: "Holidays 2024",
  childCount: 2,
  leafCount: 3,
  files: [],
};

const lakeShore: ItemSummary = {
  id: "album-2",
  libraryId,
  kind: "photo_album",
  title: "Lake Shore",
  parentId: "album-1",
  leafCount: 2,
};

function photoSummary(id: string, fields: Partial<ItemSummary> = {}): ItemSummary {
  return {
    id,
    libraryId,
    kind: "photo",
    title: `${id}.jpg`,
    parentId: "album-1",
    thumbnailState: "ready",
    square: { id, tag: "tag-1" },
    ...fields,
  };
}

const eiffel: ItemDetail = {
  id: "IMG_0101",
  libraryId,
  kind: "photo",
  title: "IMG_0101.jpg",
  ancestors: [{ id: "album-1", kind: "photo_album", title: "Holidays 2024" }],
  artwork: { square: { id: "IMG_0101", tag: "tag-1" } },
  thumbnailState: "ready",
  files: [mediaFile({ id: "jpeg", fileName: "IMG_0101.jpg" })],
  photo: {
    takenAt: "2024-07-14T09:30:00Z",
    takenAtSource: "exif",
    make: "Lumora",
    model: "LX-100",
    exposureTime: 0.004,
    fNumber: 5.6,
    width: 6000,
    height: 4000,
  },
};

function renderAlbum(children: readonly ItemSummary[], details: readonly ItemDetail[] = []) {
  const server = hierarchyServer({
    details: [holidays, ...details],
    children: { "album-1": children },
  });
  renderItemPage("/items/album-1", { fetch: server.fetch });
  return server;
}

function renderPhoto(photo: ItemDetail, languages = ["en"]) {
  const server = hierarchyServer({ details: [photo], children: {} });
  renderItemPage(`/items/${photo.id}`, { fetch: server.fetch, languages });
}

afterEach(resetWindowScroll);

describe("Photo album page", () => {
  it("shows the photo count, then the sub-albums and the photos as tiles", async () => {
    renderAlbum([lakeShore, photoSummary("IMG_0101", { takenAt: "2024-07-14T09:30:00Z" })]);

    expect(await screen.findByRole("heading", { level: 1, name: "Holidays 2024" })).toBeTruthy();
    expect(screen.getByText("3 photos")).toBeTruthy();
    const list = await screen.findByRole("list", { name: "Album contents" });
    const links = within(list).getAllByRole("link");
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "/items/album-2",
      "/items/IMG_0101",
    ]);
    expect(links[1]?.getAttribute("aria-label")).toBe("IMG_0101.jpg, Jul 14, 2024");
    expect(within(list).getByText("2 photos")).toBeTruthy();
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("pages through a large album without an order as the window scrolls to the end", async () => {
    const photos = Array.from({ length: manyPhotos }, (_, index) =>
      photoSummary(`photo-${String(index)}`),
    );
    const server = renderAlbum(photos);

    const list = await screen.findByRole("list", { name: "Album contents" });
    expect(within(list).getAllByRole("listitem").length).toBeLessThan(manyPhotos);
    scrollWindowToEnd();

    await waitFor(() => {
      expect(list.querySelector(`a[href="/items/photo-${String(manyPhotos - 1)}"]`)).not.toBeNull();
    });
    // The server lists albums in one order only and rejects order=desc.
    expect(
      server.requests.every((request) => !new URL(request.url).searchParams.has("order")),
    ).toBe(true);
  });

  it("tells an empty album", async () => {
    renderAlbum([]);

    expect(await screen.findByText("This album is empty")).toBeTruthy();
  });
});

describe("Photo page", () => {
  it("shows the photo large with its date, camera and exposure", async () => {
    renderPhoto(eiffel);

    expect(await screen.findByRole("heading", { level: 1, name: "IMG_0101.jpg" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Holidays 2024" }).getAttribute("href")).toBe(
      "/items/album-1",
    );
    const image = screen.getByRole("img", { name: "IMG_0101.jpg" });
    expect(image.getAttribute("src")).toContain("/api/v1/images/IMG_0101");
    expect(image.getAttribute("srcset")).toContain("width=1920");
    expect(image.style.aspectRatio).toBe("6000 / 4000");
    const details = screen.getByRole("region", { name: "Photo details" });
    expect(within(details).getByText("July 14, 2024 at 9:30 AM")).toBeTruthy();
    expect(within(details).getByText("Lumora LX-100")).toBeTruthy();
    expect(within(details).getByText("1/250 s · f/5.6")).toBeTruthy();
    expect(screen.queryByRole("link", { name: /Play/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Play/ })).toBeNull();
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("says when the server cannot render the photo and requests no image", async () => {
    renderPhoto({ ...eiffel, thumbnailState: "unsupported" });

    expect(await screen.findByText("This photo cannot be shown.")).toBeTruthy();
    expect(screen.queryByRole("img", { name: "IMG_0101.jpg" })).toBeNull();
  });

  it("says when the image fails to load", async () => {
    renderPhoto(eiffel);

    fireEvent.error(await screen.findByRole("img", { name: "IMG_0101.jpg" }));

    expect(await screen.findByText("The photo could not be loaded.")).toBeTruthy();
  });

  it("offers Play for a video of the library instead of a still", async () => {
    renderPhoto({
      ...eiffel,
      id: "VID_0104",
      title: "VID_0104.mp4",
      playItemId: "VID_0104",
      files: [mediaFile({ id: "mp4", fileName: "VID_0104.mp4", durationSeconds: 3 })],
    });

    expect(await screen.findByRole("link", { name: "Play" })).toBeTruthy();
    expect(screen.queryByRole("img", { name: "VID_0104.mp4" })).toBeNull();
  });

  it("speaks German", async () => {
    renderPhoto({ ...eiffel, photo: { takenAt: "2024-07-14T09:30:00Z", takenAtSource: "file" } }, [
      "de",
    ]);

    const details = await screen.findByRole("region", { name: "Fotodetails" });
    expect(within(details).getByText("14. Juli 2024 um 09:30 (Dateidatum)")).toBeTruthy();
    expect(within(details).getByText("Aufgenommen")).toBeTruthy();
  });
});
