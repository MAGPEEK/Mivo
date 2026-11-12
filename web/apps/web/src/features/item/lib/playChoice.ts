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

/** The tracks the player is asked to start with; each is omitted when the viewer left it. */
export interface PlayChoice {
  readonly audioStreamIndex?: number;
  /** `SubtitleTrack.id`, or the empty string for "Off". */
  readonly subtitleTrackId?: string;
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
  const audio =
    sameVersion && version.audio.some((option) => option.value === stored.audio)
      ? stored.audio
      : undefined;
  const subtitle =
    sameVersion && isOffOrOffered(version, stored.subtitle) ? stored.subtitle : undefined;
  return {
    version,
    audio: audio ?? version.defaultAudio,
    subtitle: subtitle ?? version.defaultSubtitle,
    choice: {
      ...(audio === undefined ? {} : { audioStreamIndex: Number(audio) }),
      ...(subtitle === undefined ? {} : { subtitleTrackId: subtitle }),
    },
  };
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
