// What the player engine needs from the outside world, declared by the engine (the consumer):
// the playback API and the media element. The web app passes the API client adapter and a
// <video> element; tests pass fakes.

import type { ClientProfile, PlaybackSession, TimeRangesLike } from "@mivo/player-core";

/** Asks the server for a session (`POST /api/v1/playback/sessions`). */
export interface SessionRequest {
  readonly fileId: string;
  readonly profile: ClientProfile;
  /** File position to start at in seconds; 0 starts at the beginning. */
  readonly startPositionSeconds: number;
  /** Session the new one replaces after a seek or a recovery (ADR 0024). */
  readonly replacesSessionId: string | undefined;
}

/**
 * The playback endpoints. Failed requests reject with a `PlaybackRequestError`
 * (see playbackErrors.ts), so the engine can read status, code and `Retry-After`.
 */
export interface PlaybackApi {
  createSession(request: SessionRequest): Promise<PlaybackSession>;
  getSession(sessionId: string): Promise<PlaybackSession>;
  /**
   * Stops a session without waiting for the answer. The request must survive the page being
   * closed (`fetch` with `keepalive`), because it is sent on unmount and on `pagehide`.
   */
  stopSession(sessionId: string): void;
}

/** Media element events the engine listens to. */
export type PlaybackMediaEvent =
  "error" | "loadeddata" | "loadedmetadata" | "seeked" | "seeking" | "timeupdate";

/** `MediaError.code` of a fetch the user agent aborted; not a playback failure. */
export const mediaErrorAborted = 1;

/** The part of an `HTMLMediaElement` the engine uses; a `<video>` element satisfies it. */
export interface PlaybackMedia {
  src: string;
  currentTime: number;
  readonly paused: boolean;
  readonly seekable: TimeRangesLike;
  readonly buffered: TimeRangesLike;
  readonly error: { readonly code: number } | null;
  play(): Promise<void>;
  pause(): void;
  load(): void;
  removeAttribute(name: string): void;
  addEventListener(type: PlaybackMediaEvent, listener: () => void): void;
  removeEventListener(type: PlaybackMediaEvent, listener: () => void): void;
}
