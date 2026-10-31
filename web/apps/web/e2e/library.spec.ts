import { expect, test, type Route } from "@playwright/test";

import { blockingAxeViolations } from "./support/axe";
import { corpusClips } from "./support/corpus";
import { itemCard, openLibrary } from "./support/library";
import { e2eLibraryName } from "./support/server";

// Runs before every other spec (project dependency): the scan here fills the library of the
// shared server, which the playback specs then play from. The global setup wrote the corpus
// after the server's startup scan, so this scan adds every clip.

test.describe.configure({ mode: "serial" });

/** Holds the scan start response until `release` is called, so the running phase is visible. */
function holdScanStart(): { readonly handler: (route: Route) => Promise<void>; release(): void } {
  let resolveRelease: (() => void) | undefined;
  const released = new Promise<void>((resolve) => {
    resolveRelease = resolve;
  });
  return {
    handler: async (route) => {
      const response = await route.fetch();
      await released;
      await route.fulfill({ response });
    },
    release: () => {
      resolveRelease?.();
    },
  };
}

test("shows the library, scans it and lists the new videos", async ({ page }) => {
  // A scan of a few small clips can be finished before its start request is answered; the
  // held answer keeps the button in its running state long enough to see it.
  const scanStart = holdScanStart();
  await page.route("**/api/v1/libraries/*/scan", scanStart.handler);
  const library = await openLibrary(page);
  await expect(page.getByRole("heading", { level: 1, name: e2eLibraryName })).toBeVisible();
  await expect(page.getByText("No videos yet")).toBeVisible();

  await page.getByRole("button", { name: "Scan library" }).click();

  await expect(page.getByRole("button", { name: "Scanning…" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  await expect(library.scanStatus).toHaveText("Scanning the library folder…");
  scanStart.release();
  await expect(library.scanStatus).toHaveText(
    new RegExp(
      `^Last scan: ${String(corpusClips.length)} added, 0 updated, 0 removed, 0 unchanged, ` +
        "0 failed, \\d+ skipped\\.$",
    ),
  );
  await expect(page.getByRole("button", { name: "Scan library" })).toBeVisible();
  const titles = library.grid.getByRole("listitem").getByRole("link");
  await expect(titles).toHaveText(corpusClips.map((clip) => clip.title).toSorted());
  for (const clip of corpusClips) {
    await expect(itemCard(library.grid, clip)).toHaveCount(1);
  }
});

test("the library page has no serious or critical axe violations", async ({ page }) => {
  const library = await openLibrary(page);
  // Scan the settled page: the grid has replaced its loading skeleton.
  await expect(library.grid.getByRole("listitem")).toHaveCount(corpusClips.length);
  await expect(library.scanStatus).toContainText("Last scan:");

  expect(await blockingAxeViolations(page)).toEqual([]);
});
