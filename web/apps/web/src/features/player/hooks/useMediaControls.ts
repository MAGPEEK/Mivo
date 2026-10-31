import type { TimelineSession } from "@mivo/player-core";
import { useMemo, useSyncExternalStore } from "react";

import { setVolume, toggleMute, togglePlayback } from "../lib/mediaActions";
import { createMediaStateStore, type MediaState } from "../lib/mediaState";

/** The element's state with the actions of the control bar. */
export interface MediaControls extends MediaState {
  readonly togglePlay: () => void;
  readonly toggleMute: () => void;
  /** Sets the volume (0 to 1); 0 mutes, anything above unmutes. */
  readonly setVolume: (volume: number) => void;
}

/**
 * Follows the play state, sound and produced ranges of `video` and offers the control bar's
 * actions on it. Seeking goes through the engine instead (`usePlaybackSession`).
 */
export function useMediaControls(
  video: HTMLVideoElement | null,
  session: TimelineSession | undefined,
): MediaControls {
  const store = useMemo(() => createMediaStateStore(video, session), [video, session]);
  const state = useSyncExternalStore(store.subscribe, store.getSnapshot);
  return {
    ...state,
    togglePlay: () => {
      if (video !== null) {
        togglePlayback(video);
      }
    },
    toggleMute: () => {
      if (video !== null) {
        toggleMute(video);
      }
    },
    setVolume: (volume) => {
      if (video !== null) {
        setVolume(video, volume);
      }
    },
  };
}
