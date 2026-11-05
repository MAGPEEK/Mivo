import { expect, test, type Locator, type Page } from "@playwright/test";

import { browseFixtureFolderCount, browseFixtureFolderName } from "./support/server";
import { openApp } from "./support/shell";

// The narrowest phone width WCAG 1.4.10 asks for, where the browser is full screen, and a
// common desktop size.
const viewports = [
  { name: "phone", size: { width: 320, height: 640 } },
  { name: "desktop", size: { width: 1280, height: 800 } },
] as const;

const addLibraryPath = "/admin/manage/libraries?add=1";

async function openFolderBrowser(page: Page): Promise<Locator> {
  await openApp(page, addLibraryPath);
  const libraryDialog = page.getByRole("dialog", { name: "Add library" });
  await libraryDialog.getByRole("tab", { name: "Folders" }).click();
  await libraryDialog.getByRole("button", { name: "Add folder" }).click();
  const browser = page.getByRole("dialog", { name: "Choose folder" });
  await expect(browser.getByRole("list", { name: "Sub-folders" })).toBeVisible();
  return browser;
}

/** True when the element's content is wider than its box, i.e. it scrolls sideways. */
function scrollsHorizontally(element: Locator): Promise<boolean> {
  return element.evaluate((node) => node.scrollWidth > node.clientWidth);
}

for (const viewport of viewports) {
  test(`keeps "Select this folder" in view above a long folder list on ${viewport.name} (FIX-folder-select)`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport.size);
    const browser = await openFolderBrowser(page);
    const select = browser.getByRole("button", { name: "Select this folder" });
    const lastFolder = browser.getByRole("button", {
      name: browseFixtureFolderName(browseFixtureFolderCount - 1),
    });

    await expect(select).toBeInViewport({ ratio: 1 });
    await expect(browser.getByRole("button", { name: "Up" })).toBeInViewport({ ratio: 1 });
    await expect(lastFolder).not.toBeInViewport();
    await lastFolder.scrollIntoViewIfNeeded();
    await expect(lastFolder).toBeInViewport();
    await expect(select).toBeInViewport({ ratio: 1 });
    expect(await scrollsHorizontally(browser)).toBe(false);
    expect(await scrollsHorizontally(page.locator("html"))).toBe(false);

    await select.click();
    await expect(browser).toBeHidden();
    const libraryDialog = page.getByRole("dialog", { name: "Add library" });
    await expect(
      libraryDialog.getByRole("list", { name: "Folders" }).getByRole("listitem"),
    ).toHaveCount(1);
  });
}
