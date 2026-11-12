import type { AudioTrack, KnownReasonCode, KnownWarningCode } from "@mivo/player-core";

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

const aacCodec = "aac";
/** Most channels a browser plays in an HLS stream without conversion (5.1). */
const maxPlayableChannels = 6;

/** Whether the file has an audio track that would answer the reason behind `code`. */
function actionExists(code: string, alternatives: readonly AudioTrack[]): boolean {
  if (code === "audio_codec_not_supported") {
    return alternatives.some((track) => track.codec?.toLowerCase() === aacCodec);
  }
  if (code === "audio_channels_not_supported") {
    return alternatives.some(
      (track) => track.channels !== undefined && track.channels <= maxPlayableChannels,
    );
  }
  return true;
}

/**
 * The hint key for a code as the server sent it; unknown codes have none. A hint that tells the
 * viewer to choose an audio track appears only when the file has such a track other than the
 * one that plays (`alternativeAudio`).
 */
export function reasonHintKey(
  code: string,
  alternativeAudio: readonly AudioTrack[] = [],
): ReasonHintKey | undefined {
  return actionExists(code, alternativeAudio) ? hintLookup.get(code) : undefined;
}

/** The audio tracks of the file other than the one that plays. */
export function alternativeAudioTracks(
  tracks: readonly AudioTrack[],
  currentIndex: number | undefined,
): AudioTrack[] {
  return tracks.filter((track) => track.index !== currentIndex);
}
