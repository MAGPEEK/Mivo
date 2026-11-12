import { useEffect, useState } from "react";

import { browserLocalStorage } from "../../../shared/lib/persistentFlag";
import { readAutoPlay, writeAutoPlay } from "../lib/autoPlayPreference";
import {
  applyPlaybackSpeed,
  readPlaybackSpeed,
  writePlaybackSpeed,
  type PlaybackSpeed,
} from "../lib/playbackSpeed";

/** The playback preferences kept per device (PL-14, PL-15). */
export interface PlaybackPreferences {
  readonly autoPlay: boolean;
  readonly speed: PlaybackSpeed;
  readonly setAutoPlay: (enabled: boolean) => void;
  readonly setSpeed: (speed: PlaybackSpeed) => void;
}

/**
 * Auto play and speed, remembered per device. The speed is applied to `video` as soon as the
 * element exists and again whenever it changes, with the pitch preserved.
 */
export function usePlaybackPreferences(video: HTMLVideoElement | null): PlaybackPreferences {
  const [autoPlay, setAutoPlayState] = useState(() => readAutoPlay(browserLocalStorage()));
  const [speed, setSpeedState] = useState(() => readPlaybackSpeed(browserLocalStorage()));
  useEffect(() => {
    if (video !== null) {
      applyPlaybackSpeed(video, speed);
    }
  }, [video, speed]);
  return {
    autoPlay,
    speed,
    setAutoPlay: (enabled) => {
      setAutoPlayState(enabled);
      writeAutoPlay(browserLocalStorage(), enabled);
    },
    setSpeed: (next) => {
      setSpeedState(next);
      writePlaybackSpeed(browserLocalStorage(), next);
    },
  };
}
