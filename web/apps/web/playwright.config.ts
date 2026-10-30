import { defineConfig, devices } from "@playwright/test";

import { baseUrlVariable } from "./e2e/support/baseUrl";

// The specs assert English UI strings, so the browser must not pick up the host's language.
const browserLocale = "en-US";
const testTimeoutMs = 15_000;

/**
 * E2E against the real server binary (task M1-17): the global setup builds it with the
 * embedded web app, starts it isolated on a free loopback port and stops it afterwards.
 * Only the headless Chromium shell is needed: `playwright install --only-shell chromium`.
 */
export default defineConfig({
  testDir: "./e2e",
  outputDir: "./test-results",
  globalSetup: "./e2e/globalSetup.ts",
  timeout: testTimeoutMs,
  fullyParallel: true,
  // A focused test committed by accident must fail the CI run instead of skipping the rest.
  forbidOnly: process.env["CI"] !== undefined,
  reporter: [["list"], ["html", { outputFolder: "playwright-report", open: "never" }]],
  use: {
    // Undefined while the runner loads the config before the global setup; set in workers.
    baseURL: process.env[baseUrlVariable],
    locale: browserLocale,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
