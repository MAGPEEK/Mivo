// Waits for the library scan the server starts on its own at startup (OD-3), so the corpus is
// written into a library that the next scan, the one the library spec starts, finds new.
import type { LibraryList } from "@mivo/api-client";
import type { APIRequestContext } from "@playwright/test";
import { setTimeout as delay } from "node:timers/promises";

import { apiContext, cachedAdminStorageState } from "./auth";

const librariesPath = "/api/v1/libraries";
const defaultScanTimeoutMs = 30_000;
const pollIntervalMs = 100;
const requestTimeoutMs = 5_000;

/** Whether every library has finished a scan and none is running. */
function allScansFinished(list: LibraryList): boolean {
  return list.libraries.every(
    (library) => library.scan.state !== "running" && library.scan.finishedAt !== undefined,
  );
}

/** @throws Error when a scan stopped early: waiting longer would only hide the cause. */
function assertNoFailedScan(list: LibraryList): void {
  const failed = list.libraries.find((library) => library.scan.state === "failed");
  if (failed !== undefined) {
    throw new Error(`The scan of library ${failed.name} failed: ${JSON.stringify(failed.scan)}`);
  }
}

/**
 * Polls the library list of the server at `baseUrl` until every library has finished its
 * startup scan. It signs in first when the server requires it (task M7-28).
 * @throws Error when a library has not finished a scan within `scanTimeoutMs`.
 */
export async function waitForStartupScans(
  baseUrl: string,
  scanTimeoutMs = defaultScanTimeoutMs,
): Promise<void> {
  const context = await apiContext(baseUrl, await cachedAdminStorageState(baseUrl));
  try {
    await pollScans(context, scanTimeoutMs);
  } finally {
    await context.dispose();
  }
}

async function pollScans(context: APIRequestContext, scanTimeoutMs: number): Promise<void> {
  const deadline = Date.now() + scanTimeoutMs;
  let last = "no answer";
  while (Date.now() < deadline) {
    const response = await context.get(librariesPath, { timeout: requestTimeoutMs });
    if (response.ok()) {
      const list = (await response.json()) as LibraryList;
      assertNoFailedScan(list);
      if (allScansFinished(list)) {
        return;
      }
      last = JSON.stringify(list.libraries.map((library) => library.scan));
    } else {
      last = `HTTP ${String(response.status())}`;
    }
    await delay(pollIntervalMs);
  }
  throw new Error(
    `The startup scan did not finish within ${String(scanTimeoutMs)} ms; last status: ${last}`,
  );
}
