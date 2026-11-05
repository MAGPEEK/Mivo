import type { ItemDetail, ItemSummary } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

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

const library = fakeLibrary({ name: "Audiobooks", kind: "audiobooks" });
const libraryPath = `/libraries/${library.id}`;
// More than one page of the Browse listing.
const manyBooks = 150;
const notFoundStatus = 404;
const serverErrorStatus = 500;

function book(id: string, fields: Partial<ItemSummary> = {}): ItemSummary {
  return { id, libraryId: library.id, kind: "audiobook", title: `Book ${id}`, ...fields };
}

const orchard = book("book-1", {
  title: "The Clockwork Orchard",
  year: 2021,
  poster: { id: "cover-1", tag: "tag-1" },
  media: { available: true, durationSeconds: 3_725 },
});
const lighthouse = book("book-2", { title: "The Paper Lighthouse" });

const details: readonly ItemDetail[] = [
  {
    id: orchard.id,
    libraryId: library.id,
    kind: "audiobook",
    title: orchard.title,
    people: [
      { name: "Ada Inkwell", role: "author" },
      { name: "Nora Vale", role: "narrator" },
    ],
    files: [],
  },
  {
    id: lighthouse.id,
    libraryId: library.id,
    kind: "audiobook",
    title: lighthouse.title,
    people: [
      { name: "Bram Quill", role: "author" },
      { name: "Cleo Marsh", role: "author" },
    ],
    files: [],
  },
];

const detailRoute: FakeRoute = {
  method: "GET",
  path: "/api/v1/items/:itemId",
  respond: ({ params }) => {
    const detail = details.find((candidate) => candidate.id === params["itemId"]);
    return detail === undefined
      ? problemResponse(notFoundStatus, "not_found")
      : jsonResponse(detail);
  },
};

function renderBooks(
  routes: readonly FakeRoute[],
  languages = ["en"],
  path = libraryPath,
): FakeServer {
  const server = createFakeServer([libraryRoute(library), detailRoute, ...routes]);
  renderApp(path, { fetch: server.fetch, languages });
  return server;
}

describe("Audiobook grid", () => {
  it("lists books with 2:3 covers, their authors and length", async () => {
    const server = renderBooks([itemsRoute(library.id, [orchard, lighthouse])]);

    const list = await screen.findByRole("list", { name: "Audiobooks" });
    const card = within(list).getByRole("link", { name: "The Clockwork Orchard" });
    expect(card.getAttribute("href")).toBe("/items/book-1");
    expect(await within(list).findByText("Ada Inkwell")).toBeTruthy();
    expect(await within(list).findByText("Bram Quill and Cleo Marsh")).toBeTruthy();
    expect(within(list).getByText("1:02:05")).toBeTruthy();
    const cover = list.querySelector("img");
    expect(cover?.getAttribute("src")).toContain("/api/v1/images/cover-1");
    expect(cover?.parentElement?.className).toContain("aspect-2/3");
    expect(server.countRequests("GET", `/api/v1/items/${orchard.id}`)).toBe(1);
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("names the books in the library heading's count", async () => {
    renderBooks([itemsRoute(library.id, [orchard])]);

    expect(await screen.findByText("2 audiobooks")).toBeTruthy();
  });

  it("shows a card without an author line when the book cannot be loaded", async () => {
    const stranger = book("book-9", { title: "Unknown Author" });
    renderBooks([itemsRoute(library.id, [stranger])]);

    const list = await screen.findByRole("list", { name: "Audiobooks" });
    const card = within(list).getByRole("listitem");
    expect(within(card).getByRole("link", { name: "Unknown Author" })).toBeTruthy();
    await waitFor(() => {
      expect(card.textContent).toBe("Unknown Author");
    });
  });

  it("shows the empty state of a library without books", async () => {
    renderBooks([itemsRoute(library.id, [])]);

    expect(await screen.findByText("No audiobooks yet")).toBeTruthy();
    expect(screen.queryByRole("list", { name: "Videos" })).toBeNull();
  });

  it("tells an empty Unmatched result from an empty library", async () => {
    renderBooks([itemsRoute(library.id, [])], ["en"], `${libraryPath}?matchState=unmatched`);

    expect(await screen.findByText("Everything is matched")).toBeTruthy();
    expect(
      screen.getByText("Every audiobook of this library is matched with a provider entry."),
    ).toBeTruthy();
  });

  it("offers a retry after a failed load", async () => {
    let failing = true;
    renderBooks([
      {
        method: "GET",
        path: `/api/v1/libraries/${library.id}/items`,
        respond: () =>
          failing
            ? problemResponse(serverErrorStatus, "internal")
            : jsonResponse({ items: [orchard] }),
      },
    ]);

    expect(await screen.findByText("The audiobooks could not be loaded.")).toBeTruthy();
    failing = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(await screen.findByRole("list", { name: "Audiobooks" })).toBeTruthy();
  });

  it("loads more books and moves focus to the first new card", async () => {
    const many = Array.from({ length: manyBooks }, (_, index) => book(`book-${String(index)}`));
    renderBooks([itemsRoute(library.id, many)]);

    const list = await screen.findByRole("list", { name: "Audiobooks" });
    const firstPage = within(list).getAllByRole("listitem").length;
    expect(firstPage).toBeLessThan(manyBooks);
    fireEvent.click(screen.getByRole("button", { name: "Load more" }));

    await waitFor(() => {
      expect(document.activeElement?.textContent).toBe(`Book book-${String(firstPage)}`);
    });
  });

  it("speaks German", async () => {
    renderBooks([itemsRoute(library.id, [])], ["de"]);

    expect(await screen.findByText("Noch keine Hörbücher")).toBeTruthy();
  });
});
