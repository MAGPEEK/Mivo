// The stream-info overlay as a view model (ADR 0024, plan screen 4): mode, why that mode, what
// happens to each stream, container, encoder and source bit rate. It holds no sentences, only
// translation keys with params, technical labels and ui-tokens colour names, so the web app and
// the TV app render the same facts in their own way.

import { containerLabel } from "../labels/formatLabels";
import type { PlaybackSession } from "./playbackSession";
import { reasonTexts, warningTexts, type ReasonText } from "./reasonTexts";
import { encoderText, streamLine, type EncoderText, type StreamLine } from "./streamLines";
import {
  keyLookup,
  knownModes,
  type BitRateKey,
  type ModeKey,
  type ReasonKey,
  type WarningKey,
} from "./translationKeys";

/**
 * Name of a colour token in `@mivo/ui-tokens` (`colors`): the playback status colours, and the
 * muted text colour for a mode this version does not know. Only the name is returned; the app
 * resolves it to a colour.
 */
export type StreamInfoColorToken = "directPlay" | "directStream" | "transcode" | "textMuted";

/** The playback mode with its name key and status colour. */
export interface ModeInfo {
  /** Mode as the server sent it, e.g. `remux`. */
  readonly id: string;
  readonly key: ModeKey;
  readonly colorToken: StreamInfoColorToken;
}

/** Source and target container, e.g. MKV → MP4. */
export interface ContainerChange {
  /** Label of the source container, e.g. `MKV`. */
  readonly source: string;
  /** Label of the container the client receives, e.g. `MP4`. */
  readonly target: string;
  /** Whether the server packs the streams into another container. */
  readonly changed: boolean;
}

/** The source bit rate in the unit of its key; the app formats the number for the locale. */
export interface BitRateText {
  readonly key: BitRateKey;
  readonly params: { readonly value: number };
}

/** Everything the stream-info overlay shows about a session. */
export interface StreamInfo {
  readonly mode: ModeInfo;
  readonly reasons: readonly ReasonText<ReasonKey>[];
  readonly warnings: readonly ReasonText<WarningKey>[];
  readonly streams: readonly StreamLine[];
  readonly container: ContainerChange;
  /** Absent when every stream is copied. */
  readonly encoder: EncoderText | undefined;
  /** Absent when the source bit rate is unknown. */
  readonly sourceBitRate: BitRateText | undefined;
}

/** The parts of a session the overlay shows. */
export type StreamInfoSession = Pick<
  PlaybackSession,
  | "mode"
  | "sourceContainer"
  | "targetContainer"
  | "sourceBitRate"
  | "streams"
  | "reasons"
  | "warnings"
>;

// Remux is "Direct Stream" in the overlay and in the design system's status colours.
const modeColorTokens: ReadonlyMap<string, StreamInfoColorToken> = new Map([
  ["direct_play", "directPlay"],
  ["remux", "directStream"],
  ["transcode", "transcode"],
]);
const unknownModeColorToken: StreamInfoColorToken = "textMuted";

const modeKey = keyLookup("mode", knownModes);

const bitsPerKilobit = 1000;
const bitsPerMegabit = 1_000_000;
/** Megabits are shown with one decimal, e.g. 4.1 Mbit/s. */
const megabitDecimalFactor = 10;

/** Builds the stream-info view model of a session. */
export function streamInfo(session: StreamInfoSession): StreamInfo {
  return {
    mode: {
      id: session.mode,
      key: modeKey(session.mode),
      colorToken: modeColorTokens.get(session.mode) ?? unknownModeColorToken,
    },
    reasons: reasonTexts(session),
    warnings: warningTexts(session),
    streams: session.streams.map(streamLine),
    container: {
      source: containerLabel(session.sourceContainer),
      target: containerLabel(session.targetContainer),
      changed: session.sourceContainer !== session.targetContainer,
    },
    encoder: encoderText(session.streams),
    sourceBitRate: bitRateText(session.sourceBitRate),
  };
}

/**
 * The bit rate in kbit/s (whole numbers) below 1 Mbit/s, otherwise in Mbit/s with one decimal
 * (1 kbit/s = 1000 bit/s, as players and encoders count).
 * A missing, zero or unusable bit rate counts as unknown.
 */
export function bitRateText(bitsPerSecond: number | undefined): BitRateText | undefined {
  if (bitsPerSecond === undefined || !Number.isFinite(bitsPerSecond) || bitsPerSecond <= 0) {
    return undefined;
  }
  const kilobits = Math.round(bitsPerSecond / bitsPerKilobit);
  // Compared after rounding, so 999 600 bit/s reads 1 Mbit/s and never 1000 kbit/s.
  if (kilobits < bitsPerMegabit / bitsPerKilobit) {
    return { key: "streamInfo.bitRate.kilobits", params: { value: kilobits } };
  }
  return {
    key: "streamInfo.bitRate.megabits",
    params: {
      value:
        Math.round((bitsPerSecond / bitsPerMegabit) * megabitDecimalFactor) / megabitDecimalFactor,
    },
  };
}
