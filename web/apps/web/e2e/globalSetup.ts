import type { FullConfig } from "@playwright/test";
import { mkdir } from "node:fs/promises";
import { join } from "node:path";

import { baseUrlVariable } from "./support/baseUrl";
import { startE2EServer } from "./support/server";

/**
 * Starts one server for the whole run and returns the teardown that stops it. The tests only
 * read server state, so sharing the server keeps them independent and the suite fast.
 */
export default async function globalSetup(config: FullConfig): Promise<() => Promise<void>> {
  // Playwright empties the output directory before the global setup, so the log is fresh.
  const outputDirectory = config.projects[0]?.outputDir ?? join(config.rootDir, "test-results");
  await mkdir(outputDirectory, { recursive: true });
  const server = await startE2EServer({ logFile: join(outputDirectory, "server.log") });
  process.env[baseUrlVariable] = server.baseUrl;
  return server.stop;
}
