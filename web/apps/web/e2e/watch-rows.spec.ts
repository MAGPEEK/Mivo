import type { HomeRows, ItemPage, ItemSummary, LibraryList, Row } from "@mivo/api-client";
import { expect, test as base, type APIRequestContext } from "@playwright/test";
import { join } from "node:path";

import { apiContext, cachedAdminStorageState, type StorageState } from "./support/auth";
import {
  expectedLibrary,
  libraryCorpusExists,
  libraryCorpusFolder,
  missingLibraryCorpusReason,
} from "./support/libraryCorpus";
import { waitForStartupScans } from "./support/scans";
import { buildTimeoutMs, startE2EServer, startupTimeoutMs, type E2EServer } from "./support/server";
import { openApp } from "./support/shell";

// Continue watching on Home and the unmark confirmation of a show card (task M7-37). The suite
// owns its server, so the watch state it writes stays out of the other specs.

const scanTimeoutMs = 120_000;
const fixtureTimeoutMs = buildTimeoutMs + startupTimeoutMs + scanTimeoutMs;
const suiteTestTimeoutMs = 60_000;
const noContentStatus = 204;
const resumeSeconds = 31;
const durationSeconds = 120;

interface Fixtures {
  readonly baseURL: string;
  readonly storageState: StorageState;
}

interface WorkerFixtures {
  readonly watchServer: E2EServer;
}

const test = base.extend<Fixtures, WorkerFixtures>({
  watchServer: [
    // Playwright needs the destructuring pattern to see that the fixture has no dependencies.
    // eslint-disable-next-line no-empty-pattern
    async ({}, provide, workerInfo) => {
      const server = await startE2EServer({
        logFile: join(workerInfo.project.outputDir, "server-watch-rows.log"),
        libraries: [
          {
            name: "Movies",
            kind: "movies",
            copyFrom: libraryCorpusFolder(expectedLibrary("movies")),
          },
          {
            name: "Shows",
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
  baseURL: async ({ watchServer }, provide) => {
    await provide(watchServer.baseUrl);
  },
  storageState: async ({ watchServer }, provide) => {
    await provide(await cachedAdminStorageState(watchServer.baseUrl));
  },
});

if (!libraryCorpusExists()) {
  // A skipped suite must not let a CI run pass unnoticed.
  if (process.env["CI"] !== undefined) {
    throw new Error(missingLibraryCorpusReason);
  }
  test.skip(true, missingLibraryCorpusReason);
}
test.describe.configure({ mode: "serial", timeout: suiteTestTimeoutMs });

async function firstMovie(api: APIRequestContext): Promise<ItemSummary> {
  const libraries = (await (await api.get("/api/v1/libraries")).json()) as LibraryList;
  const library = libraries.libraries.find((candidate) => candidate.kind === "movies");
  if (library === undefined) {
    throw new Error("The server has no movies library.");
  }
  const page = (await (await api.get(`/api/v1/libraries/${library.id}/items`)).json()) as ItemPage;
  const [movie] = page.items;
  if (movie === undefined) {
    throw new Error("The movies library has no items.");
  }
  return movie;
}

// The corpus clips are shorter than the 30 s below which the server drops a resume position,
// so the row comes from a stubbed response built on a real movie; the clicks, the requests
// and the reload after them are real.
function continueWatchingRows(movie: ItemSummary, hidden: boolean): HomeRows {
  const watching: ItemSummary = {
    ...movie,
    media: { available: true, durationSeconds: durationSeconds },
    viewerState: { watched: false, positionSeconds: resumeSeconds, playCount: 0 },
  };
  const rows: Row[] = hidden
    ? []
    : [
        {
          id: "continueWatching",
          kind: "continueWatching",
          titleKey: "rows.continueWatching",
          cardFormat: "poster",
          items: [watching],
        },
      ];
  return { rows, hero: [] };
}

test("Continue watching plays from the position and the overflow menu removes the entry", async ({
  page,
  watchServer,
  storageState,
}) => {
  const api = await apiContext(watchServer.baseUrl, storageState);
  const movie = await firstMovie(api);
  await api.dispose();
  let hidden = false;
  await page.route("**/api/v1/home/rows", (route) =>
    route.fulfill({ json: continueWatchingRows(movie, hidden) }),
  );
  await page.route(`**/api/v1/items/${movie.id}/continue-watching`, (route) => {
    hidden = true;
    return route.fulfill({ status: noContentStatus });
  });

  await openApp(page);
  const region = page.getByRole("region", { name: "Continue watching" });
  await expect(region.getByRole("listitem")).toHaveCount(1);
  await expect(region.getByRole("progressbar")).toBeVisible();
  await expect(region.getByRole("link", { name: movie.title })).toHaveAttribute(
    "href",
    `/items/${movie.id}/play?t=${String(resumeSeconds)}`,
  );

  await region.getByRole("listitem").hover();
  await region.getByRole("button", { name: `More actions for ${movie.title}` }).click();
  await page.getByRole("menuitem", { name: "Remove from continue watching" }).click();

  await expect(region).toHaveCount(0);
});

// The e2e server does not store watch state yet (the watch operations answer 501), so the
// listing marks its first show watched and the requests are stubbed.
test("unmarking a show from its card asks first and only then unmarks", async ({ page }) => {
  const unmarked: string[] = [];
  await page.route(/\/api\/v1\/libraries\/[^/]+\/items(\?.*)?$/, async (route) => {
    try {
      const response = await route.fetch();
      const listing = (await response.json()) as ItemPage;
      const [first, ...others] = listing.items;
      const watched = { watched: true, positionSeconds: 0, playCount: 1 };
      await route.fulfill({
        response,
        json: {
          ...listing,
          items: first === undefined ? [] : [{ ...first, viewerState: watched }, ...others],
        },
      });
    } catch {
      // A request the page abandoned (a reload or navigation) has no body to patch any more.
      await route.abort().catch(() => undefined);
    }
  });
  await page.route("**/api/v1/items/*/watched", (route) => {
    unmarked.push(`${route.request().method()} ${route.request().url()}`);
    return route.fulfill({ status: noContentStatus });
  });
  await openApp(page);
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("link", { name: "Shows", exact: true })
    .click();
  const card = page
    .getByRole("main")
    .getByRole("list", { name: "Videos" })
    .getByRole("listitem")
    .first();
  const title = (await card.getByRole("heading").textContent()) ?? "";

  await card.hover();
  await card.getByRole("button", { name: `Mark ${title} as unwatched` }).click();
  const dialog = page.getByRole("dialog", { name: `Mark ${title} as unwatched?` });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
  expect(unmarked).toEqual([]);
  // The card's buttons stay shown while the dialog has focus, so focus can return to the toggle.
  await expect(card.getByRole("button", { name: `Mark ${title} as unwatched` })).toBeFocused();

  await card.hover();
  await card.getByRole("button", { name: `Mark ${title} as unwatched` }).click();
  await dialog.getByRole("button", { name: "Mark as unwatched" }).click();
  await expect(dialog).toHaveCount(0);
  await expect.poll(() => unmarked.length).toBe(1);
  expect(unmarked[0]).toMatch(/^DELETE .*\/watched$/);
});
