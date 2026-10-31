// The player engine (task M2-27, ADR 0023/0024): creates a playback session for the client
// profile, attaches its stream to the media element, seeks locally or by restarting the session,
// recovers once from a session the server lost and stops the session on teardown. Framework-free;
// the React hook usePlaybackSession only wires it to a <video> element.
//
// A restart empties the media element right away instead of playing the old stream until the
// new session arrives (decided in M2-30, ADR 0024 Notes M2-30): a restart means the target lies
// outside what the element has, so the old stream would keep playing the wrong part of the file,
// move the position away from the target the user just picked and keep its audio running, and
// its requests may fail once the server swaps the sessions. The player page shows the target
// position and a spinner instead.

import {
  clampPosition,
  displayedPosition,
  planSeek,
  readTimeRanges,
  restartPosition,
  shouldRecreateSession,
  type ClientProfile,
  type PlaybackSession,
  type PlaybackSessionState,
  type StartTimer,
  type TimelineSession,
} from "@mivo/player-core";

import {
  isMediaError,
  isStreamNotReady,
  type HlsFactory,
  type HlsPlayer,
  type HlsStreamError,
} from "./hlsPlayer";
import {
  genericPlaybackError,
  PlaybackRequestError,
  toPlaybackError,
  type PlaybackError,
} from "./playbackErrors";
import {
  mediaErrorAborted,
  type PlaybackApi,
  type PlaybackMedia,
  type PlaybackMediaEvent,
} from "./playbackPorts";
import { RecoveryBudget } from "./recoveryBudget";
import { StartQueue } from "./startQueue";

/** Seconds to wait for the stream when the server sent no usable `Retry-After` (ADR 0024). */
export const defaultStreamRetrySeconds = 2;

/** Longest wait between two attempts to load a stream that is not ready, in seconds. */
export const maxStreamRetrySeconds = 30;

/**
 * Attempts to load a stream that is not ready before giving up with `stream_not_ready`. Each
 * attempt waits up to 20 s on the server plus the `Retry-After` delay, so this allows about a
 * minute for a slow transcode start.
 */
export const maxStreamNotReadyRetries = 3;

const millisecondsPerSecond = 1_000;

/** The server did not produce the stream in time. */
const streamNotReadyError: PlaybackError = { code: "stream_not_ready" };

/** Phase of the playback: waiting for a playable stream, playable, or failed. */
export type PlaybackStatus = "loading" | "playing" | "error";

/** What the engine shows; replaced as a whole on every change. */
export interface PlaybackSnapshot {
  readonly status: PlaybackStatus;
  /** The session shown; during a restart still the session it replaces. */
  readonly session: PlaybackSession | undefined;
  /** Displayed file position in seconds (player-core timeline). */
  readonly position: number;
  /** Whether the server is still preparing the stream (`stream_not_ready`). */
  readonly waitingForStream: boolean;
  readonly error: PlaybackError | undefined;
  /**
   * Whether the stream starts playing once it is attached. While no stream is attached (a
   * restart, a wait for the stream) this is the only play state there is.
   */
  readonly autoplay: boolean;
}

/** Services the engine uses. */
export interface PlaybackEngineDependencies<Media extends PlaybackMedia> {
  readonly api: PlaybackApi;
  readonly createHls: HlsFactory<Media>;
  readonly startTimer: StartTimer;
}

/** What to play, and where. */
export interface PlaybackOpenRequest<Media extends PlaybackMedia> {
  readonly media: Media;
  readonly profile: ClientProfile;
  readonly fileId: string;
  /** File position to start at in seconds, already clamped to the file. */
  readonly startPositionSeconds: number;
}

/** A wish for a session: which file, where, and whether to play right away. */
interface StartOptions<Media extends PlaybackMedia> {
  readonly request: PlaybackOpenRequest<Media>;
  readonly position: number;
  readonly autoplay: boolean;
}

