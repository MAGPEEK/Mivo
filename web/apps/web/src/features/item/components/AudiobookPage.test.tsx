import type { ItemDetail } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { mediaFile } from "../test/fakeItemServer";
import { hierarchyServer } from "../test/fakeHierarchyServer";
import { renderItemPage } from "../test/renderItemPage";

const libraryId = "library-1";

// The single-file M4B of the corpus: three chapters of 2 s.
const m4bBook: ItemDetail = {
  id: "book-1",
  libraryId,
  kind: "audiobook",
  title: "The Clockwork Orchard",
  year: 2021,
  people: [
    { name: "Ada Inkwell", role: "author" },
    { name: "Nora Vale", role: "narrator" },
  ],
  artwork: { poster: { id: "cover-1", tag: "tag-1" } },
  playItemId: "book-1",
  files: [mediaFile({ id: "m4b", container: "mp4", durationSeconds: 6 })],
  chapters: [
    { title: "The Seed Vault", startMs: 0, endMs: 2_000, fileId: "m4b" },
    { title: "Brass Branches", startMs: 2_000, endMs: 4_000, fileId: "m4b" },
    { title: "Harvest Moon", startMs: 4_000, endMs: 6_000, fileId: "m4b" },
  ],
};

// A book of three MP3 parts on 25 min each, one chapter per part; the last part is unreadable.
const partLengthSeconds = 1_500;
const partsBook: ItemDetail = {
  id: "book-2",
  libraryId,
  kind: "audiobook",
  title: "The Paper Lighthouse",
  people: [
    { name: "Bram Quill", role: "author" },
    { name: "Iris Fenn", role: "narrator" },
  ],
  playItemId: "book-2",
  files: [
    mediaFile({ id: "storm", durationSeconds: partLengthSeconds }),
    mediaFile({ id: "keeper", durationSeconds: partLengthSeconds }),
    mediaFile({ id: "dawn", durationSeconds: partLengthSeconds, available: false }),
  ],
  chapters: [
    { title: "Storm", startMs: 0, endMs: 1_500_000, fileId: "storm" },
    { title: "Keeper", startMs: 0, endMs: 600_000, fileId: "keeper" },
    { title: "Keeper, continued", startMs: 600_000, endMs: 1_500_000, fileId: "keeper" },
    { title: "Dawn", startMs: 0, endMs: 1_500_000, fileId: "dawn" },
  ],
};

function renderBook(book: ItemDetail, languages = ["en"]) {
  const server = hierarchyServer({ details: [book], children: {} });
  return renderItemPage(`/items/${book.id}`, { fetch: server.fetch, languages });
}

async function findChapters(name = "Chapters"): Promise<HTMLElement> {
  return screen.findByRole("list", { name });
}

describe("Audiobook page", () => {
  it("shows cover, authors, narrator, length, chapter count and the chapters", async () => {
    renderBook(m4bBook);

    const heading = await screen.findByRole("heading", { level: 1, name: m4bBook.title });
    const header = heading.parentElement?.parentElement;
    expect(header?.querySelector("img")?.getAttribute("src")).toContain("/api/v1/images/cover-1");
    expect(header?.querySelector("img")?.parentElement?.className).toContain("aspect-2/3");
    expect(
      screen.getByText("By Ada Inkwell · Narrated by Nora Vale · 2021 · 6 s · 3 chapters"),
    ).toBeTruthy();

    const chapters = await findChapters();
    expect(
      within(chapters)
        .getAllByRole("listitem")
        .map((row) => row.textContent),
    ).toEqual([
      "1The Seed VaultStarts at 0:00Length 0:02",
      "2Brass BranchesStarts at 0:02Length 0:02",
      "3Harvest MoonStarts at 0:04Length 0:02",
    ]);
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("starts a chapter in the player at its offset in the file", async () => {
    const { router } = renderBook(m4bBook);

    const chapter = await screen.findByRole("link", { name: "Play Brass Branches" });
    expect(chapter.getAttribute("href")).toBe("/items/book-1/play?fileId=m4b&t=2");
    expect(screen.getByRole("link", { name: "Play The Seed Vault" }).getAttribute("href")).toBe(
      "/items/book-1/play?fileId=m4b",
    );
    fireEvent.click(chapter);

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/items/book-1/play");
    });
    expect(router.state.location.search).toEqual({ fileId: "m4b", t: 2 });
  });

  it("maps the chapters of a book of several parts to their file and local offset", async () => {
    renderBook(partsBook);

    const chapters = await findChapters();
    const continued = within(chapters).getByRole("link", { name: "Play Keeper, continued" });
    expect(continued.getAttribute("href")).toBe("/items/book-2/play?fileId=keeper&t=600");
    expect(within(chapters).getByRole("link", { name: "Play Keeper" }).getAttribute("href")).toBe(
      "/items/book-2/play?fileId=keeper",
    );
    // The listed start counts the earlier parts: 25 min of "Storm" plus 10 min into "Keeper".
    expect(within(chapters).getByText("Starts at 35:00")).toBeTruthy();
    expect(
      screen.getByText("By Bram Quill · Narrated by Iris Fenn · 1 h 15 min · 4 chapters"),
    ).toBeTruthy();
  });

  it("lists a chapter of an unreadable part without Play", async () => {
    renderBook(partsBook);

    const chapters = await findChapters();
    const rows = within(chapters).getAllByRole("listitem");
    const dawn = rows.at(-1);
    expect(dawn?.textContent).toContain("Unavailable");
    expect(within(chapters).queryByRole("link", { name: "Play Dawn" })).toBeNull();
  });

  it("says so when a book has no chapters", async () => {
    renderBook({ ...m4bBook, chapters: [] });

    expect(await screen.findByText("This audiobook has no chapters.")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Play" }).getAttribute("href")).toBe(
      "/items/book-1/play?fileId=m4b",
    );
  });

  it("speaks German", async () => {
    renderBook(m4bBook, ["de"]);

    expect(await findChapters("Kapitel")).toBeTruthy();
    expect(
      screen.getByText("Von Ada Inkwell · Gelesen von Nora Vale · 2021 · 6 Sek. · 3 Kapitel"),
    ).toBeTruthy();
    expect(screen.getByRole("link", { name: "Brass Branches abspielen" })).toBeTruthy();
    expect(screen.getAllByText("Beginnt bei 0:02")).toHaveLength(1);
  });
});
