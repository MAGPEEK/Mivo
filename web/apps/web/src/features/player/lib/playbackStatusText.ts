import type { PlaybackErrorCode } from "./playbackErrors";
import type { PlaybackSnapshot } from "./playbackEngine";

/** Translation key (player namespace) of the text while the stream is not playable yet. */
export type PlaybackLoadingKey = "status.loading" | "status.waitingForStream" | "status.switching";

/** Translation key (player namespace) of an error text. */
export type PlaybackErrorKey = `errors.${PlaybackErrorCode}`;

/** What the player shows over the video: nothing, a spinner with a text, or an error. */
export type PlaybackNotice =
  | { readonly kind: "none" }
  | { readonly kind: "loading"; readonly key: PlaybackLoadingKey }
  | { readonly kind: "error"; readonly key: PlaybackErrorKey; readonly retryable: boolean };

/**
 * Errors a retry cannot fix: the file and the browser do not fit (`no_compatible_format`), or
 * the server lacks the encoders and can only Direct Play and Remux (`transcode_unavailable`).
 */
const permanentErrors: ReadonlySet<PlaybackErrorCode> = new Set([
  "no_compatible_format",
  "transcode_unavailable",
]);

/**
 * The notice for a playback state: the error, the wait for the stream, the switch of a track
 * or the quality (PL-11), or nothing. The wait for the stream is more specific than a switch.
 */
export function playbackNotice(
  view: Pick<PlaybackSnapshot, "status" | "waitingForStream" | "error"> & {
    readonly switching?: boolean;
  },
): PlaybackNotice {
  if (view.status === "error") {
    const code = view.error?.code ?? "generic";
    return { kind: "error", key: `errors.${code}`, retryable: !permanentErrors.has(code) };
  }
  if (view.status === "playing") {
    return { kind: "none" };
  }
  if (view.waitingForStream) {
    return { kind: "loading", key: "status.waitingForStream" };
  }
  return { kind: "loading", key: view.switching === true ? "status.switching" : "status.loading" };
}
