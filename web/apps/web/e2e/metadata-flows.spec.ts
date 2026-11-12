import type { ItemPage, ItemSummary, LibraryList } from "@mivo/api-client";
import { productId } from "@mivo/ui-tokens";
import {
  expect,
  test as base,
  type APIRequestContext,
  type Locator,
  type Page,
} from "@playwright/test";
import { join } from "node:path";

import { apiContext, cachedAdminStorageState, type StorageState } from "./support/auth";
import {
  ambiguousPick,
  ambiguousTitle,
  fakeOverview,
  fakeTmdbUrlVariable,
  startFakeTmdb,
  type FakeTmdb,
} from "./support/fakeTmdb";
import {
  expectedLibrary,
  libraryCorpusExists,
  libraryCorpusFolder,
  missingLibraryCorpusReason,
} from "./support/libraryCorpus";
import { expectVideoPlays, playerVideo } from "./support/realPlayback";
import { waitForStartupScans } from "./support/scans";
import { buildTimeoutMs, startE2EServer, startupTimeoutMs, type E2EServer } from "./support/server";
import { openApp } from "./support/shell";

// Online metadata against a local fake TMDB (task M6-17): matching, the identify flow, removing
// a match, the offline behaviour and the global off switch. The server gets its own library
// and its own fake, so nothing here reaches the internet. The flows share one server and run in
// order, because each builds on the state the one before left.

const moviesLibraryName = "Movies";
const matchedTitle = "Blade Runner";
const removedTitle = "Amélie";
const envPrefix = `${productId.toUpperCase()}_`;
const scanTimeoutMs = 120_000;
const fixtureTimeoutMs = buildTimeoutMs + startupTimeoutMs + scanTimeoutMs;
const flowTimeoutMs = 60_000;
const matchWaitMs = 30_000;
// Long enough for the worker (polls every 100 ms) to have contacted the fake if it wanted to.
const reloadCheckMs = 2_000;
const quietPeriodMs = 1_500;
const acceptedStatus = 202;

interface Fixtures {
  readonly baseURL: string;
  readonly storageState: StorageState;
}

interface WorkerFixtures {
  readonly fakeTmdb: FakeTmdb;
  readonly metadataServer: E2EServer;
}

