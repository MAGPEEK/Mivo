// The playback speed (PL-15), kept per device: one speed for video and, later, audiobooks.

import { productId } from "@mivo/ui-tokens";

/** The speeds the viewer can choose, slowest first. */
export const playbackSpeeds = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2] as const;

/** One of {@link playbackSpeeds}. */
export type PlaybackSpeed = (typeof playbackSpeeds)[number];

/** Normal speed, used when nothing valid was saved. */
export const defaultPlaybackSpeed: PlaybackSpeed = 1;

/** Key of the preference in `localStorage`. */
export const speedStorageKey = `${productId}.player.speed`;

/** Whether `value` is one of the offered speeds. */
export function isPlaybackSpeed(value: unknown): value is PlaybackSpeed {
  return playbackSpeeds.some((speed) => speed === value);
}

/** Reads the saved speed; 1 without storage or for a value this version cannot read. */
export function readPlaybackSpeed(storage: Storage | undefined): PlaybackSpeed {
  try {
    const stored = storage?.getItem(speedStorageKey);
    const parsed = stored === null || stored === undefined ? Number.NaN : Number(stored);
    return isPlaybackSpeed(parsed) ? parsed : defaultPlaybackSpeed;
  } catch {
    return defaultPlaybackSpeed;
  }
}

/** Saves the speed; a missing storage or a failed write only costs the memory. */
export function writePlaybackSpeed(storage: Storage | undefined, speed: PlaybackSpeed): void {
  try {
    storage?.setItem(speedStorageKey, String(speed));
  } catch {
    // Deliberately ignored: the speed then lasts for this page load only.
  }
}

/** The speed-related properties of a media element. */
export interface SpeedSettings {
  playbackRate: number;
  defaultPlaybackRate: number;
  preservesPitch: boolean;
}

/**
 * Gives `media` the speed. The default rate is set too: loading a new source (a switch of
 * audio or quality restarts the stream) resets the rate to it, which would drop the choice.
 */
export function applyPlaybackSpeed(media: SpeedSettings, speed: PlaybackSpeed): void {
  media.defaultPlaybackRate = speed;
  media.playbackRate = speed;
  media.preservesPitch = true;
}
