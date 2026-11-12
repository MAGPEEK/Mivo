import type { PlayChoice } from "../../player";
import type { VersionChoices } from "./versions";

/**
 * What the viewer picked on the detail page, as the option values of {@link VersionChoices}.
 * Only what was picked is set: a track left out follows the default, so the server's policy
 * (and the profile's preferred languages) still applies.
 */
export interface StoredChoice {
  readonly fileId?: string;
  /** Stream index of the audio track, as a string. */
  readonly audio?: string;
  /** Subtitle track id; the empty string is "Off". */
  readonly subtitle?: string;
}

/** The version and tracks the selectors show, with the choice to hand to the player. */
export interface ResolvedChoice<Version extends VersionChoices> {
  readonly version: Version;
  /** Value of the shown audio track; `undefined` for a file without audio. */
  readonly audio: string | undefined;
  /** Value of the shown subtitle track; the empty string is "Off". */
  readonly subtitle: string;
  readonly choice: PlayChoice;
}

/**
 * The shown version and tracks for a stored choice. A stored value that the file no longer
 * offers (the library changed since) counts as not chosen. `undefined` without any version.
 *
 * The subtitle default follows the shown audio: picking another audio track moves it to the
 * subtitle that goes with that language. That subtitle is then handed over explicitly, so the
 * player starts with what the page shows.
 */
export function resolveChoice<Version extends VersionChoices>(
  versions: readonly Version[],
  stored: StoredChoice,
): ResolvedChoice<Version> | undefined {
  const version = versions.find((candidate) => candidate.fileId === stored.fileId) ?? versions[0];
  if (version === undefined) {
    return undefined;
  }
  // Tracks belong to the version they were picked on.
  const sameVersion = stored.fileId === version.fileId;
  const pickedAudio =
    sameVersion && version.audio.some((option) => option.value === stored.audio)
      ? stored.audio
      : undefined;
  const pickedSubtitle =
    sameVersion && isOffOrOffered(version, stored.subtitle) ? stored.subtitle : undefined;
  const subtitle = pickedSubtitle ?? subtitleDefault(version, pickedAudio);
  return {
    version,
    audio: pickedAudio ?? version.defaultAudio,
    subtitle,
    choice: {
      ...(pickedAudio === undefined ? {} : { audioStreamIndex: Number(pickedAudio) }),
      ...(pickedSubtitle === undefined && pickedAudio === undefined
        ? {}
        : { subtitleTrackId: subtitle }),
    },
  };
}

function subtitleDefault(version: VersionChoices, pickedAudio: string | undefined): string {
  if (pickedAudio === undefined) {
    return version.defaultSubtitle;
  }
  return version.defaultSubtitleByAudio[pickedAudio] ?? version.defaultSubtitle;
}

function isOffOrOffered(version: VersionChoices, value: string | undefined): value is string {
  return value === "" || version.subtitles.some((option) => option.value === value);
}

/** The stored choice after the viewer picked another version: its tracks start from default. */
export function chooseVersion(fileId: string): StoredChoice {
  return { fileId };
}

/** The stored choice after the viewer picked an audio track of the shown version. */
export function chooseAudio(stored: StoredChoice, fileId: string, audio: string): StoredChoice {
  // An explicit subtitle pick stays: it is the viewer's word, whatever the audio.
  return { ...keepTracksOf(stored, fileId), fileId, audio };
}

/** The stored choice after the viewer picked a subtitle track, or "Off" (empty string). */
export function chooseSubtitle(
  stored: StoredChoice,
  fileId: string,
  subtitle: string,
): StoredChoice {
  return { ...keepTracksOf(stored, fileId), fileId, subtitle };
}

function keepTracksOf(stored: StoredChoice, fileId: string): StoredChoice {
  return stored.fileId === fileId ? stored : {};
}
