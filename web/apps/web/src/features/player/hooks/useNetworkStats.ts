import { bufferAheadSeconds, throughputBytesPerSecond } from "@mivo/player-core";
import { useEffect, useState } from "react";

import { streamPartsPrefix, transferSamples } from "../lib/resourceTransfers";

/** What the stream-info overlay shows about the buffer and the network; `undefined` = unknown. */
export interface NetworkStats {
  /** Seconds buffered ahead of the playhead. */
  readonly bufferAheadSeconds: number | undefined;
  /** Bytes per second of the stream's recent fragments (rule: `throughputBytesPerSecond`). */
  readonly throughputBytesPerSecond: number | undefined;
}

const unknownStats: NetworkStats = {
  bufferAheadSeconds: undefined,
  throughputBytesPerSecond: undefined,
};

/** The numbers change a few times per second at most; once a second is enough to read. */
export const networkStatsIntervalMilliseconds = 1000;

/**
 * The browser keeps 250 resource entries by default and silently drops new ones when the
 * buffer is full, which a long HLS stream fills in minutes; the throughput would freeze. While
 * the overlay samples, the buffer is enlarged and doubled each time it reports being full.
 * Returns the function that stops listening.
 */
function growResourceTimingBuffer(): () => void {
  if (
    typeof performance.setResourceTimingBufferSize !== "function" ||
    typeof performance.addEventListener !== "function"
  ) {
    return () => undefined;
  }
  let size = resourceTimingBufferSize;
  performance.setResourceTimingBufferSize(size);
  const onFull = () => {
    size *= 2;
    performance.setResourceTimingBufferSize(size);
  };
  performance.addEventListener("resourcetimingbufferfull", onFull);
  return () => {
    performance.removeEventListener("resourcetimingbufferfull", onFull);
  };
}

/** Entries kept while sampling: about an hour of six-second fragments. */
export const resourceTimingBufferSize = 1000;

function readStats(video: HTMLVideoElement, streamUrl: string): NetworkStats {
  const prefix = streamPartsPrefix(streamUrl, window.location.href);
  const entries = performance
    .getEntriesByType("resource")
    .filter((entry): entry is PerformanceResourceTiming => entry.entryType === "resource");
  return {
    bufferAheadSeconds: bufferAheadSeconds(video.buffered, video.currentTime),
    throughputBytesPerSecond: throughputBytesPerSecond(transferSamples(entries, prefix)),
  };
}

/**
 * Samples the buffer ahead (from the media element) and the throughput (from Resource Timing
 * entries of the stream's URL directory) once a second, but only while `active` (the overlay is
 * open), so a closed overlay costs nothing.
 */
export function useNetworkStats(
  video: HTMLVideoElement | null,
  streamUrl: string | undefined,
  active: boolean,
): NetworkStats {
  const [stats, setStats] = useState<NetworkStats>(unknownStats);
  useEffect(() => {
    if (!active || video === null || streamUrl === undefined) {
      return undefined;
    }
    const read = () => {
      setStats(readStats(video, streamUrl));
    };
    const stopGrowing = growResourceTimingBuffer();
    const first = window.setTimeout(read, 0);
    const repeat = window.setInterval(read, networkStatsIntervalMilliseconds);
    return () => {
      window.clearTimeout(first);
      window.clearInterval(repeat);
      stopGrowing();
      // The next opening starts unknown instead of showing the old numbers for a second.
      setStats(unknownStats);
    };
  }, [active, video, streamUrl]);
  return active && video !== null && streamUrl !== undefined ? stats : unknownStats;
}
