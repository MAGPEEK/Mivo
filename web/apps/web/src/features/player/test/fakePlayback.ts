// A stand-in for usePlaybackSession: the page's tests set what the engine shows and record what
// the controls ask of it. Install it with `vi.mock("../hooks/usePlaybackSession")` and
// `vi.mocked(usePlaybackSession).mockImplementation(fake.usePlayback)`.

import { useSyncExternalStore } from "react";
import { vi } from "vitest";

import type { PlaybackSessionView } from "../hooks/usePlaybackSession";
import type { PlaybackSnapshot } from "../lib/playbackEngine";

/** The fake hook with its controls. */
export interface FakePlayback {
  /** The hook to install in place of usePlaybackSession. */
  readonly usePlayback: () => PlaybackSessionView;
  /** Changes what the engine shows; the page re-renders. */
  readonly show: (snapshot: Partial<PlaybackSnapshot>) => void;
  readonly seek: ReturnType<typeof vi.fn<(position: number) => void>>;
  readonly retry: ReturnType<typeof vi.fn<() => void>>;
  /** Records the call, plays or pauses a playing stream and shows the new intent, like the engine. */
  readonly setAutoplay: ReturnType<typeof vi.fn<(autoplay: boolean) => void>>;
  readonly attachVideo: ReturnType<typeof vi.fn<(element: HTMLVideoElement | null) => void>>;
}

const loading: PlaybackSnapshot = {
  status: "loading",
  session: undefined,
  position: 0,
  waitingForStream: false,
  error: undefined,
  autoplay: true,
};

/** Creates a fake engine hook that starts with `initial` (loading without a session). */
export function createFakePlayback(initial: Partial<PlaybackSnapshot> = {}): FakePlayback {
  const listeners = new Set<() => void>();
  const seek = vi.fn<(position: number) => void>();
  const retry = vi.fn<() => void>();
  const attachVideo = vi.fn<(element: HTMLVideoElement | null) => void>();
  const setAutoplay = vi.fn<(autoplay: boolean) => void>();
  let view: PlaybackSessionView = {
    ...loading,
    ...initial,
    attachVideo,
    seek,
    retry,
    setAutoplay,
  };
  const show = (snapshot: Partial<PlaybackSnapshot>) => {
    view = { ...view, ...snapshot };
    for (const listener of listeners) {
      listener();
    }
  };
  // Like the engine, the intent also plays or pauses the stream that is attached and playing.
  let element: HTMLVideoElement | null = null;
  attachVideo.mockImplementation((video) => {
    element = video;
  });
  setAutoplay.mockImplementation((autoplay) => {
    if (view.status === "playing" && element !== null) {
      if (autoplay) {
        Promise.resolve()
          .then(() => element?.play())
          .catch(() => undefined);
      } else {
        element.pause();
      }
    }
    show({ autoplay });
  });
  const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  };
  return {
    usePlayback: () => useSyncExternalStore(subscribe, () => view),
    show,
    seek,
    retry,
    setAutoplay,
    attachVideo,
  };
}
