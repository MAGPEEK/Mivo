// Numbers of the stream-info overlay that come from the media element and the network (PL-42):
// how much is buffered ahead of the playhead and how fast the stream arrives. Framework-free;
// the app reads the element and the Resource Timing entries and passes plain values.

import { rangeContains, readTimeRanges, type TimeRangesLike } from "../timeline/timeRanges";

/** One finished download of a stream part (an HLS fragment or a range of the file). */
export interface TransferSample {
  /** Body bytes received (Resource Timing `encodedBodySize`). */
  readonly bytes: number;
  /** Time from request start to the last byte, in seconds. */
  readonly durationSeconds: number;
}

/**
 * Throughput is measured over the most recent transfers only, so it follows the network when
 * it changes instead of averaging the whole session. Eight fragments of six seconds each
 * cover about a minute of playback.
 */
export const throughputWindowSize = 8;

/**
 * Seconds of media buffered ahead of `currentTime`: the rest of the buffered range holding the
 * playhead, 0 when nothing is buffered there. Unknown (`undefined`) without an element or with
 * an unusable time.
 */
export function bufferAheadSeconds(
  buffered: TimeRangesLike | undefined,
  currentTime: number,
): number | undefined {
  if (buffered === undefined || !Number.isFinite(currentTime)) {
    return undefined;
  }
  const holding = readTimeRanges(buffered).find((range) => rangeContains(range, currentTime));
  return holding === undefined ? 0 : holding.end - currentTime;
}

/**
 * Bytes per second over the last {@link throughputWindowSize} usable transfers: the sum of
 * their bytes divided by the sum of their durations (so a long transfer weighs more than a
 * short one). Transfers without bytes (served from cache) or without a positive duration are
 * ignored; without any usable transfer the rate is unknown (`undefined`).
 */
export function throughputBytesPerSecond(samples: readonly TransferSample[]): number | undefined {
  const usable = samples
    .filter(
      (sample) =>
        Number.isFinite(sample.bytes) &&
        sample.bytes > 0 &&
        Number.isFinite(sample.durationSeconds) &&
        sample.durationSeconds > 0,
    )
    .slice(-throughputWindowSize);
  const seconds = usable.reduce((sum, sample) => sum + sample.durationSeconds, 0);
  if (seconds === 0) {
    return undefined;
  }
  return usable.reduce((sum, sample) => sum + sample.bytes, 0) / seconds;
}
