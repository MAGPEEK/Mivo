import type { ItemDetail, PlaybackRequest, PlaybackSession } from "@mivo/api-client";
import { expect, test as base, type Locator, type Page } from "@playwright/test";
import { join } from "node:path";

import { blockingAxeViolations } from "./support/axe";
import {
  audiobookCorpusExists,
  expectedAudiobooks,
  libraryCorpusDirectory,
  missingAudiobookCorpusReason,
  type ExpectedAudiobook,
} from "./support/libraryCorpus";
import {
  expectSessionDeleted,
  expectVideoPlays,
  filePosition,
  nextSessionResponse,
} from "./support/realPlayback";
import { waitForStartupScans } from "./support/scans";
import { buildTimeoutMs, startE2EServer, startupTimeoutMs, type E2EServer } from "./support/server";
import { openApp } from "./support/shell";

// Audiobooks in a real browser (task M3-14h): an audiobook library from the corpus lists its
// books with author and 2:3 cover, a book lists its chapters, and a chapter plays from its own
// part file at its offset. The suite owns its server, so the other specs' corpus stays
// untouched.

const audiobookLibraryName = "Books";
const scanTimeoutMs = 60_000;
const fixtureTimeoutMs = buildTimeoutMs + startupTimeoutMs + scanTimeoutMs;
const suiteTestTimeoutMs = 60_000;
const createdStatus = 201;

interface Fixtures {
  readonly baseURL: string;
}

interface WorkerFixtures {
  readonly audiobookServer: E2EServer;
}

const test = base.extend<Fixtures, WorkerFixtures>({
  audiobookServer: [
    // Playwright needs the destructuring pattern to see that the fixture has no dependencies.
    // eslint-disable-next-line no-empty-pattern
    async ({}, provide, workerInfo) => {
      const server = await startE2EServer({
        logFile: join(workerInfo.project.outputDir, "server-audiobooks.log"),
        libraries: [
          {
            name: audiobookLibraryName,
            kind: "audiobooks",
            copyFrom: join(libraryCorpusDirectory, expectedAudiobooks().path),
          },
        ],
      });
      try {
        await waitForStartupScans(server.baseUrl, scanTimeoutMs);
        await provide(server);
      } finally {
        await server.stop();
      }
    },
    { scope: "worker", timeout: fixtureTimeoutMs },
  ],
  baseURL: async ({ audiobookServer }, provide) => {
    await provide(audiobookServer.baseUrl);
  },
});

if (!audiobookCorpusExists()) {
  // A skipped suite must not let a CI run pass unnoticed.
  if (process.env["CI"] !== undefined) {
    throw new Error(missingAudiobookCorpusReason);
  }
  test.skip(true, missingAudiobookCorpusReason);
}
test.describe.configure({ timeout: suiteTestTimeoutMs });

// The single-file M4B with embedded chapters, and the book of several MP3 parts.
function bookWith(container: string, severalParts: boolean): ExpectedAudiobook {
  const book = expectedAudiobooks().books.find(
    (candidate) =>
      candidate.container === container &&
      candidate.parts.length > 1 === severalParts &&
      candidate.chapters.length > 1,
  );
  if (book === undefined) {
    throw new Error(`The audiobook corpus has no ${container} book with several chapters.`);
  }
  return book;
}

async function openAudiobookLibrary(page: Page): Promise<Locator> {
  const { sidebar } = await openApp(page);
  await sidebar.getByRole("link", { name: audiobookLibraryName, exact: true }).click();
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { level: 1, name: audiobookLibraryName })).toBeVisible();
  return main.getByRole("list", { name: "Audiobooks" });
}

function bookCard(grid: Locator, book: ExpectedAudiobook): Locator {
  return grid
    .getByRole("listitem")
    .filter({ has: grid.page().getByRole("link", { name: book.title, exact: true }) });
}

// Opens the book's page and returns its detail as the server sends it.
async function openBook(page: Page, book: ExpectedAudiobook): Promise<ItemDetail> {
  const grid = await openAudiobookLibrary(page);
  await bookCard(grid, book).getByRole("link", { name: book.title, exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: book.title })).toBeVisible();
  const itemId = new URL(page.url()).pathname.split("/").at(-1) ?? "";
  const response = await page.request.get(`/api/v1/items/${itemId}`);
  expect(response.ok()).toBe(true);
  return (await response.json()) as ItemDetail;
}

