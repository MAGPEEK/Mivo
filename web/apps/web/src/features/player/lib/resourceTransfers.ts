import type { TransferSample } from "@mivo/player-core";

/** The parts of a Resource Timing entry the throughput reads. */
export interface ResourceEntry {
  readonly name: string;
  /** Body bytes as received, before decoding; 0 for a cached answer. */
  readonly encodedBodySize: number;
  /** Request start to last byte, in milliseconds. */
  readonly duration: number;
}

const millisecondsPerSecond = 1000;

/**
 * The absolute URL prefix every request of a session's stream starts with: the directory of
 * the stream URL (the HLS playlist's fragments, or the direct-play file's range requests
 * and siblings). Same-origin, so Resource Timing reports sizes.
 */
export function streamPartsPrefix(streamUrl: string, pageUrl: string): string {
  const absolute = new URL(streamUrl, pageUrl);
  return new URL(".", absolute).href;
}

/** The finished transfers of the stream's parts among `entries`, oldest first. */
export function transferSamples(
  entries: readonly ResourceEntry[],
  prefix: string,
): TransferSample[] {
  return entries
    .filter((entry) => entry.name.startsWith(prefix))
    .map((entry) => ({
      bytes: entry.encodedBodySize,
      durationSeconds: entry.duration / millisecondsPerSecond,
    }));
}
