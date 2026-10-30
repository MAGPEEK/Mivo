import type { ContainerCandidate, ContainerSupport, SupportLevel } from "./capabilities";

/**
 * Answers whether a MIME type is playable, like `HTMLMediaElement.canPlayType`. A media element
 * satisfies this interface, so callers inject one instead of player-core touching the DOM.
 */
export interface MediaTypeProbe {
  canPlayType(mimeType: string): string;
}

/** The containers a client is probed for when the caller does not pass its own list. */
export const defaultContainerCandidates: readonly ContainerCandidate[] = [
  { container: "mp4", mimeType: "video/mp4" },
  { container: "webm", mimeType: "video/webm" },
  { container: "matroska", mimeType: "video/x-matroska" },
  { container: "mpegts", mimeType: "video/mp2t" },
  { container: "hls", mimeType: "application/vnd.apple.mpegurl" },
];

/**
 * Maps a `canPlayType` answer to a support level. Any answer other than `maybe` or `probably`
 * counts as unsupported, because some engines return non-standard values (e.g. `no`).
 */
export function toSupportLevel(answer: string): SupportLevel {
  switch (answer) {
    case "probably":
      return "probably";
    case "maybe":
      return "maybe";
    default:
      return "unsupported";
  }
}

/** Asks the probe about every candidate and returns one support entry per candidate, in order. */
export function detectContainerSupport(
  probe: MediaTypeProbe,
  candidates: readonly ContainerCandidate[] = defaultContainerCandidates,
): ContainerSupport[] {
  return candidates.map((candidate) => ({
    ...candidate,
    level: toSupportLevel(probe.canPlayType(candidate.mimeType)),
  }));
}