const test = base.extend<Fixtures, WorkerFixtures>({
  fakeTmdb: [
    // Playwright needs the destructuring pattern to see that the fixture has no dependencies.
    // eslint-disable-next-line no-empty-pattern
    async ({}, provide) => {
      const fake = await startFakeTmdb();
      try {
        await provide(fake);
      } finally {
        await fake.stop();
      }
    },
    { scope: "worker" },
  ],
  metadataServer: [
    async ({ fakeTmdb }, provide, workerInfo) => {
      const server = await startE2EServer({
        logFile: join(workerInfo.project.outputDir, "server-metadata-flows.log"),
        libraries: [
          {
            name: moviesLibraryName,
            kind: "movies",
            copyFrom: libraryCorpusFolder(expectedLibrary("movies")),
          },
        ],
        environment: {
          [`${envPrefix}METADATA_ONLINE_ENABLED`]: "true",
          [`${envPrefix}METADATA_TMDB_API_KEY`]: "e2e-fake-key",
          [`${envPrefix}METADATA_MUSICBRAINZ_ENABLED`]: "false",
          [fakeTmdbUrlVariable]: fakeTmdb.origin,
        },
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
  baseURL: async ({ metadataServer }, provide) => {
    await provide(metadataServer.baseUrl);
  },
  storageState: async ({ metadataServer }, provide) => {
    await provide(await cachedAdminStorageState(metadataServer.baseUrl));
  },
});

if (!libraryCorpusExists()) {
  // A skipped suite must not let a CI run pass unnoticed.
  if (process.env["CI"] !== undefined) {
    throw new Error(missingLibraryCorpusReason);
  }
  test.skip(true, missingLibraryCorpusReason);
}
test.describe.configure({ mode: "serial", timeout: flowTimeoutMs });

/** Reads the server's API as the signed-in administrator. */
async function withApi<Result>(
  server: E2EServer,
  work: (api: APIRequestContext) => Promise<Result>,
): Promise<Result> {
  const api = await apiContext(server.baseUrl, await cachedAdminStorageState(server.baseUrl));
  try {
    return await work(api);
  } finally {
    await api.dispose();
  }
}

async function moviesLibraryId(api: APIRequestContext): Promise<string> {
  const list = (await (await api.get("/api/v1/libraries")).json()) as LibraryList;
  const library = list.libraries.find((candidate) => candidate.name === moviesLibraryName);
  if (library === undefined) {
    throw new Error(`The server has no library ${moviesLibraryName}.`);
  }
  return library.id;
}

/** The movie titled `title` as the library list shows it, or undefined. */
async function findMovie(server: E2EServer, title: string): Promise<ItemSummary | undefined> {
  return withApi(server, async (api) => {
    const id = await moviesLibraryId(api);
    const response = await api.get(`/api/v1/libraries/${id}/items?limit=500`);
    const page = (await response.json()) as ItemPage;
    return page.items.find((item) => item.title === title);
  });
}

/** Polls until the movie titled `title` has the given match state. */
async function expectMatchState(
  server: E2EServer,
  title: string,
  state: "matched" | "unmatched" | "disabled",
): Promise<void> {
  await expect
    .poll(async () => (await findMovie(server, title))?.matchState, { timeout: matchWaitMs })
    .toBe(state);
}

async function queueLength(server: E2EServer): Promise<number> {
  return withApi(server, async (api) => {
    const status = (await (await api.get("/api/v1/metadata/status")).json()) as {
      queueLength: number;
    };
    return status.queueLength;
  });
}

async function openMovies(page: Page): Promise<Locator> {
  const { sidebar } = await openApp(page);
  await sidebar.getByRole("link", { name: moviesLibraryName, exact: true }).click();
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { level: 1, name: moviesLibraryName })).toBeVisible();
  return main.getByRole("list", { name: "Videos" });
}

async function openMovie(page: Page, title: string): Promise<void> {
  const grid = await openMovies(page);
  await grid.getByRole("link", { name: title, exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
}

async function chooseFromItemMenu(page: Page, entry: string): Promise<void> {
  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: entry }).click();
}

function providerState(page: Page, provider: string): Locator {
  return page
    .getByRole("list", { name: "Provider states" })
    .getByRole("listitem")
    .filter({ hasText: provider });
}

async function searchIdentifyDialog(page: Page, query: string): Promise<Locator> {
  await chooseFromItemMenu(page, "Correct match…");
  const dialog = page.getByRole("dialog", { name: "Correct match" });
  await dialog.getByLabel("Title, or a TMDB or MusicBrainz link or ID").fill(query);
  await dialog.getByRole("button", { name: "Search" }).click();
  return dialog;
}

test("flow 1: the scan matches the movies at the fake provider and the page credits TMDB", async ({
  page,
  metadataServer,
  fakeTmdb,
}) => {
  await expectMatchState(metadataServer, matchedTitle, "matched");
  expect(fakeTmdb.requests().some((request) => request.query === matchedTitle)).toBe(true);
  // Without a local poster, the grid card shows the provider poster the server downloaded.
  await expect
    .poll(async () => (await findMovie(metadataServer, matchedTitle))?.poster, {
      timeout: matchWaitMs,
    })
    .toBeDefined();

  const grid = await openMovies(page);
  const card = grid
    .getByRole("listitem")
    .filter({ has: page.getByRole("link", { name: matchedTitle, exact: true }) });
  const poster = card.locator("img[src^='/api/v1/images/']");
  // The grid loads posters lazily, so the card has to be in view.
  await poster.scrollIntoViewIfNeeded();
  await expect(poster).toBeVisible();
  await expect
    .poll(() => poster.evaluate((image: HTMLImageElement) => image.naturalWidth), {
      timeout: matchWaitMs,
    })
    .toBeGreaterThan(0);

  await card.getByRole("link", { name: matchedTitle, exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: matchedTitle })).toBeVisible();
  await expect(page.getByText(fakeOverview)).toBeVisible();
  await expect(page.getByRole("img", { name: "TMDB" })).toBeVisible();
  await expect(
    page.getByText("This product uses the TMDB API but is not endorsed or certified by TMDB."),
  ).toBeVisible();
});

test("flow 2: an ambiguous title stays unmatched, the filter finds it, Correct match fixes it", async ({
  page,
  metadataServer,
}) => {
  await expect.poll(() => queueLength(metadataServer), { timeout: matchWaitMs }).toBe(0);
  await expectMatchState(metadataServer, ambiguousTitle, "unmatched");

  const grid = await openMovies(page);
  await page
    .getByRole("group", { name: "Browse options" })
    .getByRole("button", { name: "Filter" })
    .click();
  await page.getByRole("menuitemradio", { name: "Unmatched" }).click();
  await page.keyboard.press("Escape");
  await expect(grid.getByRole("link", { name: ambiguousTitle, exact: true })).toBeVisible();
  // The filter hides what is matched.
  await expect(grid.getByRole("link", { name: matchedTitle, exact: true })).toHaveCount(0);

  await grid.getByRole("link", { name: ambiguousTitle, exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: ambiguousTitle })).toBeVisible();
  await chooseFromItemMenu(page, "Correct match…");
  const dialog = page.getByRole("dialog", { name: "Correct match" });
  const results = dialog.getByRole("list", { name: "Results" });
  await expect(results.getByRole("listitem")).toHaveCount(2);
  await results.getByRole("button", { name: new RegExp(`^Apply ${ambiguousPick.title}`) }).click();
  await expect(dialog).toBeHidden();

  // The title is not locked, so the item takes the provider's.
  await expectMatchState(metadataServer, ambiguousPick.title, "matched");
  // The match is stored at once, the details follow from the queue: read the page again.
  await expect(async () => {
    await page.reload();
    await expect(page.getByText(fakeOverview)).toBeVisible({ timeout: reloadCheckMs });
  }).toPass({ timeout: matchWaitMs });
});

