// Which tracks play without a choice of the viewer. This mirrors the server's default policy
// (ADR 0023 Notes M4-04a) so the track menu can preselect before a session exists; once a
// session runs, what it reports (`sessionAudioIndex`, `sessionSubtitle`) is the truth.

import type { ClientProfile, SubtitlePreferenceMode } from "../profile/clientProfile";
import { languageRank } from "./language";
import type { AudioTrack, SubtitleTrack } from "./trackModel";

/** The track preference of a viewer, the same fields as on the client profile. */
export interface TrackPreference {
  /** Audio languages, most wanted first. */
  readonly audioLanguages?: readonly string[] | undefined;
  /** Languages of full subtitles for mode `always`; falls back to the audio languages. */
  readonly subtitleLanguages?: readonly string[] | undefined;
  /** Omitted means `forced`. */
  readonly subtitleMode?: SubtitlePreferenceMode | undefined;
  readonly preferSdh?: boolean | undefined;
}

/** The track preference a client profile carries. */
export function trackPreferenceOf(profile: ClientProfile): TrackPreference {
  return {
    audioLanguages: profile.preferredAudioLanguages,
    subtitleLanguages: profile.preferredSubtitleLanguages,
    subtitleMode: profile.subtitleMode,
    preferSdh: profile.preferSdh,
  };
}

/** Lower is better, compared position by position; ties keep the earlier track. */
type Rank = readonly number[];

function isBetter(rank: Rank, other: Rank): boolean {
  for (const [position, value] of rank.entries()) {
    const otherValue = other[position] ?? value;
    if (value !== otherValue) {
      return value < otherValue;
    }
  }
  return false;
}

function best<Track>(tracks: readonly Track[], rankOf: (track: Track) => Rank): Track | undefined {
  let winner: { readonly track: Track; readonly rank: Rank } | undefined;
  for (const track of tracks) {
    const rank = rankOf(track);
    if (winner === undefined || isBetter(rank, winner.rank)) {
      winner = { track, rank };
    }
  }
  return winner?.track;
}

/**
 * The audio track that plays by default: no commentary or audio description, then the earliest
 * preferred language, then the default flag, then file order. Channel counts do not rank,
 * because browsers play the first track of a progressive file and "stereo first, 5.1 second"
 * must keep direct play. `undefined` for a file without audio.
 */
export function defaultAudioTrack(
  tracks: readonly AudioTrack[],
  preference: TrackPreference = {},
): AudioTrack | undefined {
  const languages = preference.audioLanguages ?? [];
  return best(tracks, (track) => [
    Number(track.commentary),
    languageRank(track.language, languages),
    Number(!track.default),
  ]);
}

/**
 * The subtitle track shown by default for audio in `audioLanguage`, `undefined` for none:
 *
 * - `off` shows nothing.
 * - `forced` (also an omitted mode) shows a forced track in the audio language; an
 *   undetermined audio language shows nothing.
 * - `always` shows a full track in the earliest preferred subtitle language (empty list: the
 *   audio languages; no language at all: any track). A full track ranks above a forced-only
 *   one, then the language, the wanted SDH kind, the default flag and file order.
 *
 * The default never burns in (ADR 0030 Notes M4-02b), so only tracks the client renders itself
 * are candidates.
 */
export function defaultSubtitleTrack(
  tracks: readonly SubtitleTrack[],
  audioLanguage: string | undefined,
  preference: TrackPreference = {},
): SubtitleTrack | undefined {
  const mode = preference.subtitleMode ?? "forced";
  if (mode === "always") {
    return bestSubtitle(tracks, alwaysLanguages(preference), {
      preferSdh: preference.preferSdh === true,
      forcedOnly: false,
    });
  }
  if (mode === "forced") {
    return bestSubtitle(tracks, [audioLanguage ?? ""], {
      preferSdh: preference.preferSdh === true,
      forcedOnly: true,
    });
  }
  // "off", and modes of later versions this one does not know.
  return undefined;
}

function alwaysLanguages(preference: TrackPreference): readonly string[] {
  const subtitleLanguages = preference.subtitleLanguages ?? [];
  return subtitleLanguages.length > 0 ? subtitleLanguages : (preference.audioLanguages ?? []);
}

interface SubtitleRule {
  readonly preferSdh: boolean;
  readonly forcedOnly: boolean;
}

/** An empty language list accepts every language. */
function bestSubtitle(
  tracks: readonly SubtitleTrack[],
  languages: readonly string[],
  rule: SubtitleRule,
): SubtitleTrack | undefined {
  const candidates = tracks.filter(
    (track) =>
      track.sidecar &&
      (!rule.forcedOnly || track.forced) &&
      (languages.length === 0 || languageRank(track.language, languages) < languages.length),
  );
  return best(candidates, (track) => [
    Number(track.forced),
    languageRank(track.language, languages),
    Number(track.sdh !== rule.preferSdh),
    Number(!track.default),
  ]);
}