/** How the current stream is attached to which media element. */
interface Attachment<Media> {
  readonly kind: "file" | "native" | "mse";
  readonly media: Media;
}

const mediaEvents: readonly PlaybackMediaEvent[] = [
  "error",
  "loadeddata",
  "loadedmetadata",
  "seeked",
  "seeking",
  "timeupdate",
];

/** Bounds positions before any session told the duration: inside `[0, ∞)`. */
const fileOfUnknownLength: TimelineSession = { delivery: "file", startPositionSeconds: 0 };

/** The snapshot before anything was opened and after close. */
export const initialPlaybackSnapshot: PlaybackSnapshot = {
  status: "loading",
  session: undefined,
  position: 0,
  waitingForStream: false,
  error: undefined,
  autoplay: true,
};

/** Plays one file at a time on one media element; see the file comment. */
export class PlaybackEngine<Media extends PlaybackMedia> {
  private readonly dependencies: PlaybackEngineDependencies<Media>;
  private readonly listeners = new Set<() => void>();
  /** Recreating a lost session, once per position. */
  private readonly recovery = new RecoveryBudget();
  /** hls.js `recoverMediaError` after a fatal media error, once per position. */
  private readonly mediaRecovery = new RecoveryBudget();
  private readonly starts = new StartQueue<StartOptions<Media>>();
  private snapshot: PlaybackSnapshot = initialPlaybackSnapshot;
  private request: PlaybackOpenRequest<Media> | undefined;
  /** Incremented whenever the current attempt is abandoned; late answers compare it. */
  private generation = 0;
  /** The newest session the server created for this player; the next request replaces it. */
  private session: PlaybackSession | undefined;
  /**
   * The open request `session` was created for. After `open` during a pending request,
   * `session` may belong to the previous file: it lends its slot but not its duration.
   */
  private sessionRequest: PlaybackOpenRequest<Media> | undefined;
  private attachment: Attachment<Media> | undefined;
  private hls: HlsPlayer<Media> | undefined;
  private cancelTimer: (() => void) | undefined;
  private autoplay = true;
  private pendingFileStart = 0;
  private streamNotReadyRetries = 0;
  private suspended = false;

  constructor(dependencies: PlaybackEngineDependencies<Media>) {
    this.dependencies = dependencies;
  }

  /** Registers a listener for snapshot changes; returns the function that removes it. */
  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  /** The current snapshot; the same object until something changes. */
  readonly getSnapshot = (): PlaybackSnapshot => this.snapshot;

  /** Starts playing `request.fileId`, closing whatever played before. */
  open(request: PlaybackOpenRequest<Media>): void {
    this.close();
    this.request = request;
    for (const event of mediaEvents) {
      request.media.addEventListener(event, this.mediaListeners[event]);
    }
    this.resetRecovery();
    this.start({ request, position: request.startPositionSeconds, autoplay: true });
  }

  /** Stops playback: hls.js destroyed, media element emptied, session stopped on the server. */
  close(): void {
    const request = this.request;
    if (request === undefined) {
      return;
    }
    this.teardown();
    for (const event of mediaEvents) {
      request.media.removeEventListener(event, this.mediaListeners[event]);
    }
    this.request = undefined;
    this.suspended = false;
    this.publish(initialPlaybackSnapshot);
  }

  /**
   * Moves to the file position `target`: locally when the element already has it, otherwise
   * with a new session that replaces the current one. Before a stream plays, every seek
   * restarts at the target, except in a direct-play file that is already attached; a seek
   * during a pending request is applied when it settles.
   */
  seek(target: number): void {
    const { request, session, attachment } = this;
    if (request === undefined || this.suspended) {
      return;
    }
    this.resetRecovery();
    const { media } = request;
    // A direct-play file seeks in the element as soon as it is attached, unless the element
    // has failed already: then only a new session helps.
    const loadingFile =
      session?.delivery === "file" && attachment !== undefined && media.error === null;
    if (session === undefined || (this.snapshot.status !== "playing" && !loadingFile)) {
      this.restart(request, target, this.autoplay);
      return;
    }
    const plan = planSeek(target, {
      session,
      seekable: readTimeRanges(media.seekable),
      buffered: readTimeRanges(media.buffered),
    });
    if (plan.kind === "local") {
      this.seekLocally(media, plan.time);
      return;
    }
    this.restart(request, plan.position, !media.paused);
  }

