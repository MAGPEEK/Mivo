import type { ItemPage, Library, LibraryList } from "@mivo/api-client";
import { expect, test, type Locator, type Page } from "@playwright/test";

import { openApp } from "./support/shell";

// The narrowest phone width WCAG 1.4.10 (reflow) asks for, and a common desktop width.
const phoneViewport = { width: 320, height: 640 };
const desktopViewport = { width: 1280, height: 800 };

const library: Library = {
  id: "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c",
  name: "Movies with a rather long library name",
  kind: "movies",
  available: true,
  itemCount: 2,
  scan: {
    state: "idle",
    counts: { added: 0, updated: 0, removed: 0, unchanged: 0, failed: 0, skipped: 0 },
  },
};

const items: ItemPage = {
  items: [
    {
      id: "item-1",
      libraryId: library.id,
      title: "Big Buck Bunny",
      media: { available: true, container: "mkv", videoCodec: "h264", audioCodec: "aac" },
    },
    { id: "item-2", libraryId: library.id, title: "Sintel", media: { available: false } },
  ],
};

// The library endpoints are not wired in the server yet; the layout only needs their shape.
async function fakeLibraryApi(page: Page): Promise<void> {
  await page.route(
    (url) => url.pathname.startsWith("/api/v1/libraries"),
    (route) => {
      const { pathname } = new URL(route.request().url());
      let body: LibraryList | Library | ItemPage = { libraries: [library] };
      if (pathname.endsWith("/items")) {
        body = items;
      } else if (pathname !== "/api/v1/libraries") {
        body = library;
      }
      return route.fulfill({ json: body });
    },
  );
}

async function expectNoHorizontalScroll(page: Page): Promise<void> {
  const widths = await page.evaluate(() => ({
    scroll: document.documentElement.scrollWidth,
    viewport: window.innerWidth,
  }));
  expect(widths.scroll).toBeLessThanOrEqual(widths.viewport);
}

/**
 * Asserts that `locator` has a size and that nothing lies on top of its centre. Visibility
 * checks ignore overlapping elements, so a sidebar covering the page would pass them.
 */
async function expectUncovered(locator: Locator): Promise<void> {
  const box = await locator.boundingBox();
  expect(box?.width ?? 0).toBeGreaterThan(0);
  const uncovered = await locator.evaluate((element) => {
    const { left, top, width, height } = element.getBoundingClientRect();
    const topmost = document.elementFromPoint(left + width / 2, top + height / 2);
    return topmost !== null && element.contains(topmost);
  });
  expect(uncovered).toBe(true);
}

async function openOverlay(sidebarToggle: Locator): Promise<void> {
  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "false");
  await sidebarToggle.click();
  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "true");
}

test.describe("on a 320 px phone", () => {
  test.use({ viewport: phoneViewport });

  test("home starts with the sidebar closed and fits the screen", async ({ page }) => {
    const { heading } = await openApp(page);

    await expectNoHorizontalScroll(page);
    await expectUncovered(heading);
  });

  test("the open sidebar fits the screen and Escape closes it", async ({ page }) => {
    const { heading, sidebarToggle } = await openApp(page);
    await openOverlay(sidebarToggle);
    await expectNoHorizontalScroll(page);

    await page.keyboard.press("Escape");

    await expect(sidebarToggle).toHaveAttribute("aria-expanded", "false");
    await expect(sidebarToggle).toBeFocused();
    await expectUncovered(heading);
  });

  test("a library link closes the sidebar and the page is not covered", async ({ page }) => {
    await fakeLibraryApi(page);
    const { sidebar, sidebarToggle, heading } = await openApp(page);
    await openOverlay(sidebarToggle);

    await sidebar.getByRole("link", { name: library.name }).click();

    await expect(heading).toHaveText(library.name);
    await expect(sidebarToggle).toHaveAttribute("aria-expanded", "false");
    await expectNoHorizontalScroll(page);
    await expectUncovered(heading);
    await expectUncovered(page.getByRole("link", { name: "Big Buck Bunny" }));
  });

  test("tabbing out of the sidebar closes it before focus lands on the page", async ({ page }) => {
    await fakeLibraryApi(page);
    const { sidebar, sidebarToggle, searchBox } = await openApp(page);
    await openOverlay(sidebarToggle);
    const libraryLink = sidebar.getByRole("link", { name: library.name });
    await libraryLink.focus();

    await page.keyboard.press("Tab");

    await expect(searchBox).toBeFocused();
    await expect(sidebarToggle).toHaveAttribute("aria-expanded", "false");
    await expectUncovered(searchBox);
  });

  test("a press on the page closes the sidebar", async ({ page }) => {
    const { sidebarToggle, heading } = await openApp(page);
    await openOverlay(sidebarToggle);

    await page.mouse.click(phoneViewport.width - 10, phoneViewport.height / 2);

    await expect(sidebarToggle).toHaveAttribute("aria-expanded", "false");
    await expectUncovered(heading);
  });
});

test("the expanded sidebar sits beside the content on a desktop screen", async ({ page }) => {
  await page.setViewportSize(desktopViewport);
  const { sidebar, sidebarToggle, heading } = await openApp(page);
  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "true");

  const sidebarBox = await sidebar.boundingBox();
  const mainBox = await page.getByRole("main").boundingBox();

  expect(sidebarBox).not.toBeNull();
  expect(mainBox?.x ?? 0).toBeGreaterThanOrEqual((sidebarBox?.x ?? 0) + (sidebarBox?.width ?? 0));
  await expectNoHorizontalScroll(page);
  await expectUncovered(heading);
});

test("a desktop window resized to phone width closes the sidebar", async ({ page }) => {
  await page.setViewportSize(desktopViewport);
  const { sidebarToggle, heading } = await openApp(page);
  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "true");

  await page.setViewportSize(phoneViewport);

  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "false");
  await expectNoHorizontalScroll(page);
  await expectUncovered(heading);

  await page.setViewportSize(desktopViewport);
  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "true");
});
