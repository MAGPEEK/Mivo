import type { PlaybackErrorCode } from "./playbackErrors";
import type { PlaybackSnapshot } from "./playbackEngine";

/** Translation key (player namespace) of the status line below the video. */
export type PlaybackStatusKey =
  "status.loading" | "status.waitingForStream" | "status.playing" | `errors.${PlaybackErrorCode}`;

/** The status line for a playback state: the error, the wait for the stream, or the phase. */
export function playbackStatusKey(
  view: Pick<PlaybackSnapshot, "status" | "waitingForStream" | "error">,
): PlaybackStatusKey {
  if (view.status === "error") {
    return `errors.${view.error?.code ?? "generic"}`;
  }
  if (view.status === "playing") {
    return "status.playing";
  }
  return view.waitingForStream ? "status.waitingForStream" : "status.loading";
}
