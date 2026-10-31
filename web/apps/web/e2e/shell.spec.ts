import type { SystemInfo } from "@mivo/api-client";
import { expect, test } from "@playwright/test";

import { e2eLibraryName } from "./support/server";
import { abortSystemInfo, openApp } from "./support/shell";

// Enough intermediate pointer positions to land several times in the gap beside the icon.
const pointerSteps = 20;

test("renders the shell with navigation, search, home heading and empty state", async ({
  page,
}) => {
  const { sidebar, searchBox, heading } = await openApp(page);

  await expect(sidebar).toBeVisible();
  await expect(searchBox).toBeVisible();
  await expect(heading).toHaveText("Home");
  await expect(page.getByRole("main").getByText("Your library is empty")).toBeVisible();
});

test("shows the version the system info API reports in the sidebar footer", async ({
  page,
  request,
}) => {
  const response = await request.get("/api/v1/system/info");
  expect(response.status()).toBe(200);
  const { version } = (await response.json()) as SystemInfo;
  expect(version).not.toBe("");

  const { serverStatus } = await openApp(page);

  await expect(serverStatus).toHaveText(`Server version ${version}`);
});

test("focuses the search box with Control+K", async ({ page }) => {
  const { searchBox } = await openApp(page);
  await expect(searchBox).not.toBeFocused();

  await page.keyboard.press("Control+K");

  await expect(searchBox).toBeFocused();
});

test("collapses and expands the sidebar and keeps the state across reloads", async ({ page }) => {
  const { sidebar, sidebarToggle } = await openApp(page);
  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "true");
  const expandedWidth = (await sidebar.boundingBox())?.width ?? 0;

  await sidebarToggle.click();
  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "false");
  await expect.poll(async () => (await sidebar.boundingBox())?.width).toBeLessThan(expandedWidth);
  await page.reload();
  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "false");

  await sidebarToggle.click();
  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "true");
  await page.reload();
  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "true");
});

test("serves the app for a deep link and shows the not-found view", async ({ page }) => {
  const response = await page.goto("/does-not-exist");
  expect(response?.status()).toBe(200);

  await expect(page.getByRole("heading", { level: 1, name: "Page not found" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Back to home" })).toBeVisible();
});

test("keeps an unreachable server visible and reachable in the collapsed sidebar", async ({
  page,
}) => {
  await abortSystemInfo(page);
  const { sidebarToggle, serverStatus } = await openApp(page);
  await sidebarToggle.click();
  const indicator = serverStatus.getByRole("button", { name: "Server unreachable" });
  const tooltip = indicator.getByText("Server unreachable");
  await expect(indicator).toBeVisible();
  await expect(tooltip).toHaveCSS("opacity", "0");

  // Keyboard: the label appears on focus, Enter expands the sidebar and focus stays in it.
  await sidebarToggle.focus();
  await page.keyboard.press("Tab");
  await expect(page.getByRole("link", { name: "Home" })).toBeFocused();
  // The shared server has one library (task M2-34); its link comes before the footer.
  await page.keyboard.press("Tab");
  await expect(page.getByRole("link", { name: e2eLibraryName, exact: true })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(indicator).toBeFocused();
  await expect(tooltip).toHaveCSS("opacity", "1");
  // WCAG 1.4.13: Escape hides the tooltip without moving focus.
  await page.keyboard.press("Escape");
  await expect(tooltip).toHaveCSS("opacity", "0");
  await expect(indicator).toBeFocused();
  await page.keyboard.press("Enter");

  await expect(sidebarToggle).toHaveAttribute("aria-expanded", "true");
  await expect(sidebarToggle).toBeFocused();
  await expect(serverStatus).toHaveText("Server unreachable");
});

test("keeps the unreachable tooltip open while the pointer moves onto it", async ({ page }) => {
  await abortSystemInfo(page);
  const { sidebarToggle, serverStatus } = await openApp(page);
  await sidebarToggle.click();
  const indicator = serverStatus.getByRole("button", { name: "Server unreachable" });
  const tooltip = indicator.getByText("Server unreachable");

  // hover() waits until the collapse animation has settled, so the boxes below are final.
  await indicator.hover();
  await expect(tooltip).toHaveCSS("opacity", "1");
  const tooltipBox = await tooltip.boundingBox();
  expect(tooltipBox).not.toBeNull();
  const { x, y, width, height } = tooltipBox ?? { x: 0, y: 0, width: 0, height: 0 };
  // Small steps cross the gap between icon and text the way a real pointer does.
  await page.mouse.move(x + width / 2, y + height / 2, { steps: pointerSteps });
  await expect(tooltip).toHaveCSS("opacity", "1");

  // WCAG 1.4.13: Escape hides hovered content as well, without moving the pointer.
  await page.keyboard.press("Escape");
  await expect(tooltip).toHaveCSS("opacity", "0");
});