  /** Sets the element's clock, or the start of a direct-play file still loading its metadata. */
  private seekLocally(media: Media, time: number): void {
    if (this.pendingFileStart > 0) {
      // A direct-play file's clock is the file position, so `time` is the position, too.
      this.pendingFileStart = time;
      this.publish({ ...this.snapshot, position: time });
      return;
    }
    media.currentTime = time;
    this.updatePosition();
  }

  /**
   * Starts again at the current position after an error, e.g. `session_limit_reached`. It
   * keeps the intent: playing, unless the user paused (ADR 0024 Notes M2-41).
   */
  retry(): void {
    const { request } = this;
    if (request === undefined || this.suspended) {
      return;
    }
    this.resetRecovery();
    this.restart(request, this.snapshot.position, this.autoplay);
  }

  /**
   * Sets whether playback should run. Before a stream is attached, this decides whether it
   * starts playing once it is; an attached stream plays or pauses right away.
   *
   * Only the user's play and pause set the intent; the engine does not follow the element's own
   * `pause` events (the end of the file, a refused autoplay), so a recovery after those still
   * plays as the user last asked.
   */
  setAutoplay(autoplay: boolean): void {
    if (this.request === undefined) {
      return;
    }
    this.autoplay = autoplay;
    const media = this.attachment?.media;
    if (media !== undefined && autoplay) {
      this.playIfWanted(media);
    } else {
      media?.pause();
    }
    this.publish({ ...this.snapshot, autoplay });
  }

  /** Stops the session while the page is hidden (`pagehide`); {@link resume} restarts it. */
  suspend(): void {
    if (this.request === undefined || this.suspended) {
      return;
    }
    this.teardown();
    this.suspended = true;
  }

  /** Restarts at the last position after {@link suspend}, e.g. when the page is restored. */
  resume(): void {
    const { request } = this;
    if (request === undefined || !this.suspended) {
      return;
    }
    this.suspended = false;
    this.start({ request, position: this.snapshot.position, autoplay: false });
  }

  private resetRecovery(): void {
    this.recovery.reset();
    this.mediaRecovery.reset();
  }

  /** Starts a session that replaces the current one, inside the file (`restartPosition`). */
  private restart(request: PlaybackOpenRequest<Media>, position: number, autoplay: boolean): void {
    const ownSession = this.sessionRequest === request ? this.session : undefined;
    const bounds = ownSession ?? fileOfUnknownLength;
    this.start({ request, position: restartPosition(bounds, position), autoplay });
  }

  private start(options: StartOptions<Media>): void {
    this.abandonAttempt();
    this.autoplay = options.autoplay;
    this.streamNotReadyRetries = 0;
    this.publish({
      ...this.snapshot,
      status: "loading",
      position: options.position,
      waitingForStream: false,
      error: undefined,
      autoplay: options.autoplay,
    });
    this.dispatch(options);
  }

  /** Sends the session request for `options`, or keeps it until the one in flight settles. */
  private dispatch(options: StartOptions<Media>): void {
    if (!this.starts.tryBegin(options)) {
      return;
    }
    const generation = this.generation;
    const { request } = options;
    this.dependencies.api
      .createSession({
        fileId: request.fileId,
        profile: request.profile,
        startPositionSeconds: options.position,
        replacesSessionId: this.session?.id,
      })
      .then(
        (session) => {
          this.onSessionCreated(generation, options, session);
        },
        (error: unknown) => {
          this.onCreateFailed(generation, error);
        },
      );
  }

