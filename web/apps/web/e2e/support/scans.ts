// Waits for the library scan the server starts on its own at startup (OD-3), so the corpus is
// written into a library that the next scan, the one the library spec starts, finds new.
import type { LibraryList } from "@mivo/api-client";
import { setTimeout as delay } from "node:timers/promises";

const librariesPath = "/api/v1/libraries";
const scanTimeoutMs = 30_000;
const pollIntervalMs = 100;

/** Whether every library has finished a scan and none is running. */
function allScansFinished(list: LibraryList): boolean {
  return list.libraries.every(
    (library) => library.scan.state !== "running" && library.scan.finishedAt !== undefined,
  );
}

/**
 * Polls the library list of the server at `baseUrl` until every library has finished its
 * startup scan.
 * @throws Error when a library has not finished a scan within the timeout.
 */
export async function waitForStartupScans(baseUrl: string): Promise<void> {
  const deadline = Date.now() + scanTimeoutMs;
  let last = "no answer";
  while (Date.now() < deadline) {
    const response = await fetch(`${baseUrl}${librariesPath}`);
    if (response.ok) {
      const list = (await response.json()) as LibraryList;
      if (allScansFinished(list)) {
        return;
      }
      last = JSON.stringify(list.libraries.map((library) => library.scan));
    } else {
      await response.body?.cancel();
      last = `HTTP ${String(response.status)}`;
    }
    await delay(pollIntervalMs);
  }
  throw new Error(
    `The startup scan did not finish within ${String(scanTimeoutMs)} ms; last status: ${last}`,
  );
}
