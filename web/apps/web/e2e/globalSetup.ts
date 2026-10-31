import type { FullConfig } from "@playwright/test";
import { mkdir } from "node:fs/promises";
import { join } from "node:path";

import { baseUrlVariable } from "./support/baseUrl";
import { brandedChannel } from "./support/brandedChannel";
import { assertFFmpegAvailable, generateCorpus } from "./support/corpus";
import { waitForStartupScans } from "./support/scans";
import { startE2EServer, type E2EServer } from "./support/server";
import { assertNoSessionsLeft } from "./support/sessions";

/**
 * Starts one server for the whole run and returns the teardown that stops it. The corpus is
 * written into its library only after the startup scan, so the library spec's scan is the one
 * that fills the library; every later spec reads that state or plays from it.
 */
export default async function globalSetup(config: FullConfig): Promise<() => Promise<void>> {
  // Before the server build: a missing FFmpeg must fail fast with its own message.
  assertFFmpegAvailable();
  reportMissingBrandedChannel();
  // Playwright empties the output directory before the global setup, so the log is fresh.
  const outputDirectory = config.projects[0]?.outputDir ?? join(config.rootDir, "test-results");
  await mkdir(outputDirectory, { recursive: true });
  const server = await startE2EServer({ logFile: join(outputDirectory, "server.log") });
  try {
    await waitForStartupScans(server.baseUrl);
    await generateCorpus(server.mediaDirectory);
  } catch (error) {
    await server.stop();
    throw error;
  }
  process.env[baseUrlVariable] = server.baseUrl;
  return () => teardown(server);
}

// The list reporter shows skipped tests without their reason, so the reason is printed once.
function reportMissingBrandedChannel(): void {
  const branded = brandedChannel();
  if (!branded.installed) {
    console.warn(`Skipping the branded project: ${branded.skipReason}`);
  }
}

// The sessions are checked while the server still runs: on Windows the stop is a hard kill,
// so only the deletions on navigation can have emptied the directory.
async function teardown(server: E2EServer): Promise<void> {
  try {
    await assertNoSessionsLeft(server.transcodeDirectory);
  } finally {
    await server.stop();
  }
}