  private onSessionCreated(
    generation: number,
    options: StartOptions<Media>,
    session: PlaybackSession,
  ): void {
    const next = this.starts.finish();
    if (next !== undefined) {
      this.continueAfter(session, options, next);
    } else if (generation === this.generation) {
      this.useSession(options.request, session, session.startPositionSeconds);
    } else {
      // Closed or hidden while the request was in flight.
      this.dependencies.api.stopSession(session.id);
    }
  }

  /**
   * A newer wish waited while `session` was being created. It replaces `session`, so one
   * server slot serves the whole series of seeks. A direct-play file needs no new session: the
   * element seeks in it.
   *
   * When the newer wish opens another file (`open` while the request was in flight), `session`
   * belongs to the previous file: it only lends its slot, is never shown, and its duration says
   * nothing about the new file, whose start `open` has already clamped.
   *
   * When the newer wish wants the position `session` was created for (the page was hidden and
   * shown again meanwhile), `session` already is what it asks for and is used as it is.
   */
  private continueAfter(
    session: PlaybackSession,
    finished: StartOptions<Media>,
    next: StartOptions<Media>,
  ): void {
    this.session = session;
    this.sessionRequest = finished.request;
    if (next.request !== finished.request) {
      this.dispatch(next);
      return;
    }
    const position = restartPosition(session, next.position);
    if (session.delivery === "file") {
      this.useSession(next.request, session, position);
      return;
    }
    if (next.position === finished.position) {
      this.useSession(next.request, session, session.startPositionSeconds);
      return;
    }
    // The session shown is replaced already; show the one that exists until the next arrives.
    this.publish({ ...this.snapshot, session });
    this.dispatch({ ...next, position });
  }

  private onCreateFailed(generation: number, error: unknown): void {
    const next = this.starts.finish();
    if (next !== undefined) {
      // The newest wish still counts; it replaces the last session that exists.
      this.dispatch(next);
    } else if (generation === this.generation) {
      this.fail(toPlaybackError(error));
    }
  }

  /** Shows `session` and attaches its stream, starting at the file position `startPosition`. */
  private useSession(
    request: PlaybackOpenRequest<Media>,
    session: PlaybackSession,
    startPosition: number,
  ): void {
    this.session = session;
    this.sessionRequest = request;
    this.pendingFileStart = session.delivery === "file" ? startPosition : 0;
    const position = clampPosition(session, startPosition);
    this.publish({ ...this.snapshot, session, position });
    this.attach(this.generation, request, session);
  }

  /** Loads the session's stream into the media element; refuses deliveries it does not know. */
  private attach(
    generation: number,
    request: PlaybackOpenRequest<Media>,
    session: PlaybackSession,
  ): void {
    const { media, profile } = request;
    // Widened on purpose: a newer server may send a delivery this version does not know.
    const delivery: string = session.delivery;
    if (delivery === "file") {
      this.attachSource({ kind: "file", media }, session.streamUrl);
    } else if (delivery === "hls" && profile.hls.delivery === "mse") {
      void this.attachHls(generation, request, session.streamUrl);
    } else if (delivery === "hls" && profile.hls.delivery === "native") {
      this.attachSource({ kind: "native", media }, session.streamUrl);
    } else {
      this.fail(genericPlaybackError);
    }
  }

  private attachSource(attachment: Attachment<Media>, streamUrl: string): void {
    this.attachment = attachment;
    attachment.media.src = streamUrl;
    this.playIfWanted(attachment.media);
  }

