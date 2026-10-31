// Checks that the playback specs left no session behind: the player stops its session when the
// page is left, and the server then deletes the session's directory (ADR 0024).
import { readdir } from "node:fs/promises";
import { join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";

// The last DELETE of a spec may still be in flight while the runner moves on to the teardown.
const settleTimeoutMs = 10_000;
const pollIntervalMs = 200;

async function sessionEntries(sessionsDirectory: string): Promise<string[]> {
  try {
    return await readdir(sessionsDirectory);
  } catch (error) {
    // The server creates the directory with the first session; none means nothing is left.
    if (error instanceof Error && "code" in error && error.code === "ENOENT") {
      return [];
    }
    throw error;
  }
}

/**
 * Waits until `<transcodeDirectory>/sessions` is empty.
 * @throws Error listing the remaining entries when it does not empty in time.
 */
export async function assertNoSessionsLeft(transcodeDirectory: string): Promise<void> {
  const sessionsDirectory = join(transcodeDirectory, "sessions");
  const deadline = Date.now() + settleTimeoutMs;
  let entries = await sessionEntries(sessionsDirectory);
  while (entries.length > 0 && Date.now() < deadline) {
    await delay(pollIntervalMs);
    entries = await sessionEntries(sessionsDirectory);
  }
  if (entries.length > 0) {
    throw new Error(
      `${sessionsDirectory} still holds ${entries.join(", ")} after the run: a player did not ` +
        "stop its session when the page was left, or the server did not delete it.",
    );
  }
}
