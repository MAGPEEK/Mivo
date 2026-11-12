// The rows of the playback options panel (PL-10 … PL-13) as plain data: what each choice is
// called, what it costs and whether it is the one playing now. The panel only renders them.

import {
  channelLabel,
  codecLabel,
  originalQualityBitRate,
  sessionAudioIndex,
  sessionSubtitle,
  subtitleFormatLabel,
  type AudioTrack,
  type PlaybackSession,
  type QualityOption,
  type SubtitleTrack,
  type TrackSelection,
} from "@mivo/player-core";

import { isChosen, type QualityChoice } from "./qualityPreference";

const detailSeparator = " · ";
const bitsPerMegabit = 1_000_000;

// SubStation formats carry styling the player does not draw yet (task M4-19); say so.
const styledFormats: ReadonlySet<string> = new Set(["ass", "ssa"]);

/** One audio track of the panel. */
export interface AudioRow {
  /** `MediaStream.index`, what the server takes as `audioStreamIndex`. */
  readonly index: number;
  readonly language: string | undefined;
  readonly title: string | undefined;
  /** Codec and channels, e.g. `E-AC-3 · 5.1`; empty when neither is known. */
  readonly details: string;
  /** The browser cannot play the codec, so the server converts the audio to AAC. */
  readonly converted: boolean;
  readonly current: boolean;
}

/** The index of the audio track that plays: the viewer's choice, else the session's. */
export function currentAudioIndex(
  selection: TrackSelection,
  session: PlaybackSession | undefined,
): number | undefined {
  return (
    selection.audioStreamIndex ?? (session === undefined ? undefined : sessionAudioIndex(session))
  );
}

/**
 * The audio rows in file order. `supportedCodecs` are the audio codecs the client plays in an
 * HLS stream (the profile's); any other track is converted.
 */
export function audioRows(
  tracks: readonly AudioTrack[],
  currentIndex: number | undefined,
  supportedCodecs: readonly string[],
): AudioRow[] {
  return tracks.map((track) => ({
    index: track.index,
    language: track.language,
    title: track.title,
    details: [
      track.codec === undefined ? undefined : codecLabel(track.codec),
      track.channels === undefined ? undefined : channelLabel(track.channels),
    ]
      .filter((part) => part !== undefined)
      .join(detailSeparator),
    converted: track.codec !== undefined && !supportedCodecs.includes(track.codec),
    current: track.index === currentIndex,
  }));
}

/** One subtitle track of the panel. */
export interface SubtitleRow {
  readonly id: string;
  readonly language: string | undefined;
  readonly title: string | undefined;
  readonly format: string;
  readonly forced: boolean;
  readonly sdh: boolean;
  readonly external: boolean;
  /** An image or otherwise unrenderable track: the server burns it into the video. */
  readonly burnIn: boolean;
  /** Drawn as plain text; its styling is not supported yet. */
  readonly unstyled: boolean;
  readonly current: boolean;
  /** The server could not burn this track in when it was chosen. */
  readonly refused: boolean;
}

/** The subtitle rows in file order; `refusedId` is the track the server could not burn in. */
export function subtitleRows(
  tracks: readonly SubtitleTrack[],
  shownId: string,
  refusedId: string | undefined,
): SubtitleRow[] {
  return tracks.map((track) => ({
    id: track.id,
    language: track.language,
    title: track.title,
    format: subtitleFormatLabel(track.format),
    forced: track.forced,
    sdh: track.sdh,
    external: track.external,
    burnIn: !track.sidecar,
    unstyled: track.sidecar && styledFormats.has(track.format),
    current: track.id === shownId,
    refused: track.id === refusedId,
  }));
}

/** Whether the shown subtitle is burned into the video, so its size cannot be changed. */
export function subtitleIsBurnedIn(session: PlaybackSession | undefined): boolean {
  return session !== undefined && sessionSubtitle(session).mode === "burn_in";
}

/** One quality of the panel. */
export interface QualityRow {
  readonly kind: QualityOption["kind"] | "automatic";
  /** The limit in bits per second; {@link originalQualityBitRate} for original and Automatic. */
  readonly maxBitRate: number;
  /** Video height the step is made for; `undefined` for original and Automatic. */
  readonly height: number | undefined;
  /** The limit in Mbit/s, whole numbers; `undefined` for original and Automatic. */
  readonly megabits: number | undefined;
  readonly current: boolean;
}

/**
 * The quality rows: the original, Automatic when HLS can measure the speed, then the steps
 * below the source, best first. Steps above the source are not in `options`.
 */
export function qualityRows(
  options: readonly QualityOption[],
  choice: QualityChoice,
  automaticAvailable: boolean,
): QualityRow[] {
  const rows = options.map((option): QualityRow => ({
    kind: option.kind,
    maxBitRate: option.maxBitRate,
    height: option.kind === "limited" ? option.height : undefined,
    megabits:
      option.kind === "limited" ? Math.round(option.maxBitRate / bitsPerMegabit) : undefined,
    current: isChosen(choice, option),
  }));
  if (!automaticAvailable) {
    return rows;
  }
  const automatic: QualityRow = {
    kind: "automatic",
    maxBitRate: originalQualityBitRate,
    height: undefined,
    megabits: undefined,
    current: choice.kind === "automatic",
  };
  return [...rows.slice(0, 1), automatic, ...rows.slice(1)];
}

/**
 * The rows shown before "Show all steps": original, Automatic and a chosen step. Everything
 * when `showAll` is set.
 */
export function visibleQualityRows(rows: readonly QualityRow[], showAll: boolean): QualityRow[] {
  return rows.filter((row) => showAll || row.kind !== "limited" || row.current);
}

/** The step Automatic plays at now: `undefined` while it plays the original. */
export function activeStep(
  rows: readonly QualityRow[],
  selection: TrackSelection,
): QualityRow | undefined {
  const maxBitRate = selection.maxBitRate ?? originalQualityBitRate;
  return rows.find((row) => row.kind === "limited" && row.maxBitRate === maxBitRate);
}
