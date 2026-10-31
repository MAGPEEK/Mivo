// The real hls.js behind the HlsFactory boundary. hls.js is loaded with a dynamic import, so it
// lands in its own chunk and only browsers that play an HLS session through MSE download it.
//
// hls.js runs its transmuxer in a Web Worker created from a blob URL (`enableWorker`). A
// Content-Security-Policy added later (BACKLOG) must therefore allow `worker-src blob:`.

import type { ErrorData, HlsConfig, LoaderConfig } from "hls.js";

import { retryAfterFromNetworkDetails } from "./retryAfter";
import type { HlsFactory, HlsStreamError } from "./hlsPlayer";

/** Seconds of media hls.js buffers ahead; its default, made explicit. */
export const maxBufferSeconds = 30;

/**
 * Upper bound for the forward buffer. hls.js grows it up to 600 s by default, which keeps a
 * transcode running far ahead of the viewer and fills memory on TVs.
 */
export const maxMaxBufferSeconds = 60;

/** How long the server holds the first playlist request before it answers 503 (ADR 0024). */
const serverPlaylistWaitMilliseconds = 20_000;

/** Time on top of the server's wait for its answer to arrive. */
const playlistAnswerMarginMilliseconds = 10_000;

// hls.js' defaults for the first playlist request, with two exceptions:
// - The load time: its default of 20 s equals the server's wait, so hls.js would time out
//   instead of receiving `stream_not_ready` with its Retry-After.
// - No retry after an error answer: hls.js would retry a 503 after its own delay and ignore
//   Retry-After. The engine paces those attempts (one server wait of up to 20 s each).
const manifestLoaderConfig: LoaderConfig = {
  maxTimeToFirstByteMs: Number.POSITIVE_INFINITY,
  maxLoadTimeMs: serverPlaylistWaitMilliseconds + playlistAnswerMarginMilliseconds,
  timeoutRetry: { maxNumRetry: 2, retryDelayMs: 0, maxRetryDelayMs: 0 },
  errorRetry: { maxNumRetry: 0, retryDelayMs: 0, maxRetryDelayMs: 0 },
};

/**
 * Where hls.js starts in the stream. The server's EVENT playlist has no ENDLIST until FFmpeg
 * finishes, so hls.js would treat it as live and start a few segments behind the newest one,
 * tens of seconds after the requested position when a remux runs faster than real time. The
 * stream's own clock starts at the session's start position (ADR 0024), so 0 is that position.
 */
export const streamStartSeconds = 0;

/** The hls.js settings of the web player; everything else keeps the hls.js defaults. */
export const hlsConfig: Partial<HlsConfig> = {
  enableWorker: true,
  maxBufferLength: maxBufferSeconds,
  maxMaxBufferLength: maxMaxBufferSeconds,
  manifestLoadPolicy: { default: manifestLoaderConfig },
  startPosition: streamStartSeconds,
};

/** The fields of an hls.js error the engine needs, with the request's `Retry-After` delay. */
export function toHlsStreamError(data: ErrorData): HlsStreamError {
  return {
    error: { type: data.type, details: data.details, fatal: data.fatal, response: data.response },
    retryAfterSeconds: retryAfterFromNetworkDetails(data.networkDetails),
  };
}

/** Creates hls.js instances with {@link hlsConfig}, loading hls.js on first use. */
export const createHlsJsPlayer: HlsFactory<HTMLMediaElement> = async (onError) => {
  const { default: Hls } = await import("hls.js");
  const hls = new Hls(hlsConfig);
  hls.on(Hls.Events.ERROR, (_event, data) => {
    onError(toHlsStreamError(data), hls);
  });
  return hls;
};
