// Errors of the player engine, typed for the UI (task M2-27). The server sends machine-readable
// problem codes (ADR 0012); the engine reduces them to the few cases the player page explains
// and treats everything else, including codes of later server versions, as generic.

/** A failure the player page explains with its own text. */
export type PlaybackErrorCode =
  | "no_compatible_format"
  | "transcode_unavailable"
  | "session_limit_reached"
  | "stream_not_ready"
  | "generic";

/**
 * Why playback stopped. The page offers a retry, except for the codes a retry cannot fix
 * (`playbackNotice` in playbackStatusText.ts).
 */
export interface PlaybackError {
  readonly code: PlaybackErrorCode;
  /** Seconds the server asked the client to wait before retrying (`Retry-After`), when sent. */
  readonly retryAfterSeconds?: number | undefined;
}

/** Fields of a failed API request the engine needs to decide what to do. */
export interface RequestFailure {
  /** HTTP status, or 0 when no response arrived. */
  readonly status: number;
  /** Problem code of the server or of the API client (`network_error`, ...). */
  readonly code: string;
  /** Value of the response's `Retry-After` header in seconds, when present and valid. */
  readonly retryAfterSeconds: number | undefined;
}

/** A failed request of the playback API; thrown by the {@link PlaybackApi} port. */
export class PlaybackRequestError extends Error implements RequestFailure {
  override readonly name = "PlaybackRequestError";
  readonly status: number;
  readonly code: string;
  readonly retryAfterSeconds: number | undefined;

  constructor(failure: RequestFailure, options?: ErrorOptions) {
    super(`Playback request failed: ${failure.code} (HTTP ${String(failure.status)})`, options);
    this.status = failure.status;
    this.code = failure.code;
    this.retryAfterSeconds = failure.retryAfterSeconds;
  }
}

/** Server problem codes the page explains with a text of their own. */
const explainedCodes: ReadonlySet<string> = new Set<PlaybackErrorCode>([
  "no_compatible_format",
  "transcode_unavailable",
  "session_limit_reached",
  "stream_not_ready",
]);

function isExplainedCode(code: string): code is PlaybackErrorCode {
  return explainedCodes.has(code);
}

/** The generic error, for failures without a text of their own. */
export const genericPlaybackError: PlaybackError = { code: "generic" };

/**
 * Maps any failure of a playback request to a UI error. Known problem codes keep their code
 * and the `Retry-After` value; anything else is generic.
 */
export function toPlaybackError(error: unknown): PlaybackError {
  if (!(error instanceof PlaybackRequestError) || !isExplainedCode(error.code)) {
    return genericPlaybackError;
  }
  return error.retryAfterSeconds === undefined
    ? { code: error.code }
    : { code: error.code, retryAfterSeconds: error.retryAfterSeconds };
}
