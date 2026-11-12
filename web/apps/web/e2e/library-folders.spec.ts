import { expect, test } from "@playwright/test";

import { blockingAxeViolations } from "./support/axe";
import { webmClip } from "./support/corpus";
import { openLibrary } from "./support/library";

// The folder view of the shared E2E library (task M5-36): the library spec has scanned the
// corpus, so every clip sits in its own "Title (Year)" folder below the library folder.

const clipFolderName = "Open Codecs (2001)";

test("browses a library by folders, restores the folder on reload and Back", async ({ page }) => {
  await openLibrary(page);
  await page.getByRole("link", { name: "Browse folders" }).click();
  const main = page.getByRole("main");

  // The top lists the library's folder by its name, never by its absolute path.
  const top = main.getByRole("list", { name: "Folders" });
  await expect(top.getByRole("link")).toHaveCount(1);
  await top.getByRole("link").click();

  await main.getByRole("link", { name: clipFolderName, exact: true }).click();
  await expect(page).toHaveURL(/\/folders\/[^/]+\/Open%20Codecs%20\(2001\)$/);
  await expect(main.getByRole("link", { name: webmClip.title, exact: true })).toBeVisible();
  const crumbs = main.getByRole("navigation", { name: "Folder path" });
  await expect(crumbs.getByRole("link")).toHaveCount(3);
  await expect(crumbs.getByRole("link").last()).toHaveAttribute("aria-current", "page");
  expect(await blockingAxeViolations(page)).toEqual([]);

  await page.reload();
  await expect(main.getByRole("link", { name: webmClip.title, exact: true })).toBeVisible();

  await page.goBack();
  await expect(main.getByRole("link", { name: clipFolderName, exact: true })).toBeVisible();
});

test("shows the not-found state for a path that cannot be a folder", async ({ page }) => {
  const library = await openLibrary(page);
  await expect(library.grid).toBeVisible();
  const { pathname } = new URL(page.url());
  const folderRequests: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.endsWith("/folders")) {
      folderRequests.push(request.url());
    }
  });

  // A backslash is no part of a library-relative path: refused before any request.
  await page.goto(`${pathname}/folders/a%5Cb`);

  await expect(page.getByRole("heading", { name: "Folder not found" })).toBeVisible();
  expect(folderRequests).toEqual([]);
});

// A browser reads %2e%2e as a dot segment and folds it into the address before the app sees it
// (the route is then simply unknown); a double-encoded one reaches the app as a plain name that
// the server answers with 404. Either way the page shows a not-found state and does not crash.
for (const encoded of ["%2e%2e", "%252e%252e"]) {
  test(`answers a not-found state for /folders/${encoded}/x`, async ({ page }) => {
    const library = await openLibrary(page);
    await expect(library.grid).toBeVisible();
    const { pathname } = new URL(page.url());
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));

    await page.goto(`${pathname}/folders/${encoded}/x`);

    await expect(
      page.getByRole("heading", { name: /^(Folder not found|Page not found)$/ }),
    ).toBeVisible();
    expect(errors).toEqual([]);
  });
}
