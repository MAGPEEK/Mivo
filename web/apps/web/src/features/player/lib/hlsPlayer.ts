// The boundary between the engine and hls.js. The engine only sees these small interfaces, so
// tests inject a fake factory (jsdom has no Media Source Extensions) and the real hls.js stays
// in its own lazily loaded chunk (see hlsJs.ts).

import type { HlsErrorData } from "@mivo/player-core";

/** An error hls.js reported, with the `Retry-After` delay of the failed request, if any. */
export interface HlsStreamError {
  readonly error: HlsErrorData;
  readonly retryAfterSeconds: number | undefined;
}

/** The part of an hls.js instance the engine uses. */
export interface HlsPlayer<Media> {
  attachMedia(media: Media): void;
  loadSource(url: string): void;
  /** Resets the media pipeline after a fatal decoding or buffering error (hls.js API). */
  recoverMediaError(): void;
  destroy(): void;
}

/** Receives every error an hls.js instance reports, fatal or not, with the reporting instance. */
export type HlsErrorListener<Media> = (error: HlsStreamError, player: HlsPlayer<Media>) => void;

/** Creates an hls.js instance that reports its errors to `onError`. */
export type HlsFactory<Media> = (onError: HlsErrorListener<Media>) => Promise<HlsPlayer<Media>>;

/** hls.js detail of a failed first playlist request (`ErrorDetails.MANIFEST_LOAD_ERROR`). */
export const manifestLoadErrorDetails = "manifestLoadError";

/** hls.js type of decoding and buffering errors (`ErrorTypes.MEDIA_ERROR`). */
export const mediaErrorType = "mediaError";

/** The server answers the playlist with 503 `stream_not_ready` until the first segment exists. */
const streamNotReadyStatus = 503;

/**
 * Whether hls.js gave up on the playlist because the stream has no segment yet. The server
 * answers 503 `stream_not_ready` with `Retry-After` (ADR 0024); hls.js cannot read the problem
 * code, so any 503 on the playlist counts.
 */
export function isStreamNotReady(error: HlsErrorData): boolean {
  return (
    error.fatal &&
    error.details === manifestLoadErrorDetails &&
    error.response?.code === streamNotReadyStatus
  );
}

/** Whether hls.js gave up decoding or buffering; `recoverMediaError` may fix that. */
export function isMediaError(error: HlsErrorData): boolean {
  return error.fatal && error.type === mediaErrorType;
}
