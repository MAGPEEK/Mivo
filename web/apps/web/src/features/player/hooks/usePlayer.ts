import { knownDuration, streamInfo, type StreamInfo } from "@mivo/player-core";
import { useQuery } from "@tanstack/react-query";
import { useCallback, useRef, useState, type RefObject } from "react";

import { useApiClient } from "../../../shared/api";
import { playerItemQueryOptions } from "../api/playerQueries";
import type { MediaState } from "../lib/mediaState";
import { seekStepSeconds } from "../lib/playerKeys";
import { playbackNotice, type PlaybackNotice } from "../lib/playbackStatusText";
import { useControlsVisibility, useKeyboardFocusInControls } from "./useControlsVisibility";
import { useFullscreen } from "./useFullscreen";
import { useMediaControls } from "./useMediaControls";
import {
  usePlaybackSession,
  type PlaybackSessionView,
  type PlaybackTarget,
} from "./usePlaybackSession";
import { usePlayerExit } from "./usePlayerExit";
import { usePlayerKeyboard } from "./usePlayerKeyboard";
import { useStreamInfoOverlay } from "./useStreamInfoOverlay";

/** Everything the player page shows. */
export interface PlayerView {
  /** Title of the item; `undefined` until it has loaded. */
  readonly title: string | undefined;
  readonly notice: PlaybackNotice;
  /** What the stream-info overlay shows; `undefined` before the first session. */
  readonly streamInfo: StreamInfo | undefined;
  /** Displayed file position in seconds. */
  readonly position: number;
  /** Probed duration of the file in seconds; `undefined` until known. */
  readonly duration: number | undefined;
  readonly media: MediaState;
  readonly fullscreenSupported: boolean;
  readonly fullscreenActive: boolean;
  readonly controlsVisible: boolean;
  readonly streamInfoOpen: boolean;
}

/** What the player page's controls do. */
export interface PlayerActions {
  readonly togglePlay: () => void;
  /** Moves to a file position in seconds. */
  readonly seek: (position: number) => void;
  readonly toggleMute: () => void;
  readonly setVolume: (volume: number) => void;
  readonly toggleFullscreen: () => void;
  readonly toggleStreamInfo: () => void;
  readonly closeStreamInfo: () => void;
  readonly retry: () => void;
  /** Back in the history, or to the item's page. */
  readonly exit: () => void;
  /** To the item's page. */
  readonly openItem: () => void;
}

/** The player page's state, actions and the elements it needs. */
export interface Player {
  readonly view: PlayerView;
  readonly actions: PlayerActions;
  /** Pass as the `<video>` element's `ref`. */
  readonly videoRef: (element: HTMLVideoElement | null) => void;
  /** Pass as the `ref` of the element that goes full screen (video plus controls). */
  readonly containerRef: (element: HTMLElement | null) => void;
  /** The play button; focus moves there after a retry, which removes the retry button. */
  readonly playButtonRef: RefObject<HTMLButtonElement | null>;
  readonly streamInfoToggleRef: RefObject<HTMLButtonElement | null>;
  readonly streamInfoRegionRef: RefObject<HTMLElement | null>;
  readonly streamInfoHeadingRef: RefObject<HTMLHeadingElement | null>;
}

function usePlayerTitle(itemId: string): string | undefined {
  const client = useApiClient();
  // The engine loads the same query; this only reads its cache entry.
  return useQuery(playerItemQueryOptions(client, itemId)).data?.title;
}

/**
 * The `<video>` element as state, for the hooks that follow it, and a ref callback that also
 * hands it to the engine. The callback is stable: a new one on every render would make React
 * detach and re-attach the element, which closes and reopens the session.
 */
function useVideoElement(
  attachVideo: (element: HTMLVideoElement | null) => void,
): [HTMLVideoElement | null, (element: HTMLVideoElement | null) => void] {
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const videoRef = useCallback(
    (element: HTMLVideoElement | null) => {
      setVideo(element);
      attachVideo(element);
    },
    [attachVideo],
  );
  return [video, videoRef];
}

/**
 * Wraps the engine's retry so that focus moves to the play button: the retry button
 * disappears while the video loads again, which would otherwise drop focus to the document.
 */
