import type { PlaybackSession } from "@mivo/api-client";
import { expect, test as base, type Locator, type Page, type Response } from "@playwright/test";
import { join } from "node:path";

import { blockingAxeViolations } from "./support/axe";
import {
  expectedLibrary,
  libraryCorpusExists,
  libraryCorpusFolder,
  missingLibraryCorpusReason,
  type ExpectedItem,
} from "./support/libraryCorpus";
import {
  expectMode,
  expectVideoPlays,
  modes,
  nextSessionResponse,
  openStreamInfo,
  playerVideo,
  type Mode,
} from "./support/realPlayback";
import { waitForStartupScans } from "./support/scans";
import { buildTimeoutMs, startE2EServer, startupTimeoutMs, type E2EServer } from "./support/server";
import { openApp } from "./support/shell";

// Library model v2 in a real browser (task M3-18): typed movies and shows libraries are
// scanned, posters come from the image endpoint and a show opens down to its episodes.
// The suite owns its server, so the media corpus of the other specs stays untouched.

const moviesLibraryName = "Movies";
const showsLibraryName = "Shows";
// 140 small files with two probes at a time.
const scanTimeoutMs = 120_000;
const fixtureTimeoutMs = buildTimeoutMs + startupTimeoutMs + scanTimeoutMs;
const suiteTestTimeoutMs = 60_000;
const createdStatus = 201;
const notModifiedStatus = 304;
const okStatus = 200;
const imagePathPrefix = "/api/v1/images/";
const imageUrlPattern = /\/api\/v1\/images\/[^?]+\?width=\d+&tag=.+/;
const showWithSpecials = "Pioneer One";
const absoluteNumberedShow = "Dragon Ball Super";
const absoluteNumberedEpisodeCount = 131;

interface Fixtures {
  readonly baseURL: string;
}

interface WorkerFixtures {
  readonly libraryServer: E2EServer;
}

