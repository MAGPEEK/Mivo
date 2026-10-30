// Translation keys of the stream-info overlay: the contract between player-core and the
// locale files of the app's player feature (M2-30). Keys are relative to the namespace the app
// translates them in, nested under `streamInfo`. Every known code has its own key; codes added
// by a newer server map to the group's `unknown` key, so the overlay always shows some text.

import type { MediaStreamKind, PlaybackMode, StreamAction } from "./playbackSession";

/** Name of the fallback key in every group whose values come from the server. */
export const unknownKeyName = "unknown";
type Unknown = typeof unknownKeyName;

/** Modes of the API, in order of cost. */
export const knownModes = [
  "direct_play",
  "remux",
  "transcode",
] as const satisfies readonly PlaybackMode[];

/** Reason codes of ADR 0023: why a cheaper mode was not possible. */
export const knownReasonCodes = [
  "container_not_supported",
  "video_codec_not_supported",
  "video_profile_not_supported",
  "video_bit_depth_not_supported",
  "audio_codec_not_supported",
  "audio_track_not_default",
] as const;

/** Warning codes of ADR 0023: degradations of the result. */
export const knownWarningCodes = [
  "hdr_not_tone_mapped",
  "subtitles_not_supported",
  "video_downscaled",
] as const;

/** Stream kinds of the API. */
export const knownStreamKinds = [
  "video",
  "audio",
  "subtitle",
  "other",
] as const satisfies readonly MediaStreamKind[];

/** Stream actions of the API. */
export const knownStreamActions = ["copy", "transcode"] as const satisfies readonly StreamAction[];

/** How an encoder runs: on the CPU, or on a GPU or media engine (milestone 4). */
export const encoderTypes = ["software", "hardware"] as const;

/** Units the source bit rate is shown in. */
export const bitRateUnits = ["kilobits", "megabits"] as const;

/** A known reason code. */
export type KnownReasonCode = (typeof knownReasonCodes)[number];
/** A known warning code. */
export type KnownWarningCode = (typeof knownWarningCodes)[number];

type Key<Group extends string, Name extends string> = `streamInfo.${Group}.${Name}`;

/** Key of a mode's name. Params: none. */
export type ModeKey = Key<"mode", (typeof knownModes)[number] | Unknown>;
/** Key of a reason. Params: see `ReasonText`. */
export type ReasonKey = Key<"reason", KnownReasonCode | Unknown>;
/** Key of a warning. Params: see `ReasonText`. */
export type WarningKey = Key<"warning", KnownWarningCode | Unknown>;
/** Key of a stream kind's name. Params: none. */
export type StreamKindKey = Key<"streamKind", (typeof knownStreamKinds)[number] | Unknown>;
/** Key of a stream action's name. Params: none. */
export type StreamActionKey = Key<"action", (typeof knownStreamActions)[number] | Unknown>;
/** Key of the encoder line. Params: `encoder` (FFmpeg encoder name, e.g. `libx264`). */
export type EncoderKey = Key<"encoder", (typeof encoderTypes)[number]>;
/** Key of the source bit rate. Params: `value` (a number in the key's unit). */
export type BitRateKey = Key<"bitRate", (typeof bitRateUnits)[number]>;

/** Every translation key the stream-info view model can contain. */
export type StreamInfoTranslationKey =
  ModeKey | ReasonKey | WarningKey | StreamKindKey | StreamActionKey | EncoderKey | BitRateKey;

function groupKeys<Group extends string, Name extends string>(
  group: Group,
  names: readonly Name[],
): Key<Group, Name>[] {
  return names.map((name) => `streamInfo.${group}.${name}` as const);
}

/**
 * Every key of {@link StreamInfoTranslationKey}, so the app can test that each one is
 * translated in every language.
 */
export const streamInfoTranslationKeys: readonly StreamInfoTranslationKey[] = [
  ...groupKeys("mode", [...knownModes, unknownKeyName]),
  ...groupKeys("reason", [...knownReasonCodes, unknownKeyName]),
  ...groupKeys("warning", [...knownWarningCodes, unknownKeyName]),
  ...groupKeys("streamKind", [...knownStreamKinds, unknownKeyName]),
  ...groupKeys("action", [...knownStreamActions, unknownKeyName]),
  ...groupKeys("encoder", encoderTypes),
  ...groupKeys("bitRate", bitRateUnits),
];

/**
 * Builds a lookup from server values to keys of one group: a known value gets its own key,
 * anything else the group's `unknown` key. A `Set` keeps ids such as `constructor` away from
 * `Object.prototype`.
 */
export function keyLookup<Group extends string, Name extends string>(
  group: Group,
  knownNames: readonly Name[],
): (value: string) => Key<Group, Name | Unknown> {
  const known: ReadonlySet<string> = new Set(knownNames);
  const isKnown = (value: string): value is Name => known.has(value);
  return (value) => `streamInfo.${group}.${isKnown(value) ? value : unknownKeyName}` as const;
}
