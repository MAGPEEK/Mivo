import { useEffect } from "react";

import { browserLocalStorage } from "../../../shared/lib/persistentFlag";
import {
  applyVolumePreference,
  preferenceToSave,
  readVolumePreference,
  writeVolumePreference,
} from "../lib/volumePreference";

/**
 * Keeps the sound of `video` per device: a new element starts with the saved volume and mute
 * state, and every change the user makes is saved again. The element is the source of truth
 * while it lives; `volumechange` covers the slider, the mute button and the M key alike. A
 * slider pulled to 0 is saved as muted at the last audible volume (preferenceToSave).
 */
export function usePersistedVolume(video: HTMLVideoElement | null): void {
  useEffect(() => {
    if (video === null) {
      return undefined;
    }
    const storage = browserLocalStorage();
    const saved = readVolumePreference(storage);
    if (saved !== undefined) {
      applyVolumePreference(video, saved);
    }
    const save = () => {
      const sound = { volume: video.volume, muted: video.muted };
      writeVolumePreference(storage, preferenceToSave(sound, readVolumePreference(storage)));
    };
    video.addEventListener("volumechange", save);
    return () => {
      video.removeEventListener("volumechange", save);
    };
  }, [video]);
}
