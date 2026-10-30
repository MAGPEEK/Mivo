// A playback session as the server returns it (ADR 0024). These types are structurally
// identical to the API schema `PlaybackSession` in api/openapi.yaml, written out here because
// player-core must not depend on the generated client; the web app checks assignability at
// compile time. Arrays stay mutable for the same reason as in `ClientProfile`.
//
// The unions list the values this version knows. Servers of the same API major version may send
// more, so every consumer in player-core falls back to a generic result for unknown values.

/** How the file is played, from cheapest to most expensive. */
export type PlaybackMode = "direct_play" | "remux" | "transcode";

/** How the client loads `streamUrl`: a progressive file or an HLS playlist. */
export type PlaybackDelivery = "file" | "hls";

/** Lifecycle of a session; direct-play sessions start as `ready`. */
export type PlaybackSessionState = "starting" | "ready" | "ended" | "failed";

/** What happens to one source stream: passed through unchanged or re-encoded. */
export type StreamAction = "copy" | "transcode";

/** Kind of a stream inside a media file. */
export type MediaStreamKind = "video" | "audio" | "subtitle" | "other";

/** Part of the file a reason or warning concerns. */
export type PlaybackReasonScope = "container" | "video" | "audio" | "subtitle";

/** What the session does with one selected source stream. */
export interface StreamDecision {
  kind: MediaStreamKind;
  /** Index of the source stream in its container. */
  sourceIndex: number;
  action: StreamAction;
  /** ffprobe codec name of the source stream. */
  sourceCodec: string;
  /** Codec the client receives; equals `sourceCodec` when copied. */
  targetCodec: string;
  /** Width of a video stream the client receives, in pixels. */
  targetWidth?: number;
  /** Height of a video stream the client receives, in pixels. */
  targetHeight?: number;
  /** Channels of an audio stream the client receives. */
  targetChannels?: number;
  /** FFmpeg encoder of a transcoded stream, e.g. `libx264`; absent when copied. */
  encoder?: string;
}

/** A machine-readable reason or warning code with the part of the file it concerns. */
export interface PlaybackReason {
  code: string;
  scope: PlaybackReasonScope;
}

/** A running or finished playback of one media file for one client. */
export interface PlaybackSession {
  id: string;
  fileId: string;
  state: PlaybackSessionState;
  /** Why the session failed; only present when `state` is `failed`. */
  failureCode?: string;
  mode: PlaybackMode;
  delivery: PlaybackDelivery;
  /** Opaque URI reference to load; never parsed or built by the client. */
  streamUrl: string;
  /** Position in the file where the stream actually starts, in seconds. */
  startPositionSeconds: number;
  /** Probed duration of the whole file in seconds; absent when unknown. */
  durationSeconds?: number;
  /** Container short name of the source file, e.g. `mkv`. */
  sourceContainer: string;
  /** Container the client receives: the source container for direct play, `mp4` for HLS. */
  targetContainer: string;
  /** Overall bit rate of the source file in bits per second; absent when unknown. */
  sourceBitRate?: number;
  /** One decision per selected source stream, video first. */
  streams: StreamDecision[];
  /** Why a cheaper mode was not possible; empty for direct play. */
  reasons: PlaybackReason[];
  /** Degradations of the result, e.g. a downscaled video. */
  warnings: PlaybackReason[];
}
