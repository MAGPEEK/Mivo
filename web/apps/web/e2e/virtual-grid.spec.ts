import type { ItemPage, ItemSummary, LetterIndex, Library, LibraryList } from "@mivo/api-client";
import { expect, test, type Locator, type Page } from "@playwright/test";

import { blockingAxeViolations } from "./support/axe";
import { openApp } from "./support/shell";

// The virtual grid and the A-Z bar (task M5-34) in a real browser. A listing of 2,600 movies is
// served by the page's own routes, so the test needs neither a scan nor a big corpus: it is the
// layout, scrolling and focus behaviour of a real browser that count here.

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ".split("");
const moviesPerLetter = 100;
const totalCount = letters.length * moviesPerLetter;
const pageLimitFallback = 100;
const notFoundStatus = 404;

const library: Library = {
  id: "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c",
  name: "Big movies",
  kind: "movies",
  available: true,
  itemCount: totalCount,
  scan: {
    state: "idle",
    counts: { added: 0, updated: 0, removed: 0, unchanged: 0, failed: 0, skipped: 0 },
    unavailableFolderCount: 0,
  },
  folders: [{ id: "6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d", path: "/media/movies", available: true }],
  metadataLanguage: "",
  options: { scanExtras: true, readNfo: true, localArtwork: true },
  importedFromConfig: false,
};

// "A title 1" to "A title 100", then "B title 1" and so on: title order is list order.
const movies: readonly ItemSummary[] = letters.flatMap((letter) =>
  Array.from({ length: moviesPerLetter }, (_unused, index) => ({
    id: `movie-${letter}-${String(index + 1)}`,
    libraryId: library.id,
    kind: "movie" as const,
    title: `${letter} title ${String(index + 1)}`,
    media: { available: true, container: "mkv", videoCodec: "h264", audioCodec: "aac" },
  })),
);

const letterIndex: LetterIndex = {
  totalCount,
  letters: letters.map((letter, index) => ({ letter, offset: index * moviesPerLetter })),
};

function itemPage(url: URL): ItemPage {
  const limit = Number(url.searchParams.get("limit") ?? pageLimitFallback);
  const start = Number(url.searchParams.get("cursor") ?? 0);
  const end = Math.min(totalCount, start + limit);
  const ordered = url.searchParams.get("order") === "desc" ? [...movies].reverse() : movies;
  return {
    items: ordered.slice(start, end),
    totalCount,
    ...(end < totalCount ? { nextCursor: String(end) } : {}),
  };
}

async function fakeBigLibrary(page: Page): Promise<void> {
  await page.route(
    (url) => url.pathname.startsWith("/api/v1/libraries"),
    (route) => {
      const url = new URL(route.request().url());
      let body: LibraryList | Library | ItemPage | LetterIndex = { libraries: [library] };
      if (url.pathname.endsWith("/items")) {
        body = itemPage(url);
      } else if (url.pathname.endsWith("/letters")) {
        body = letterIndex;
      } else if (url.pathname !== "/api/v1/libraries") {
        body = library;
      }
      return route.fulfill({ json: body });
    },
  );
  // Opening a card leaves the listing; the item page itself is not under test.
  await page.route("**/api/v1/items/**", (route) =>
    route.fulfill({
      status: notFoundStatus,
      contentType: "application/problem+json",
      json: { type: "about:blank", title: "not_found", status: notFoundStatus, code: "not_found" },
    }),
  );
}

async function openBigLibrary(page: Page): Promise<Locator> {
  await fakeBigLibrary(page);
  await openApp(page, `/libraries/${library.id}`);
  const grid = page.getByRole("main").getByRole("list", { name: "Videos" });
  await expect(grid.getByRole("link", { name: "A title 1", exact: true })).toBeVisible();
  return grid;
}

function card(grid: Locator, title: string): Locator {
  return grid.getByRole("link", { name: title, exact: true });
}

async function windowScroll(page: Page): Promise<number> {
  return page.evaluate(() => window.scrollY);
}

