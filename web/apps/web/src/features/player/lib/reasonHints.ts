import type { KnownReasonCode, KnownWarningCode } from "@mivo/player-core";

/** Key (in the player namespace) of the one-line action hint of a reason or warning. */
export type ReasonHintKey =
  | "overlay.hint.audio_codec_not_supported"
  | "overlay.hint.audio_channels_not_supported"
  | "overlay.hint.audio_track_not_first"
  | "overlay.hint.subtitle_burn_in"
  | "overlay.hint.bitrate_cap"
  | "overlay.hint.video_downscaled"
  | "overlay.hint.subtitle_burn_in_unavailable";

/**
 * The action hint of every reason and warning code the playback API documents; `undefined`
 * where the viewer can do nothing about it. A new API code fails the test of this table until
 * it is listed here. The options panel (M4-16b) offers each action, so the hints are plain
 * text without a link.
 */
export const reasonHintKeys: Readonly<
  Record<KnownReasonCode | KnownWarningCode, ReasonHintKey | undefined>
> = {
  container_not_supported: undefined,
  video_codec_not_supported: undefined,
  video_profile_not_supported: undefined,
  video_bit_depth_not_supported: undefined,
  audio_codec_not_supported: "overlay.hint.audio_codec_not_supported",
  audio_channels_not_supported: "overlay.hint.audio_channels_not_supported",
  audio_track_not_first: "overlay.hint.audio_track_not_first",
  disc_source: undefined,
  subtitle_burn_in: "overlay.hint.subtitle_burn_in",
  bitrate_cap: "overlay.hint.bitrate_cap",
  dsd_to_pcm: undefined,
  hires_downsampled: undefined,
  hwaccel_unavailable: undefined,
  hdr_not_tone_mapped: undefined,
  subtitles_not_supported: undefined,
  video_downscaled: "overlay.hint.video_downscaled",
  subtitle_burn_in_unavailable: "overlay.hint.subtitle_burn_in_unavailable",
};

const hintLookup: ReadonlyMap<string, ReasonHintKey | undefined> = new Map(
  Object.entries(reasonHintKeys),
);

/** The hint key for a code as the server sent it; unknown codes have none. */
export function reasonHintKey(code: string): ReasonHintKey | undefined {
  return hintLookup.get(code);
}