function useRetryWithFocus(retry: () => void): [RefObject<HTMLButtonElement | null>, () => void] {
  const playButtonRef = useRef<HTMLButtonElement>(null);
  return [
    playButtonRef,
    () => {
      retry();
      playButtonRef.current?.focus();
    },
  ];
}

/**
 * Whether the controls are visible. `playingFreely` means the video plays and nothing else
 * needs the controls; keyboard focus on a control keeps them, so its focus ring never fades.
 */
function useAutoHidingControls(playingFreely: boolean): boolean {
  const keyboardFocusInControls = useKeyboardFocusInControls();
  return useControlsVisibility(playingFreely && !keyboardFocusInControls);
}

/** What the play button shows and does. */
interface PlayControl {
  readonly paused: boolean;
  readonly togglePlay: () => void;
}

/**
 * Play and pause act on the element while a stream plays. Before that (loading, a restart, a
 * wait for the stream, an error) the element is empty, so they set the engine's intent for
 * the next stream instead, and the button shows that intent.
 */
function playControl(
  playback: Pick<PlaybackSessionView, "status" | "autoplay" | "setAutoplay">,
  element: PlayControl,
): PlayControl {
  if (playback.status === "playing") {
    return element;
  }
  return {
    paused: !playback.autoplay,
    togglePlay: () => {
      playback.setAutoplay(!playback.autoplay);
    },
  };
}

/** The keyboard map (`playerKeys.ts`) on top of the page's actions. */
function usePlayerShortcuts(actions: PlayerActions, position: number, escape: () => void): void {
  usePlayerKeyboard({
    togglePlay: actions.togglePlay,
    seekBackward: () => {
      actions.seek(Math.max(0, position - seekStepSeconds));
    },
    seekForward: () => {
      actions.seek(position + seekStepSeconds);
    },
    toggleFullscreen: actions.toggleFullscreen,
    toggleStreamInfo: actions.toggleStreamInfo,
    toggleMute: actions.toggleMute,
    escape,
  });
}

/**
 * The player page's logic (task M2-30): the playback engine, the element's play state and
 * sound, full screen, auto-hiding controls, the stream-info overlay and the keyboard map.
 * Destructure the result: the React compiler treats an object holding refs as a ref.
 */
export function usePlayer(target: PlaybackTarget): Player {
  const { attachVideo, seek, retry, ...playback } = usePlaybackSession(target);
  const [video, videoRef] = useVideoElement(attachVideo);
  const [container, setContainer] = useState<HTMLElement | null>(null);
  const { session } = playback;
  const { togglePlay, toggleMute, setVolume, ...media } = useMediaControls(video, session);
  const play = playControl(playback, { paused: media.paused, togglePlay });
  const fullscreen = useFullscreen(container);
  const overlay = useStreamInfoOverlay();
  const [playButtonRef, retryAndFocus] = useRetryWithFocus(retry);
  const title = usePlayerTitle(target.itemId);
  const { exit, openItem } = usePlayerExit(target.itemId);
  const controlsVisible = useAutoHidingControls(
    playback.status === "playing" && !play.paused && !overlay.open,
  );
  const actions: PlayerActions = {
    togglePlay: play.togglePlay,
    seek,
    toggleMute,
    setVolume,
    toggleFullscreen: fullscreen.toggle,
    toggleStreamInfo: overlay.toggle,
    closeStreamInfo: overlay.close,
    retry: retryAndFocus,
    exit,
    openItem,
  };
  usePlayerShortcuts(actions, playback.position, overlay.open ? overlay.close : fullscreen.exit);
  return {
    view: {
      title,
      notice: playbackNotice(playback),
      streamInfo: session === undefined ? undefined : streamInfo(session),
      position: playback.position,
      duration: session === undefined ? undefined : knownDuration(session),
      media: { ...media, paused: play.paused },
      fullscreenSupported: fullscreen.supported,
      fullscreenActive: fullscreen.active,
      controlsVisible,
      streamInfoOpen: overlay.open,
    },
    actions,
    videoRef,
    containerRef: setContainer,
    playButtonRef,
    streamInfoToggleRef: overlay.toggleRef,
    streamInfoRegionRef: overlay.regionRef,
    streamInfoHeadingRef: overlay.headingRef,
  };
}
