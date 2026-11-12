import { parseWebVtt, type SubtitleCue } from "@mivo/player-core";
import { queryOptions } from "@tanstack/react-query";

/** Cache key of the cues of one subtitle track of one file. */
export function subtitleCuesQueryKey(fileId: string, trackId: string) {
  return ["player", "subtitleCues", fileId, trackId] as const;
}

/**
 * Loads the WebVTT rendition at `url` and parses it into cues. The URL is an opaque reference
 * the API hands out (`SubtitleTrack.url`); it is fetched as it is, with the session cookie, like
 * the stream URL, so the generated client has no operation to call for it.
 */
async function loadCues(url: string, signal: AbortSignal): Promise<SubtitleCue[]> {
  const response = await fetch(url, { credentials: "same-origin", signal });
  if (!response.ok) {
    throw new Error(`subtitle track answered ${String(response.status)}`);
  }
  return parseWebVtt(await response.text());
}

/**
 * Query options for the cues of the track at `url`. Cues never change for a file, so they stay
 * cached for the page's life; a failure leaves the video without text.
 */
export function subtitleCuesQueryOptions(fileId: string, trackId: string, url: string) {
  return queryOptions({
    queryKey: subtitleCuesQueryKey(fileId, trackId),
    queryFn: ({ signal }) => loadCues(url, signal),
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  });
}
