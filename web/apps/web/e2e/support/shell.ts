import { expect, type Locator, type Page } from "@playwright/test";

/** The landmarks and controls of the app shell, found the way assistive technology finds them. */
export interface Shell {
  readonly sidebar: Locator;
  readonly sidebarToggle: Locator;
  readonly serverStatus: Locator;
  readonly searchBox: Locator;
  readonly heading: Locator;
}

/** Locates the shell of the English UI on `page`. */
export function shell(page: Page): Shell {
  const sidebar = page.getByRole("navigation", { name: "Main navigation" });
  return {
    sidebar,
    sidebarToggle: page.getByRole("button", { name: "Toggle sidebar" }),
    serverStatus: sidebar.getByRole("status"),
    searchBox: page.getByRole("searchbox", { name: "Search" }),
    heading: page.getByRole("heading", { level: 1 }),
  };
}

/**
 * Opens `path` and waits until the app has rendered its page heading, i.e. React is mounted
 * and its event listeners are in place.
 */
export async function openApp(page: Page, path = "/"): Promise<Shell> {
  await page.goto(path);
  const appShell = shell(page);
  await expect(appShell.heading).toBeVisible();
  return appShell;
}

/** Makes the system info request fail as if the server were unreachable. */
export async function abortSystemInfo(page: Page): Promise<void> {
  await page.route("**/api/v1/system/info", (route) => route.abort("connectionrefused"));
}
