// What a new session tells the viewer in a toast after a switch: the two consequences of the
// viewer's own choice that the stream-info overlay would otherwise hide (PL-10, PL-12).

import type { PlaybackReason, PlaybackSession } from "@mivo/player-core";

/** A message the player shows once; the text is looked up as `options.toast.<kind>`. */
export type SessionToastKind = "burnInUnavailable" | "bitRateCap";

/** The server could not burn the chosen subtitle in; the session plays without subtitles. */
const burnInUnavailableCode = "subtitle_burn_in_unavailable";

/** The chosen quality lies below the source, so the video is converted. */
const bitRateCapCode = "bitrate_cap";

function hasCode(reasons: readonly PlaybackReason[], code: string): boolean {
  return reasons.some((reason) => reason.code === code);
}

/**
 * The toasts a session asks for. The server reports `bitrate_cap` as a reason and
 * `subtitle_burn_in_unavailable` as a warning; either list is read for both, because the API
 * allows new codes in both and the viewer does not care which list a code sits in.
 */
export function sessionToasts(session: PlaybackSession): SessionToastKind[] {
  const reported = [...session.reasons, ...session.warnings];
  const toasts: SessionToastKind[] = [];
  if (hasCode(reported, burnInUnavailableCode)) {
    toasts.push("burnInUnavailable");
  }
  if (hasCode(reported, bitRateCapCode)) {
    toasts.push("bitRateCap");
  }
  return toasts;
}