test("flow 3: removing a match drops the provider poster, keeps the rest and survives a rescan", async ({
  page,
  metadataServer,
  fakeTmdb,
}) => {
  await expectMatchState(metadataServer, removedTitle, "matched");
  await expect
    .poll(async () => (await findMovie(metadataServer, removedTitle))?.poster, {
      timeout: matchWaitMs,
    })
    .toBeDefined();

  await openMovie(page, removedTitle);
  await chooseFromItemMenu(page, "Remove match");
  await page
    .getByRole("dialog", { name: "Remove match?" })
    .getByRole("button", { name: "Remove match" })
    .click();

  await expectMatchState(metadataServer, removedTitle, "disabled");
  const afterRemoval = await findMovie(metadataServer, removedTitle);
  expect(afterRemoval?.poster).toBeUndefined();
  expect(afterRemoval?.title).toBe(removedTitle);

  fakeTmdb.clearRequests();
  await withApi(metadataServer, async (api) => {
    const response = await api.post(`/api/v1/libraries/${await moviesLibraryId(api)}/scan`);
    expect(response.status()).toBe(acceptedStatus);
  });
  await waitForStartupScans(metadataServer.baseUrl, scanTimeoutMs);
  await expect.poll(() => queueLength(metadataServer)).toBe(0);
  await page.waitForTimeout(quietPeriodMs);

  expect((await findMovie(metadataServer, removedTitle))?.matchState).toBe("disabled");
  expect(fakeTmdb.requests().filter((request) => request.query === removedTitle)).toEqual([]);
});

test("flow 4: without the provider the library and playback work, the status says offline and recovers", async ({
  page,
  fakeTmdb,
}) => {
  await fakeTmdb.stop();

  await openMovie(page, matchedTitle);
  const dialog = await searchIdentifyDialog(page, "A title the fake has never been asked for");
  await expect(dialog.getByRole("alert")).toHaveText("Provider offline, try again later.");
  await dialog.getByRole("button", { name: "Cancel" }).click();

  // The library and the player do not need the provider.
  const grid = await openMovies(page);
  await expect(grid.getByRole("listitem").first()).toBeVisible();
  await grid.getByRole("link", { name: matchedTitle, exact: true }).click();
  await page.getByRole("link", { name: "Play" }).click();
  await expect(playerVideo(page)).toBeVisible();
  await expectVideoPlays(page);

  await page.goto("/admin/settings/metadata");
  await expect(providerState(page, "TMDB")).toContainText("Offline");

  await fakeTmdb.start();
  await page.goto("/admin/settings/metadata");
  await openMovie(page, matchedTitle);
  const retried = await searchIdentifyDialog(page, "A title the fake has never been asked for");
  await expect(retried.getByRole("list", { name: "Results" })).toBeVisible();
  await retried.getByRole("button", { name: "Cancel" }).click();

  await page.goto("/admin/settings/metadata");
  await expect(providerState(page, "TMDB")).toContainText("OK");
});

test("flow 5: with online metadata switched off the fake receives no request", async ({
  page,
  metadataServer,
  fakeTmdb,
}) => {
  await page.goto("/admin/settings/metadata");
  const online = page.getByRole("switch", { name: "Look up metadata online" });
  await expect(online).toBeChecked();
  // A controlled switch changes after the server answered, which `uncheck` does not wait for.
  await online.click();
  await expect(online).not.toBeChecked();
  await expect.poll(() => onlineEnabled(metadataServer)).toBe(false);

  fakeTmdb.clearRequests();
  await withApi(metadataServer, async (api) => {
    const response = await api.post(`/api/v1/libraries/${await moviesLibraryId(api)}/scan`);
    expect(response.status()).toBe(acceptedStatus);
  });
  await waitForStartupScans(metadataServer.baseUrl, scanTimeoutMs);

  await openMovie(page, matchedTitle);
  await chooseFromItemMenu(page, "Correct match…");
  await expect(page.getByRole("dialog", { name: "Correct match" }).getByRole("alert")).toHaveText(
    "Online metadata is switched off – Settings › Metadata.",
  );
  await page.waitForTimeout(quietPeriodMs);

  expect(fakeTmdb.requests()).toEqual([]);
});

async function onlineEnabled(server: E2EServer): Promise<boolean> {
  return withApi(server, async (api) => {
    const settings = (await (await api.get("/api/v1/settings/metadata")).json()) as {
      onlineEnabled: boolean;
    };
    return settings.onlineEnabled;
  });
}