interface ChapterStart {
  readonly session: PlaybackSession;
  /** What the player asked for: the chapter's file and its offset in that file. */
  readonly request: PlaybackRequest;
}

async function playChapter(page: Page, title: string): Promise<ChapterStart> {
  const response = nextSessionResponse(page);
  await page.getByRole("link", { name: `Play ${title}`, exact: true }).click();
  const answer = await response;
  expect(answer.status(), await answer.text()).toBe(createdStatus);
  return {
    session: (await answer.json()) as PlaybackSession,
    request: answer.request().postDataJSON() as PlaybackRequest,
  };
}

function chapterNamed(detail: ItemDetail, title: string) {
  const chapter = detail.chapters?.find((candidate) => candidate.title === title);
  if (chapter === undefined) {
    throw new Error(`The book ${detail.title} has no chapter ${title}.`);
  }
  return chapter;
}

test("lists every book of the corpus with its author and a 2:3 cover", async ({ page }) => {
  const { books } = expectedAudiobooks();
  const grid = await openAudiobookLibrary(page);

  await expect(grid.getByRole("listitem")).toHaveCount(books.length);
  for (const book of books) {
    const card = bookCard(grid, book);
    await expect(card).toHaveCount(1);
    await expect(card.getByText(book.author, { exact: true })).toBeVisible();
  }
  const box = await grid.getByRole("listitem").first().locator(".aspect-2\\/3").boundingBox();
  expect(box?.width).toBeGreaterThan(0);
  expect(box?.height).toBeCloseTo((box?.width ?? 0) * (3 / 2), 0);
  expect(await blockingAxeViolations(page)).toEqual([]);
});

test("opens the M4B book, lists its three chapters and plays one from its offset", async ({
  page,
}) => {
  const book = bookWith("m4b", false);
  const detail = await openBook(page, book);

  if (book.narrator !== undefined) {
    await expect(page.getByText(`Narrated by ${book.narrator}`, { exact: false })).toBeVisible();
  }
  const chapters = page.getByRole("list", { name: "Chapters" });
  await expect(chapters.getByRole("listitem")).toHaveCount(book.chapters.length);
  expect(book.chapters).toHaveLength(3);
  for (const chapter of book.chapters) {
    await expect(chapters.getByRole("link", { name: `Play ${chapter.title}` })).toBeVisible();
  }
  expect(await blockingAxeViolations(page)).toEqual([]);

  const second = book.chapters[1];
  if (second === undefined) {
    throw new Error(`The book ${book.title} has no second chapter.`);
  }
  const { session, request } = await playChapter(page, second.title);
  expect(request.fileId).toBe(chapterNamed(detail, second.title).fileId);
  expect(request.startPositionSeconds).toBe(second.startSeconds);
  expect(session.fileId).toBe(request.fileId);
  await expectVideoPlays(page);
  // Direct play streams the whole file and seeks locally, a transcode starts at the offset:
  // either way the position shown is inside the chapter, not at the start of the book.
  expect(await filePosition(page)).toBeGreaterThanOrEqual(second.startSeconds);

  await page.goBack();
  await expect(page.getByRole("heading", { level: 1, name: book.title })).toBeVisible();
  await expectSessionDeleted(page, session.id);
});

test("plays a chapter of a later part from that part's file", async ({ page }) => {
  const book = bookWith("mp3", true);
  const detail = await openBook(page, book);
  const last = book.chapters.at(-1);
  if (last === undefined) {
    throw new Error(`The book ${book.title} has no chapters.`);
  }
  const chapter = chapterNamed(detail, last.title);
  // The chapter lies in another file than the first part, which Play would open.
  expect(chapter.fileId).not.toBe(detail.files[0]?.id);

  const { session, request } = await playChapter(page, last.title);
  expect(request.fileId).toBe(chapter.fileId);
  expect(session.fileId).toBe(chapter.fileId);
  await expectVideoPlays(page);

  await page.goBack();
  await expectSessionDeleted(page, session.id);
});
