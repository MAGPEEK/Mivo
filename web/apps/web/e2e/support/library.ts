import { expect, type Locator, type Page } from "@playwright/test";

import type { CorpusClip } from "./corpus";
import { e2eLibraryName } from "./server";
import { openApp } from "./shell";

/** The parts of the library page the specs use. */
export interface LibraryPage {
  /** The video grid. */
  readonly grid: Locator;
  /** Status line of the scan button: progress, result or error. */
  readonly scanStatus: Locator;
}

/** Opens the E2E library the way a user does: home, then its link in the sidebar. */
export async function openLibrary(page: Page): Promise<LibraryPage> {
  const { sidebar } = await openApp(page);
  await sidebar.getByRole("link", { name: e2eLibraryName, exact: true }).click();
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { level: 1, name: e2eLibraryName })).toBeVisible();
  return {
    grid: main.getByRole("list", { name: "Videos" }),
    // The scan control sits above the grid, whose loading skeleton is a status, too.
    scanStatus: main.getByRole("status").first(),
  };
}

/** The card of `clip` in `grid`, told apart from a card with the same title by its chip. */
export function itemCard(grid: Locator, clip: CorpusClip): Locator {
  return grid
    .getByRole("listitem")
    .filter({ has: grid.page().getByRole("link", { name: clip.title, exact: true }) })
    .filter({ has: grid.page().getByText(clip.chip, { exact: true }) });
}

/** Opens the detail page of `clip` from the library grid. */
export async function openItem(page: Page, clip: CorpusClip): Promise<void> {
  const library = await openLibrary(page);
  await itemCard(library.grid, clip).getByRole("link").click();
  await expect(page.getByRole("heading", { level: 1, name: clip.title })).toBeVisible();
}
