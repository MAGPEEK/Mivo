import type { PlaybackSession } from "@mivo/api-client";
import { expect, test as base, type Locator, type Page } from "@playwright/test";
import { join } from "node:path";

import { blockingAxeViolations } from "./support/axe";
import {
  expectedMusic,
  libraryCorpusDirectory,
  missingMusicCorpusReason,
  musicCorpusExists,
  type ExpectedAlbum,
} from "./support/libraryCorpus";
import {
  expectSessionDeleted,
  expectVideoPlays,
  nextSessionResponse,
} from "./support/realPlayback";
import { waitForStartupScans } from "./support/scans";
import { buildTimeoutMs, startE2EServer, startupTimeoutMs, type E2EServer } from "./support/server";
import { openApp } from "./support/shell";

// Music in a real browser (task M3-13h): a music library from the corpus lists its albums
// with square covers, an album lists its tracks by disc, and a track plays in the player.
// The suite owns its server, so the media corpus of the other specs stays untouched.

const musicLibraryName = "Music";
const scanTimeoutMs = 60_000;
const fixtureTimeoutMs = buildTimeoutMs + startupTimeoutMs + scanTimeoutMs;
const suiteTestTimeoutMs = 60_000;
const createdStatus = 201;

interface Fixtures {
  readonly baseURL: string;
}

interface WorkerFixtures {
  readonly musicServer: E2EServer;
}

const test = base.extend<Fixtures, WorkerFixtures>({
  musicServer: [
    // Playwright needs the destructuring pattern to see that the fixture has no dependencies.
    // eslint-disable-next-line no-empty-pattern
    async ({}, provide, workerInfo) => {
      const server = await startE2EServer({
        logFile: join(workerInfo.project.outputDir, "server-music.log"),
        libraries: [
          {
            name: musicLibraryName,
            kind: "music",
            copyFrom: join(libraryCorpusDirectory, expectedMusic().path),
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
  baseURL: async ({ musicServer }, provide) => {
    await provide(musicServer.baseUrl);
  },
});

if (!musicCorpusExists()) {
  // A skipped suite must not let a CI run pass unnoticed.
  if (process.env["CI"] !== undefined) {
    throw new Error(missingMusicCorpusReason);
  }
  test.skip(true, missingMusicCorpusReason);
}
test.describe.configure({ timeout: suiteTestTimeoutMs });

function albumOf(container: string, multiDisc = false): ExpectedAlbum {
  const album = expectedMusic().albums.find(
    (candidate) =>
      candidate.container === container &&
      new Set(candidate.tracks.map((track) => track.disc)).size > 1 === multiDisc,
  );
  if (album === undefined) {
    throw new Error(
      `The music corpus has no ${container} album (several discs: ${String(multiDisc)}).`,
    );
  }
  return album;
}

async function openMusicLibrary(page: Page): Promise<Locator> {
  const { sidebar } = await openApp(page);
  await sidebar.getByRole("link", { name: musicLibraryName, exact: true }).click();
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { level: 1, name: musicLibraryName })).toBeVisible();
  // A music library lists its artists first; the toolbar's type menu switches to the albums.
  await main.getByRole("button", { name: "Show: Artists" }).click();
  await page.getByRole("menuitemradio", { name: "Albums" }).click();
  return main.getByRole("list", { name: "Albums" });
}

function albumCard(grid: Locator, album: ExpectedAlbum): Locator {
  return grid
    .getByRole("listitem")
    .filter({ has: grid.page().getByRole("link", { name: album.title, exact: true }) });
}

async function openAlbum(page: Page, album: ExpectedAlbum): Promise<void> {
  const grid = await openMusicLibrary(page);
  await albumCard(grid, album).getByRole("link", { name: album.title, exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: album.title })).toBeVisible();
}

async function playTrack(page: Page, title: string): Promise<PlaybackSession> {
  const response = nextSessionResponse(page);
  await page.getByRole("link", { name: `Play ${title}`, exact: true }).click();
  const answer = await response;
  expect(answer.status(), await answer.text()).toBe(createdStatus);
  return (await answer.json()) as PlaybackSession;
}

test("lists every album of the corpus with its artist and a square cover", async ({ page }) => {
  const { albums } = expectedMusic();
  const grid = await openMusicLibrary(page);

  await expect(grid.getByRole("listitem")).toHaveCount(albums.length);
  for (const album of albums) {
    const card = albumCard(grid, album);
    await expect(card).toHaveCount(1);
    await expect(card.getByText(album.albumArtist, { exact: true })).toBeVisible();
  }
  const box = await grid.getByRole("listitem").first().locator(".aspect-square").boundingBox();
  expect(box?.width).toBeGreaterThan(0);
  expect(box?.width).toBe(box?.height);
  expect(await blockingAxeViolations(page)).toEqual([]);
});

test("opens a two-disc album, lists its tracks by disc and plays the first track", async ({
  page,
}) => {
  const album = albumOf("flac", true);
  await openAlbum(page, album);

  const discs = [...new Set(album.tracks.map((track) => track.disc))];
  for (const disc of discs) {
    const titles = album.tracks.filter((track) => track.disc === disc).map((track) => track.title);
    const list = page.getByRole("list", { name: `Disc ${String(disc)}` });
    await expect(list.getByRole("listitem")).toHaveCount(titles.length);
    for (const title of titles) {
      await expect(list.getByRole("link", { name: `Play ${title}`, exact: true })).toBeVisible();
    }
  }
  expect(await blockingAxeViolations(page)).toEqual([]);

  const [first] = album.tracks;
  if (first === undefined) {
    throw new Error(`The album ${album.title} has no tracks.`);
  }
  const session = await playTrack(page, first.title);
  await expectVideoPlays(page);

  await page.goBack();
  await expect(page.getByRole("heading", { level: 1, name: album.title })).toBeVisible();
  await expectSessionDeleted(page, session.id);
});

for (const container of ["mp3", "m4a"]) {
  test(`plays the first track of the ${container} album`, async ({ page }) => {
    const album = albumOf(container);
    await openAlbum(page, album);
    const [first] = album.tracks;
    if (first === undefined) {
      throw new Error(`The album ${album.title} has no tracks.`);
    }

    const session = await playTrack(page, first.title);
    await expectVideoPlays(page);

    await page.goBack();
    await expectSessionDeleted(page, session.id);
  });
}