// A wheel scrolls smoothly; the position counts once two reads a moment apart agree.
async function settledScroll(page: Page): Promise<number> {
  let last = -1;
  await expect
    .poll(async () => {
      const now = await windowScroll(page);
      const settled = now === last;
      last = now;
      return settled;
    })
    .toBe(true);
  return last;
}

async function focusedPosition(page: Page): Promise<number> {
  return page.evaluate(() =>
    Number(document.activeElement?.closest("li")?.getAttribute("aria-posinset") ?? 0),
  );
}

test("keeps only the rows near the window in the DOM", async ({ page }) => {
  const grid = await openBigLibrary(page);

  const first = grid.getByRole("listitem").first();
  await expect(first).toHaveAttribute("aria-setsize", String(totalCount));
  expect(await grid.getByRole("listitem").count()).toBeLessThan(80);
  const height = await grid.evaluate((element) => element.getBoundingClientRect().height);
  expect(height).toBeGreaterThan(totalCount * 20);

  await page.mouse.move(400, 400);
  await page.mouse.wheel(0, 20_000);
  await expect(card(grid, "A title 1")).toHaveCount(0);
  expect(await grid.getByRole("listitem").count()).toBeLessThan(80);
  expect(await blockingAxeViolations(page)).toEqual([]);
});

test("the A-Z bar jumps to the first title of a letter and focuses it", async ({ page }) => {
  const grid = await openBigLibrary(page);
  const bar = page.getByRole("navigation", { name: "Jump to letter" });
  await expect(bar.getByRole("button", { name: "Numbers and symbols" })).toBeDisabled();

  await bar.getByRole("button", { name: "M", exact: true }).click();

  const target = card(grid, "M title 1");
  await expect(target).toBeFocused();
  const box = await target.boundingBox();
  // Below the 56 px sticky top bar and inside the window.
  expect(box?.y ?? 0).toBeGreaterThan(56);
  expect(box?.y ?? Number.MAX_SAFE_INTEGER).toBeLessThan(page.viewportSize()?.height ?? 0);
});

test("arrow keys and Page Down move the focus through the grid, which keeps it", async ({
  page,
}) => {
  const grid = await openBigLibrary(page);
  await card(grid, "A title 1").focus();

  await page.keyboard.press("ArrowRight");
  expect(await focusedPosition(page)).toBe(2);
  await page.keyboard.press("ArrowDown");
  const afterDown = await focusedPosition(page);
  expect(afterDown).toBeGreaterThan(2);
  await page.keyboard.press("PageDown");
  expect(await focusedPosition(page)).toBeGreaterThan(afterDown);
  await page.keyboard.press("ArrowLeft");
  await page.keyboard.press("ArrowUp");

  const position = await focusedPosition(page);
  expect(position).toBeGreaterThan(1);
  await expect(page.locator(":focus")).toBeVisible();
});

test("Back from an item restores the scroll position of the listing", async ({ page }) => {
  const grid = await openBigLibrary(page);
  await page.mouse.move(400, 400);
  await page.mouse.wheel(0, 6_000);
  const before = await settledScroll(page);
  expect(before).toBeGreaterThan(5_000);

  await grid.getByRole("link").first().click();
  await expect(page).toHaveURL(/\/items\//);
  await page.goBack();

  await expect(grid).toBeVisible();
  await expect.poll(async () => Math.abs((await windowScroll(page)) - before)).toBeLessThan(150);
});

test("a changed sort starts the listing at the top", async ({ page }) => {
  const grid = await openBigLibrary(page);
  await page.mouse.move(400, 400);
  await page.mouse.wheel(0, 6_000);
  await expect.poll(() => windowScroll(page)).toBeGreaterThan(5_000);
  const toolbar = page.getByRole("group", { name: "Browse options" });

  await toolbar.getByRole("button", { name: "Sort direction: Ascending" }).click();

  await expect(page).toHaveURL(/[?&]order=desc(&|$)/);
  await expect(grid.getByRole("link", { name: "Z title 100", exact: true })).toBeVisible();
  expect(await windowScroll(page)).toBeLessThan(50);
});
