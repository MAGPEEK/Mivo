import type { ProgressReport } from "@mivo/api-client";
import { createProgressReporter, type StartTimer } from "@mivo/player-core";
import { useEffect } from "react";

import { useApiClient } from "../../../shared/api";
import { createProgressSender } from "../api/progressApi";
import { trackProgress, type ProgressMedia, type ProgressSource } from "../lib/progressTracking";

/** What {@link useProgressReporting} reports, and from where. */
export interface ProgressReportingOptions {
  readonly itemId: string;
  /** The playback engine, which knows the position and the session. */
  readonly source: ProgressSource;
  readonly media: (ProgressMedia & EventTarget) | null;
  /**
   * Seconds between the file position and the reported one (`progressTarget`); `undefined`
   * while the item is unknown and for items that do not report (music, photos).
   */
  readonly offsetSeconds: number | undefined;
  readonly startTimer: StartTimer;
}

function progressReport(positionSeconds: number, sessionId: string | undefined): ProgressReport {
  return sessionId === undefined
    ? { positionSeconds }
    : { positionSeconds, playbackSessionId: sessionId };
}

/**
 * Tells the server the position of the item while it plays (task M7-35, ADR 0032 section 8),
 * so resume and Continue watching work on every device. Failures are never shown and never
 * touch playback; the reporter sends the position again with its next report.
 *
 * Call it before the effect that opens the engine: effects are cleaned up in the order they
 * were declared, and the last report reads the position before the engine closes.
 */
export function useProgressReporting(options: ProgressReportingOptions): void {
  const { itemId, source, media, offsetSeconds, startTimer } = options;
  const client = useApiClient();
  useEffect(() => {
    if (media === null || offsetSeconds === undefined) {
      return;
    }
    const send = createProgressSender(client, itemId);
    const reporter = createProgressReporter({
      send: (positionSeconds) =>
        send(progressReport(positionSeconds, source.getSnapshot().session?.id)),
      clock: { now: () => Date.now(), startTimer },
    });
    return trackProgress({ source, media, page: window, reporter, offsetSeconds });
  }, [client, itemId, source, media, offsetSeconds, startTimer]);
}
