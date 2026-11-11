// Completes a detected client profile with what the viewer wants and what the player renders
// (ADR 0030 Notes M4-02a): the track preference, the subtitle formats and the audio limits.

import { preferredLanguageList } from "../tracks/language";
import type { AudioLimits } from "./audioLimits";
import type { ClientProfile, MediaIdentifier, SubtitlePreferenceMode } from "./clientProfile";

/**
 * Subtitle formats the player renders itself from the sidecar route. With `webvtt` listed the
 * server delivers every text track as a sidecar; without it the default policy shows none.
 */
export const renderedSubtitleFormats: readonly MediaIdentifier[] = ["webvtt"];

/** What the viewer set and what was detected beyond codecs. */
export interface PlaybackPreferences {
  /**
   * Audio languages, most wanted first, e.g. the viewer's setting or `navigator.languages`.
   * Invalid tags are dropped and the list is cut to what the API accepts.
   */
  readonly audioLanguages?: readonly string[] | undefined;
  /** Languages of full subtitles for mode `always`. */
  readonly subtitleLanguages?: readonly string[] | undefined;
  /** Omitted means `forced` on the server. */
  readonly subtitleMode?: SubtitlePreferenceMode | undefined;
  readonly preferSdh?: boolean | undefined;
  /** The result of `detectAudioLimits`. */
  readonly audioLimits?: AudioLimits | undefined;
}

/**
 * The profile to send with playback requests: the detected profile plus the track preference,
 * `subtitleFormats: ["webvtt"]` and the audio limits. Empty language lists and absent settings
 * are left out, so the server applies its defaults. The detected profile is not modified.
 */
export function withPlaybackPreferences(
  profile: ClientProfile,
  preferences: PlaybackPreferences = {},
): ClientProfile {
  const result: ClientProfile = {
    ...profile,
    ...preferences.audioLimits,
    subtitleFormats: [...renderedSubtitleFormats],
  };
  const audioLanguages = preferredLanguageList(preferences.audioLanguages ?? []);
  if (audioLanguages.length > 0) {
    result.preferredAudioLanguages = audioLanguages;
  }
  const subtitleLanguages = preferredLanguageList(preferences.subtitleLanguages ?? []);
  if (subtitleLanguages.length > 0) {
    result.preferredSubtitleLanguages = subtitleLanguages;
  }
  if (preferences.subtitleMode !== undefined) {
    result.subtitleMode = preferences.subtitleMode;
  }
  if (preferences.preferSdh !== undefined) {
    result.preferSdh = preferences.preferSdh;
  }
  return result;
}