  private async attachHls(
    generation: number,
    request: PlaybackOpenRequest<Media>,
    streamUrl: string,
  ): Promise<void> {
    let hls: HlsPlayer<Media>;
    try {
      hls = await this.dependencies.createHls((error, player) => {
        this.onHlsError(generation, request, error, player);
      });
    } catch {
      // hls.js could not be loaded, e.g. its chunk is missing after a server update.
      if (generation === this.generation) {
        this.fail(genericPlaybackError);
      }
      return;
    }
    if (generation !== this.generation) {
      hls.destroy();
      return;
    }
    const { media } = request;
    this.hls = hls;
    this.attachment = { kind: "mse", media };
    hls.attachMedia(media);
    hls.loadSource(streamUrl);
    this.playIfWanted(media);
  }

  private playIfWanted(media: Media): void {
    if (this.autoplay) {
      // A browser may refuse to start without a user gesture; the element then stays paused
      // and the page offers its play button. jsdom's play() returns nothing, hence the wrapper.
      Promise.resolve()
        .then(() => media.play())
        .catch(() => undefined);
    }
  }

  /** Destroys hls.js and empties the media element; the session stays on the server. */
  private detach(): void {
    const attachment = this.attachment;
    this.attachment = undefined;
    // hls.js empties the element itself when it is destroyed.
    if (this.hls !== undefined) {
      this.hls.destroy();
      this.hls = undefined;
    } else if (attachment !== undefined) {
      attachment.media.removeAttribute("src");
      attachment.media.load();
    }
  }

  /** Ends the current attempt: late answers are ignored, timers cancelled, hls.js destroyed. */
  private abandonAttempt(): void {
    this.generation += 1;
    this.cancelTimer?.();
    this.cancelTimer = undefined;
    this.detach();
  }

  private teardown(): void {
    this.abandonAttempt();
    this.starts.clear();
    if (this.session !== undefined) {
      this.dependencies.api.stopSession(this.session.id);
      this.session = undefined;
    }
    this.sessionRequest = undefined;
  }

  /** Shows `error`; late answers and hls.js errors of the failed attempt are ignored. */
  private fail(error: PlaybackError): void {
    this.abandonAttempt();
    this.publish({ ...this.snapshot, status: "error", waitingForStream: false, error });
  }

  private onHlsError(
    generation: number,
    request: PlaybackOpenRequest<Media>,
    { error, retryAfterSeconds }: HlsStreamError,
    player: HlsPlayer<Media>,
  ): void {
    if (generation !== this.generation || !error.fatal) {
      return;
    }
    if (isStreamNotReady(error)) {
      this.retryWhenReady(generation, request, retryAfterSeconds, streamNotReadyError);
    } else if (shouldRecreateSession({ source: "hls", error })) {
      this.recover(request);
    } else if (isMediaError(error) && this.mediaRecovery.tryRecover(this.snapshot.position)) {
      player.recoverMediaError();
    } else {
      this.fail(genericPlaybackError);
    }
  }

  /**
   * A media element reports no HTTP status. The session resource tells whether the session is
   * gone (recover), still preparing its stream (wait) or broken.
   */
  private async onMediaError(): Promise<void> {
    const { session, request, attachment } = this;
    if (session === undefined || request === undefined || attachment === undefined) {
      return;
    }
    const aborted = attachment.media.error?.code === mediaErrorAborted;
    if (attachment.kind === "mse" || aborted) {
      return;
    }
    const generation = this.generation;
    let current: PlaybackSession;
    try {
      current = await this.dependencies.api.getSession(session.id);
    } catch (error) {
      if (generation === this.generation) {
        this.onSessionLookupFailure(request, error);
      }
      return;
    }
    if (generation !== this.generation) {
      return;
    }
    const errorAfterWaiting = this.streamWaitOutcome(current.state, attachment.kind);
    if (errorAfterWaiting === undefined) {
      this.fail(genericPlaybackError);
    } else {
      this.retryWhenReady(generation, request, undefined, errorAfterWaiting);
    }
  }

