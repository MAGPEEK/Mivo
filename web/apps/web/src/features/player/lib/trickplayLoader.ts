import { trickplayPollDelay, type TrickplayLayout } from "@mivo/player-core";

import type { TrickplayApi } from "../api/trickplayApi";

/** What the loader waits with; tests pass instant versions. */
export interface TrickplayLoaderWaits {
  /** Resolves after `milliseconds`, rejects when `signal` aborts. */
  readonly delay: (milliseconds: number, signal: AbortSignal) => Promise<void>;
  /** Resolves when the tab is visible (at once if it is), rejects when `signal` aborts. */
  readonly visible: (signal: AbortSignal) => Promise<void>;
}

/** Everything {@link loadTrickplay} needs. */
export interface TrickplayLoaderOptions {
  readonly fileId: string;
  readonly api: TrickplayApi;
  readonly waits: TrickplayLoaderWaits;
  readonly signal: AbortSignal;
  readonly onLayout: (layout: TrickplayLayout) => void;
}

/**
 * Fetches the manifest of the file's trickplay. A 404 means "not generated yet": the generation
 * is requested once and the manifest is polled with the capped back-off of player-core, paused
 * while the tab is hidden. Every failure ends the load quietly, because a missing preview is not
 * an error; an abort ends it without a call to `onLayout`.
 */
export async function loadTrickplay(options: TrickplayLoaderOptions): Promise<void> {
  try {
    await load(options);
  } catch {
    // Aborted, or the server refused: the seek bar goes on with the time only.
  }
}

async function load({ fileId, api, waits, signal, onLayout }: TrickplayLoaderOptions) {
  const first = await api.getManifest(fileId, signal);
  if (first !== undefined) {
    onLayout(first);
    return;
  }
  await api.requestGeneration(fileId, signal);
  for (let failedAttempts = 1; ; failedAttempts += 1) {
    const wait = trickplayPollDelay(failedAttempts);
    if (wait === undefined) {
      return;
    }
    await waits.delay(wait, signal);
    await waits.visible(signal);
    const layout = await api.getManifest(fileId, signal);
    if (layout !== undefined) {
      onLayout(layout);
      return;
    }
  }
}

function abortError(): Error {
  return new DOMException("aborted", "AbortError");
}

/** The real waits: a timer and the page visibility. */
export const browserWaits: TrickplayLoaderWaits = {
  delay: (milliseconds, signal) =>
    new Promise((resolve, reject) => {
      if (signal.aborted) {
        reject(abortError());
        return;
      }
      const onAbort = () => {
        clearTimeout(timer);
        reject(abortError());
      };
      const timer = setTimeout(() => {
        signal.removeEventListener("abort", onAbort);
        resolve();
      }, milliseconds);
      signal.addEventListener("abort", onAbort, { once: true });
    }),
  visible: (signal) =>
    new Promise((resolve, reject) => {
      if (signal.aborted) {
        reject(abortError());
        return;
      }
      if (!document.hidden) {
        resolve();
        return;
      }
      const onChange = () => {
        if (!document.hidden) {
          cleanup();
          resolve();
        }
      };
      const onAbort = () => {
        cleanup();
        reject(abortError());
      };
      const cleanup = () => {
        document.removeEventListener("visibilitychange", onChange);
        signal.removeEventListener("abort", onAbort);
      };
      document.addEventListener("visibilitychange", onChange);
      signal.addEventListener("abort", onAbort, { once: true });
    }),
};
