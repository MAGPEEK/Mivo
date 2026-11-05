// The player's sound setting, kept per device (PL-04): one browser profile remembers the last
// volume and mute state, so the next video starts as loud as the last one ended.

import { productId } from "@mivo/ui-tokens";

/** Volume (0 to 1) and mute state as the player keeps them. */
export interface VolumePreference {
  readonly volume: number;
  readonly muted: boolean;
}

/** Key of the preference in `localStorage`. */
export const volumeStorageKey = `${productId}.player.volume`;

function isVolume(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 && value <= 1;
}

function parsePreference(stored: string): VolumePreference | undefined {
  const parsed: unknown = JSON.parse(stored);
  if (typeof parsed !== "object" || parsed === null) {
    return undefined;
  }
  const volume: unknown = Reflect.get(parsed, "volume");
  const muted: unknown = Reflect.get(parsed, "muted");
  return isVolume(volume) && typeof muted === "boolean" ? { volume, muted } : undefined;
}

/**
 * Reads the saved preference; `undefined` without storage, without a saved value or for a
 * value this version cannot read (hand-edited, from an older format).
 */
export function readVolumePreference(storage: Storage | undefined): VolumePreference | undefined {
  try {
    const stored = storage?.getItem(volumeStorageKey);
    return stored === null || stored === undefined ? undefined : parsePreference(stored);
  } catch {
    return undefined;
  }
}

/**
 * Saves the preference. A missing storage or a failed write (quota, privacy mode) is ignored:
 * the setting then lasts only as long as the page.
 */
export function writeVolumePreference(
  storage: Storage | undefined,
  preference: VolumePreference,
): void {
  try {
    storage?.setItem(volumeStorageKey, JSON.stringify(preference));
  } catch {
    // Deliberately ignored, see above.
  }
}

/** Volume a muted-at-zero player comes back with when nothing louder was saved. */
export const defaultVolume = 1;

/**
 * The preference to save for the element's sound. A volume of 0 is saved as "muted" with the
 * last audible volume (or {@link defaultVolume}), so unmuting later is never silent.
 */
export function preferenceToSave(
  sound: VolumePreference,
  previous: VolumePreference | undefined,
): VolumePreference {
  if (sound.volume > 0) {
    return sound;
  }
  return { volume: previous?.volume ?? defaultVolume, muted: true };
}

/** The sound settings of a media element. */
export interface SoundSettings {
  volume: number;
  muted: boolean;
}

/** Gives `media` the saved sound settings. */
export function applyVolumePreference(media: SoundSettings, preference: VolumePreference): void {
  media.volume = preference.volume;
  media.muted = preference.muted;
}