  /**
   * Whether a media element error means "not ready yet", and what to show when waiting runs
   * out; `undefined` means it is a real failure. A session that is still starting is waited
   * for (`stream_not_ready`). Native HLS may also have got the playlist's 503 just before the
   * first segment appeared: the session is `ready` by now, but the element never played. That
   * is waited for, too, but when it keeps failing the server is not the one still preparing,
   * so it ends as a generic error.
   */
  private streamWaitOutcome(
    state: PlaybackSessionState,
    kind: Attachment<Media>["kind"],
  ): PlaybackError | undefined {
    if (state === "starting") {
      return streamNotReadyError;
    }
    const nativeBeforeFirstFrame = kind === "native" && this.snapshot.status === "loading";
    return state === "ready" && nativeBeforeFirstFrame ? genericPlaybackError : undefined;
  }

  private onSessionLookupFailure(request: PlaybackOpenRequest<Media>, error: unknown): void {
    const status = error instanceof PlaybackRequestError ? error.status : 0;
    if (shouldRecreateSession({ source: "http", status, concernsSession: true })) {
      this.recover(request);
    } else {
      this.fail(toPlaybackError(error));
    }
  }

  /** Creates a new session at the current position, once per position (RecoveryBudget). */
  private recover(request: PlaybackOpenRequest<Media>): void {
    const { position } = this.snapshot;
    if (!this.recovery.tryRecover(position)) {
      this.fail(genericPlaybackError);
      return;
    }
    this.restart(request, position, this.autoplay);
  }

  /**
   * Loads the stream of the same session again after the server's `Retry-After`, or shows
   * `errorAfterWaiting` once {@link maxStreamNotReadyRetries} attempts are used up.
   */
  private retryWhenReady(
    generation: number,
    request: PlaybackOpenRequest<Media>,
    retryAfterSeconds: number | undefined,
    errorAfterWaiting: PlaybackError,
  ): void {
    const session = this.session;
    if (session === undefined || this.streamNotReadyRetries >= maxStreamNotReadyRetries) {
      this.fail(errorAfterWaiting);
      return;
    }
    this.streamNotReadyRetries += 1;
    this.detach();
    this.publish({ ...this.snapshot, status: "loading", waitingForStream: true });
    const delaySeconds = Math.min(
      retryAfterSeconds ?? defaultStreamRetrySeconds,
      maxStreamRetrySeconds,
    );
    this.cancelTimer = this.dependencies.startTimer(delaySeconds * millisecondsPerSecond, () => {
      this.cancelTimer = undefined;
      this.attach(generation, request, session);
    });
  }

  private updatePosition(): void {
    const { session, attachment } = this;
    // Until a direct-play file has jumped to its start, its clock still reads 0.
    if (session !== undefined && attachment !== undefined && this.pendingFileStart === 0) {
      const position = displayedPosition(session, attachment.media.currentTime);
      if (position !== this.snapshot.position) {
        this.publish({ ...this.snapshot, position });
      }
    }
  }

  private onLoadedData(): void {
    if (this.attachment !== undefined && this.snapshot.status === "loading") {
      this.publish({ ...this.snapshot, status: "playing", waitingForStream: false });
    }
  }

  /** A direct-play file starts at its beginning; the session's start position is a seek. */
  private onLoadedMetadata(): void {
    const { attachment } = this;
    if (attachment?.kind === "file" && this.pendingFileStart > 0) {
      attachment.media.currentTime = this.pendingFileStart;
      this.pendingFileStart = 0;
    }
  }

  private readonly mediaListeners: Readonly<Record<PlaybackMediaEvent, () => void>> = {
    error: () => {
      void this.onMediaError();
    },
    loadeddata: () => {
      this.onLoadedData();
    },
    loadedmetadata: () => {
      this.onLoadedMetadata();
    },
    seeked: () => {
      this.updatePosition();
    },
    seeking: () => {
      this.updatePosition();
    },
    timeupdate: () => {
      this.updatePosition();
    },
  };

  private publish(snapshot: PlaybackSnapshot): void {
    this.snapshot = snapshot;
    for (const listener of this.listeners) {
      listener();
    }
  }
}