const test = base.extend<Fixtures, WorkerFixtures>({
  libraryServer: [
    // Playwright needs the destructuring pattern to see that the fixture has no dependencies.
    // eslint-disable-next-line no-empty-pattern
    async ({}, provide, workerInfo) => {
      const server = await startE2EServer({
        logFile: join(workerInfo.project.outputDir, "server-library-v2.log"),
        libraries: [
          {
            name: moviesLibraryName,
            kind: "movies",
            copyFrom: libraryCorpusFolder(expectedLibrary("movies")),
          },
          {
            name: showsLibraryName,
            kind: "shows",
            copyFrom: libraryCorpusFolder(expectedLibrary("shows")),
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
  baseURL: async ({ libraryServer }, provide) => {
    await provide(libraryServer.baseUrl);
  },
});

if (!libraryCorpusExists()) {
  // A skipped suite must not let a CI run pass unnoticed.
  if (process.env["CI"] !== undefined) {
    throw new Error(missingLibraryCorpusReason);
  }
  test.skip(true, missingLibraryCorpusReason);
}
test.describe.configure({ timeout: suiteTestTimeoutMs });

async function openLibraryByName(page: Page, name: string): Promise<Locator> {
  const { sidebar } = await openApp(page);
  await sidebar.getByRole("link", { name, exact: true }).click();
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { level: 1, name })).toBeVisible();
  return main.getByRole("list", { name: "Videos" });
}

function card(grid: Locator, title: string): Locator {
  return grid
    .getByRole("listitem")
    .filter({ has: grid.page().getByRole("link", { name: title, exact: true }) });
}

function plural(count: number, one: string, other: string): string {
  return `${String(count)} ${count === 1 ? one : other}`;
}

function seasonCaption(number: number, episodeCount: number): string {
  const name = number === 0 ? "Specials" : `Season ${String(number)}`;
  return `${name} · ${plural(episodeCount, "episode", "episodes")}`;
}

/** The episode numbers of season 1 of the show with specials, as the manifest lists them. */
function firstSeasonEpisodeNumbers(): number[] {
  const season = (findShow().seasons ?? []).find((candidate) => candidate.number === 1);
  if (season === undefined) {
    throw new Error(`The manifest has no season 1 for ${showWithSpecials}.`);
  }
  return season.episodes.map((episode) => episode.number);
}

function findShow(): ExpectedItem {
  const show = expectedLibrary("shows").items.find((item) => item.title === showWithSpecials);
  if (show?.seasons === undefined) {
    throw new Error(`The manifest has no show ${showWithSpecials} with seasons.`);
  }
  return show;
}

async function openShow(page: Page, title: string): Promise<void> {
  const grid = await openLibraryByName(page, showsLibraryName);
  await card(grid, title).getByRole("link", { name: title, exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
}

function seasonLinks(page: Page): Locator {
  return page.getByRole("region", { name: "Seasons" }).getByRole("link");
}

function episodeRows(page: Page): Locator {
  return page.getByRole("region", { name: "Episodes" }).getByRole("listitem");
}

async function openSeason(page: Page, caption: string): Promise<void> {
  await seasonLinks(page).filter({ hasText: caption }).click();
  await expect(episodeRows(page).first()).toBeVisible();
}

function modeOf(session: PlaybackSession): Mode {
  const byMode = {
    direct_play: modes.directPlay,
    remux: modes.remux,
    transcode: modes.transcode,
  } as const;
  return byMode[session.mode];
}

function trackImageResponses(page: Page): Response[] {
  const responses: Response[] = [];
  page.on("response", (response) => {
    if (new URL(response.url()).pathname.startsWith(imagePathPrefix)) {
      responses.push(response);
    }
  });
  return responses;
}

function loadedImageCount(grid: Locator): Promise<number> {
  return grid
    .locator("img")
    .evaluateAll(
      (images) => images.filter((image) => (image as HTMLImageElement).naturalWidth > 0).length,
    );
}

test("lists every movie once with its NFO title and loads posters from the image endpoint", async ({
  page,
}) => {
  const movies = expectedLibrary("movies").items;
  const postered = movies.filter((movie) => (movie.artwork ?? []).length > 0).length;
  const images = trackImageResponses(page);
  const grid = await openLibraryByName(page, moviesLibraryName);

  await expect(grid.getByRole("listitem")).toHaveCount(movies.length);
  for (const movie of movies) {
    await expect(card(grid, movie.title)).toHaveCount(1);
  }
  await expect.poll(() => loadedImageCount(grid)).toBe(postered);
  for (const image of await grid.locator("img").all()) {
    await expect(image).toHaveAttribute("src", imageUrlPattern);
  }
  expect(images.length).toBeGreaterThan(0);
  for (const image of images) {
    expect(image.status()).toBe(okStatus);
    expect(await image.headerValue("content-type")).toBe("image/jpeg");
  }
});

test("shows the placeholder for a movie without artwork", async ({ page }) => {
  const bare = expectedLibrary("movies").items.find((movie) => (movie.artwork ?? []).length === 0);
  if (bare === undefined) {
    throw new Error("The manifest has no movie without artwork.");
  }
  const grid = await openLibraryByName(page, moviesLibraryName);

  const bareCard = card(grid, bare.title);
  await expect(bareCard.locator("[data-poster-placeholder]")).toBeVisible();
  await expect(bareCard.locator("img")).toHaveCount(0);
});

function cardTitles(grid: Locator): Promise<string[]> {
  return grid.getByRole("listitem").getByRole("link").allInnerTexts();
}

test("sorts the movies from the toolbar and keeps the sort in the URL (M5-31)", async ({
  page,
}) => {
  const movieCount = expectedLibrary("movies").items.length;
  const grid = await openLibraryByName(page, moviesLibraryName);
  const toolbar = page.getByRole("group", { name: "Browse options" });
  await expect(toolbar.getByText(plural(movieCount, "movie", "movies"))).toBeVisible();
  await expect(grid.getByRole("listitem")).toHaveCount(movieCount);
  const ascending = await cardTitles(grid);

  await toolbar.getByRole("button", { name: "Sort direction: Ascending" }).click();

  await expect(page).toHaveURL(/[?&]order=desc(&|$)/);
  await expect.poll(() => cardTitles(grid)).toEqual([...ascending].reverse());
  await page.reload();
  await expect(grid.getByRole("listitem")).toHaveCount(movieCount);
  expect(await cardTitles(grid)).toEqual([...ascending].reverse());

  await toolbar.getByRole("button", { name: "Sort: Title" }).click();
  await page.getByRole("menuitemradio", { name: "Date added" }).click();
  await expect(page).toHaveURL(/[?&]sort=dateAdded(&|$)/);
  await expect(toolbar.getByRole("button", { name: "Sort: Date added" })).toBeVisible();
  await expect(grid.getByRole("listitem")).toHaveCount(movieCount);
});

test("filters the movies by a counted field value and clears it from the chips (M5-32)", async ({
  page,
}) => {
  const movieCount = expectedLibrary("movies").items.length;
  const grid = await openLibraryByName(page, moviesLibraryName);
  const toolbar = page.getByRole("group", { name: "Browse options" });
  await expect(grid.getByRole("listitem")).toHaveCount(movieCount);

  await toolbar.getByRole("button", { name: "Filter" }).click();
  await page.getByRole("menuitem", { name: "Container" }).click();
  const values = page.getByRole("menu", { name: "Container" }).getByRole("menuitemcheckbox");
  const first = values.first();
  await expect(first).toBeVisible();
  // The accessible name carries the count: "MKV, 12 items".
  const name = (await first.getAttribute("aria-label")) ?? "";
  const match = /^(.+), (\d+) items?$/.exec(name);
  expect(match).not.toBeNull();
  const [, label = "", count = "0"] = match ?? [];
  await first.click();
  await expect(first).toHaveAttribute("aria-checked", "true");
  await page.keyboard.press("Escape");

  await expect(page).toHaveURL(/[?&]container=/);
  const chips = page.getByRole("group", { name: "Active filters" });
  await expect(
    chips.getByRole("button", { name: `Remove filter Container: ${label}` }),
  ).toBeVisible();
  await expect(grid.getByRole("listitem")).toHaveCount(Number(count));

  await chips.getByRole("button", { name: "Clear all" }).click();
  await expect(chips).toHaveCount(0);
  await expect(grid.getByRole("listitem")).toHaveCount(movieCount);
});

test("answers a repeated image request from the cache or with 304", async ({ page }) => {
  const postered = expectedLibrary("movies").items.filter(
    (movie) => (movie.artwork ?? []).length > 0,
  ).length;
  const grid = await openLibraryByName(page, moviesLibraryName);
  // Every poster must be in the cache before the reload, or a late one is fetched again.
  await expect.poll(() => loadedImageCount(grid)).toBe(postered);

  await page.reload();
  await expect(grid.getByRole("listitem").first()).toBeVisible();
  await expect.poll(() => loadedImageCount(grid)).toBe(postered);

  // The network log of the page reports cache hits with status 200, too; the resource timing
  // tells them apart: a cache hit transfers nothing, a revalidation answers 304.
  const transfers = await page.evaluate(() =>
    performance
      .getEntriesByType("resource")
      .filter((entry) => entry.name.includes("/api/v1/images/"))
      .map((entry) => {
        const timing = entry as PerformanceResourceTiming;
        return {
          url: entry.name,
          transferSize: timing.transferSize,
          status: timing.responseStatus,
        };
      }),
  );
  expect(transfers.length).toBeGreaterThan(0);
  for (const transfer of transfers) {
    expect(transfer.transferSize === 0 || transfer.status === notModifiedStatus, transfer.url).toBe(
      true,
    );
  }
});

test("the show page lists its seasons in order with the specials last", async ({ page }) => {
  const show = findShow();
  const expected = (show.seasons ?? [])
    .filter((season) => season.number > 0)
    .map((season) => seasonCaption(season.number, season.episodes.length));
  const specials = (show.seasons ?? []).find((season) => season.number === 0);
  if (specials !== undefined) {
    expected.push(seasonCaption(0, specials.episodes.length));
  }

  await openShow(page, showWithSpecials);

  await expect(seasonLinks(page)).toHaveText(expected);
  expect(await blockingAxeViolations(page)).toEqual([]);
});

test("a season lists its episodes ascending and reverses them with the order link", async ({
  page,
}) => {
  const numbers = firstSeasonEpisodeNumbers();
  const labels = (order: readonly number[]) => order.map((number) => `Episode ${String(number)}`);
  await openShow(page, showWithSpecials);
  await openSeason(page, seasonCaption(1, numbers.length));

  await expect(episodeRows(page)).toHaveCount(numbers.length);
  for (const [position, label] of labels(numbers).entries()) {
    await expect(episodeRows(page).nth(position)).toContainText(label);
  }
  expect(await blockingAxeViolations(page)).toEqual([]);

  await page.getByRole("link", { name: "Show last first" }).click();
  await expect(page).toHaveURL(/order=desc/);
  await page.reload();
  for (const [position, label] of labels(numbers.toReversed()).entries()) {
    await expect(episodeRows(page).nth(position)).toContainText(label);
  }
});

test("the episode page links to its neighbours and plays", async ({ page }) => {
  await openShow(page, showWithSpecials);
  await openSeason(page, seasonCaption(1, firstSeasonEpisodeNumbers().length));
  await episodeRows(page).first().getByRole("link").click();
  const neighbours = page.getByRole("navigation", { name: "Neighbouring items" });
  await expect(neighbours.getByRole("link", { name: /^Previous episode/ })).toHaveCount(0);
  expect(await blockingAxeViolations(page)).toEqual([]);

  await neighbours.getByRole("link", { name: /^Next episode/ }).click();
  await expect(neighbours.getByRole("link", { name: /^Previous episode/ })).toBeVisible();
  await expect(neighbours.getByRole("link", { name: /^Next episode/ })).toHaveCount(0);
  await neighbours.getByRole("link", { name: /^Previous episode/ }).click();
  await expect(neighbours.getByRole("link", { name: /^Next episode/ })).toBeVisible();

  const sessionResponse = nextSessionResponse(page);
  await page.getByRole("link", { name: "Play" }).click();
  const response = await sessionResponse;
  expect(response.status(), await response.text()).toBe(createdStatus);
  const session = (await response.json()) as PlaybackSession;
  await expect(playerVideo(page)).toBeVisible();
  await expectVideoPlays(page);
  await expectMode(await openStreamInfo(page), modeOf(session));
});

test("the absolute-numbered show has one season with all its episodes", async ({ page }) => {
  await openShow(page, absoluteNumberedShow);
  await expect(seasonLinks(page)).toHaveCount(1);
  const caption = seasonCaption(1, absoluteNumberedEpisodeCount);
  await expect(seasonLinks(page)).toHaveText(caption);

  await openSeason(page, caption);
  const loadMore = page.getByRole("button", { name: "Load more" });
  while (await loadMore.isVisible()) {
    const before = await episodeRows(page).count();
    await loadMore.click();
    await expect(episodeRows(page)).not.toHaveCount(before);
  }
  await expect(episodeRows(page)).toHaveCount(absoluteNumberedEpisodeCount);
});

test("Home shows a recently added row per library with See all into Browse", async ({ page }) => {
  await openApp(page);
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { level: 2 })).toHaveText([
    `Recently added in ${moviesLibraryName}`,
    `Recently added in ${showsLibraryName}`,
  ]);

  const shows = main.getByRole("region", { name: `Recently added in ${showsLibraryName}` });
  const showTitles = expectedLibrary("shows").items.map((item) => item.title);
  const firstCard = shows.getByRole("listitem").first().getByRole("link");
  expect(showTitles).toContain(await firstCard.textContent());

  const seeAll = shows.getByRole("link", {
    name: `See all: Recently added in ${showsLibraryName}`,
  });
  const href = new URL((await seeAll.getAttribute("href")) ?? "", page.url());
  expect(href.pathname).toMatch(/^\/libraries\/[^/]+$/);
  expect(href.searchParams.get("view")).toBe("browse");
  expect(href.searchParams.get("sort")).toBe("dateAdded");
  expect(await blockingAxeViolations(page)).toEqual([]);
});

test("a library opens on Browse, shows its server rows on Overview and remembers the pivot (M5-30)", async ({
  page,
}) => {
  const grid = await openLibraryByName(page, moviesLibraryName);
  const main = page.getByRole("main");
  await expect(main.getByRole("tab", { name: "Browse", selected: true })).toBeVisible();
  await expect(grid.getByRole("listitem").first()).toBeVisible();
  const libraryPath = new URL(page.url()).pathname;

  await main.getByRole("tab", { name: "Overview" }).click();

  await expect(page).toHaveURL(/[?&]view=overview(&|$)/);
  const recent = main.getByRole("region", { name: "Recently added" });
  await expect(recent).toBeVisible();
  const movieTitles = expectedLibrary("movies").items.map((item) => item.title);
  const firstCard = recent.getByRole("listitem").first().getByRole("link");
  expect(movieTitles).toContain(await firstCard.textContent());
  await expect(grid).toHaveCount(0);
  expect(await blockingAxeViolations(page)).toEqual([]);

  // A plain library link opens the pivot used last on this device.
  await page.goto(libraryPath);
  await expect(main.getByRole("tab", { name: "Overview", selected: true })).toBeVisible();
  await expect(recent).toBeVisible();

  await recent.getByRole("link", { name: "See all: Recently added" }).click();

  await expect(page).toHaveURL(/[?&]view=browse(&|$)/);
  await expect(page).toHaveURL(/[?&]sort=dateAdded(&|$)/);
  await expect(main.getByRole("tab", { name: "Browse", selected: true })).toBeVisible();
  await expect(grid.getByRole("listitem")).toHaveCount(movieTitles.length);
});
