import type { MediaState } from "../lib/mediaState";
import { useMediaControls } from "./useMediaControls";
import type { PlaybackSessionView } from "./usePlaybackSession";
import { usePersistedVolume } from "./usePersistedVolume";

/** What the play button, the mute button and the volume slider do. */
export interface MediaActions {
  readonly togglePlay: () => void;
  readonly toggleMute: () => void;
  readonly setVolume: (volume: number) => void;
}

/** What the control bar shows of the element and does with it. */
export interface PlayerMedia {
  /** The element's sound and produced ranges; `paused` is what the play button shows. */
  readonly media: MediaState;
  readonly actions: MediaActions;
}

/**
 * Play and pause always set the engine's intent, which plays or pauses an attached stream at
 * once: a recovery or a retry of a stream that was not ready then keeps a pause the user made
 * (ADR 0024 Notes M2-41). The element's own togglePlay is therefore not used. While a stream
 * plays, the button shows and toggles the element's state, which may differ from the intent
 * (the file ended, the browser refused to autoplay). Before that (loading, a restart, a wait
 * for the stream, an error) the element is empty, so the button shows the intent for the next
 * stream. The sound is kept per device (usePersistedVolume).
 */
export function usePlayerMedia(
  video: HTMLVideoElement | null,
  playback: Pick<PlaybackSessionView, "session" | "status" | "autoplay" | "setAutoplay">,
): PlayerMedia {
  const { paused, muted, volume, producedRanges, toggleMute, setVolume } = useMediaControls(
    video,
    playback.session,
  );
  usePersistedVolume(video);
  const shownPaused = playback.status === "playing" ? paused : !playback.autoplay;
  return {
    media: { paused: shownPaused, muted, volume, producedRanges },
    actions: {
      togglePlay: () => {
        playback.setAutoplay(shownPaused);
      },
      toggleMute,
      setVolume,
    },
  };
}
