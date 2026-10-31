import { defineConfig, devices } from "@playwright/test";

import { baseUrlVariable } from "./e2e/support/baseUrl";
import { brandedChannel } from "./e2e/support/brandedChannel";

// The specs assert English UI strings, so the browser must not pick up the host's language.
const browserLocale = "en-US";
const testTimeoutMs = 15_000;

// The scan spec fills the library of the shared server; every other spec runs after it.
const librarySpec = /library\.spec\.ts$/;
// Playback in an installed Edge or Chrome with proprietary codecs (ADR 0025, OD-7).
const brandedSpec = /branded\.spec\.ts$/;

const branded = brandedChannel();

/**
 * E2E against the real server binary (tasks M1-17, M2-34): the global setup builds it with the
 * embedded web app, generates a small corpus with FFmpeg, starts the server isolated on a free
 * loopback port and stops it afterwards. The bundled browser is the headless Chromium shell
 * (`playwright install --only-shell chromium`); the `branded` project uses an installed Edge or
 * Chrome and is skipped with a reason when it is missing.
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
  projects: [
    { name: "library", testMatch: librarySpec, use: { ...devices["Desktop Chrome"] } },
    {
      name: "chromium",
      testIgnore: [librarySpec, brandedSpec],
      dependencies: ["library"],
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "branded",
      testMatch: brandedSpec,
      dependencies: ["library"],
      // Without the channel the specs skip themselves; the bundled browser is never launched.
      use: {
        ...devices["Desktop Chrome"],
        ...(branded.installed ? { channel: branded.channel } : {}),
      },
    },
  ],
});
