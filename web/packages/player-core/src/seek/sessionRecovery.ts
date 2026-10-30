// Recovery from a session the server no longer has (ADR 0024): a session that was idle for too
// long is stopped and its URLs answer 404, so the player creates a new session at its current
// position instead of showing an error.

/** HTTP status codes that mean the session's resources are gone. */
const goneStatuses: ReadonlySet<number> = new Set([404, 410]);

/** hls.js error type of failed network requests (`Hls.ErrorTypes.NETWORK_ERROR`). */
const hlsNetworkErrorType = "networkError";

/** hls.js error details of failed segment requests (`fragLoadError`, `fragLoadTimeOut`). */
const hlsSegmentErrorDetails: ReadonlySet<string> = new Set(["fragLoadError", "fragLoadTimeOut"]);

/**
 * The fields of an hls.js `ErrorData` that recovery needs. hls.js passes a structurally
 * compatible object to its `ERROR` event, so the app forwards it unchanged.
 */
export interface HlsErrorData {
  /** Error category, e.g. `networkError` or `mediaError`. */
  readonly type: string;
  /** Detail code, e.g. `fragLoadError` or `manifestLoadError`. */
  readonly details: string;
  /** Whether hls.js gave up; non-fatal errors are retried by hls.js itself. */
  readonly fatal: boolean;
  /** The HTTP response of a failed request, when there was one. */
  readonly response?: { readonly code?: number | undefined } | undefined;
}

/** An HTTP request of the player failed with `status`. */
export interface HttpPlaybackFailure {
  readonly source: "http";
  readonly status: number;
  /**
   * Whether the request addressed the current session (its resource or its stream URL). The
   * caller knows this; player-core never parses URLs, because stream URLs are opaque.
   */
  readonly concernsSession: boolean;
}

/** hls.js reported an error while playing the session's stream. */
export interface HlsPlaybackFailure {
  readonly source: "hls";
  readonly error: HlsErrorData;
}

/** A failure during playback that may be recovered by a new session. */
export type PlaybackFailure = HttpPlaybackFailure | HlsPlaybackFailure;

/**
 * Whether the player should create a new session at its current position: true for 404 or 410
 * on a request for the session, and for a fatal hls.js network error on a segment or on any
 * request of the stream that answered 404 or 410 (every hls.js request belongs to the session).
 * False otherwise; those failures are shown as errors.
 */
export function shouldRecreateSession(failure: PlaybackFailure): boolean {
  if (failure.source === "http") {
    return failure.concernsSession && goneStatuses.has(failure.status);
  }
  const { error } = failure;
  if (!error.fatal || error.type !== hlsNetworkErrorType) {
    return false;
  }
  const status = error.response?.code;
  return (
    hlsSegmentErrorDetails.has(error.details) || (status !== undefined && goneStatuses.has(status))
  );
}
