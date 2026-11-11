// The tracks a player offers for one media file (ADR 0030 Notes M4-02a): audio tracks are the
// `audio` entries of `MediaFile.streams`, subtitle tracks come from `MediaFile.subtitleTracks`.
// The input types list only the fields read here, so the generated API types are assignable
// without player-core depending on the generated client.

import type { PlaybackSession, SubtitleDecision } from "../streamInfo/playbackSession";

/** The fields of the API's `MediaStream` that describe an audio track. */
export interface TrackSourceStream {
  readonly index: number;
  /** `MediaStreamKind`; an open string, later API versions may add kinds. */
  readonly kind: string;
  readonly codec?: string | undefined;
  readonly channels?: number | undefined;
  readonly language?: string | undefined;
  readonly title?: string | undefined;
  readonly default: boolean;
}

/** The API's `SubtitleTrack`. */
export interface TrackSourceSubtitle {
  readonly id: string;
  readonly language?: string | undefined;
  readonly title?: string | undefined;
  readonly format: string;
  readonly forced: boolean;
  readonly sdh: boolean;
  readonly default: boolean;
  readonly external: boolean;
  readonly textBased: boolean;
  readonly url?: string | undefined;
}

/** The fields of the API's `MediaFile` that tracks are read from. */
export interface TrackSource {
  readonly streams: readonly TrackSourceStream[];
  /** Absent from a server that does not list subtitle tracks yet. */
  readonly subtitleTracks?: readonly TrackSourceSubtitle[] | undefined;
}

/** An audio track of a file; `index` is what `PlaybackRequest.audioStreamIndex` takes. */
export interface AudioTrack {
  readonly index: number;
  readonly codec?: string | undefined;
  readonly channels?: number | undefined;
  readonly language?: string | undefined;
  readonly title?: string | undefined;
  readonly default: boolean;
  /** Commentary or audio description, which nobody wants by default. */
  readonly commentary: boolean;
}

/** A subtitle track of a file; `id` is what `PlaybackRequest.subtitleTrackId` takes. */
export interface SubtitleTrack extends TrackSourceSubtitle {
  /**
   * Whether the client shows the track itself from its WebVTT rendition (`url`). Any other
   * track needs burn-in, which restarts the stream as a transcode.
   */
  readonly sidecar: boolean;
}

const audioKind = "audio";

// Title words that mark commentary and audio description. Detection here is by title only:
// the API carries no disposition for them. The server also reads the `comment` and
// `visual_impaired` dispositions and uses the same words as its fallback
// (internal/domain/playback/tracks.go), so for a track flagged without a telling title its
// pick can differ from this preselection; what a session reports is the truth.
const commentaryTitleWords: readonly string[] = [
  "comment",
  "kommentar",
  "description",
  "descriptive",
  "audiodeskription",
  "hörfilm",
];

function isCommentaryTitle(title: string | undefined): boolean {
  const lower = (title ?? "").toLowerCase();
  return commentaryTitleWords.some((word) => lower.includes(word));
}

/** The audio tracks of a file in container order. */
export function audioTracks(file: TrackSource): AudioTrack[] {
  return file.streams
    .filter((stream) => stream.kind === audioKind)
    .map(({ index, codec, channels, language, title, default: isDefault }) => ({
      index,
      codec,
      channels,
      language,
      title,
      default: isDefault,
      commentary: isCommentaryTitle(title),
    }));
}

/** The subtitle tracks of a file, embedded ones first; empty when the server lists none. */
export function subtitleTracks(file: TrackSource): SubtitleTrack[] {
  return (file.subtitleTracks ?? []).map((track) => ({
    ...track,
    sidecar: track.textBased && track.url !== undefined && track.url !== "",
  }));
}

/** The part of a session that says which tracks it plays. */
export type TrackSession = Pick<PlaybackSession, "streams" | "subtitle">;

/** Index of the audio stream the session plays; `undefined` when it plays none. */
export function sessionAudioIndex(session: TrackSession): number | undefined {
  return session.streams.find((stream) => stream.kind === audioKind)?.sourceIndex;
}

/** What the session does with subtitles; a session that says nothing shows none. */
export function sessionSubtitle(session: TrackSession): SubtitleDecision {
  return session.subtitle ?? { mode: "off" };
}

/**
 * The track the client must render itself for this session: the one the server delivers as a
 * sidecar. `undefined` when subtitles are off, burned into the video, or the track is unknown
 * or has no WebVTT rendition.
 */
export function sessionSidecarTrack(
  session: TrackSession,
  tracks: readonly SubtitleTrack[],
): SubtitleTrack | undefined {
  const { mode, trackId } = sessionSubtitle(session);
  if (mode !== "sidecar" || trackId === undefined) {
    return undefined;
  }
  return tracks.find((track) => track.id === trackId && track.sidecar);
}
