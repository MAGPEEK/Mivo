import { knownDuration, streamInfo, type AudioTrack, type StreamInfo } from "@mivo/player-core";
import { useCallback, useRef, useState, type RefObject } from "react";

import { currentAudioIndex } from "../lib/optionRows";
import { alternativeAudioTracks } from "../lib/reasonHints";
import type { MediaState } from "../lib/mediaState";
import type { PlaybackSnapshot } from "../lib/playbackEngine";
import { playbackNotice, type PlaybackNotice } from "../lib/playbackStatusText";
import type { SessionToastKind } from "../lib/sessionToasts";
import { useControlsVisibility, useKeyboardFocusInControls } from "./useControlsVisibility";
import { useFullscreen, type Fullscreen } from "./useFullscreen";
import {
  usePlaybackOptions,
  type PlaybackOptionsActions,
  type PlaybackOptionsInput,
  type PlaybackOptionsView,
} from "./usePlaybackOptions";
import { usePlaybackPreferences } from "./usePlaybackPreferences";
import { useInitialPlayChoice } from "./useInitialPlayChoice";
import { usePlaybackSession, type PlaybackTarget } from "./usePlaybackSession";
import { usePlayerItem, type PlayerItemInfo } from "./usePlayerItem";
import { usePlayerMedia, type MediaActions } from "./usePlayerMedia";
import type { PlayerQueueView } from "./usePlayerQueue";
import { usePlayerShortcuts } from "./usePlayerShortcuts";
import { usePlayerTransport, type TransportActions } from "./usePlayerTransport";
import { useLiveTranscodeSpeed } from "./useLiveTranscodeSpeed";
import { useNetworkStats, type NetworkStats } from "./useNetworkStats";
import { useSessionToasts } from "./useSessionToasts";
import { useStreamInfoOverlay, type StreamInfoOverlay } from "./useStreamInfoOverlay";
import { useSubtitleCues } from "./useSubtitleCues";

/** Everything the player page shows. */
export interface PlayerView {
  /** The item as the title block shows it. */
  readonly item: PlayerItemInfo;
  /** Previous, next, repeat and shuffle as the queue allows them. */
  readonly queue: PlayerQueueView;
  readonly notice: PlaybackNotice;
  /** What the stream-info overlay shows; `undefined` before the first session. */
  readonly streamInfo: StreamInfo | undefined;
  /** Buffer ahead and throughput for the overlay; sampled only while it is open. */
  readonly networkStats: NetworkStats;
  /** Audio tracks other than the one that plays; the overlay's hints to switch need one. */
  readonly alternativeAudio: readonly AudioTrack[];
  /** Displayed file position in seconds. */
  readonly position: number;
  /** Probed duration of the file in seconds; `undefined` until known. */
  readonly duration: number | undefined;
  readonly media: MediaState;
  readonly fullscreenSupported: boolean;
  readonly fullscreenActive: boolean;
  readonly controlsVisible: boolean;
  readonly streamInfoOpen: boolean;
  /** The playback options panel (PL-10 … PL-13). */
  readonly options: PlaybackOptionsView;
  /** The text of the subtitle the player draws itself now; empty when there is none. */
  readonly subtitleText: string;
  /** Messages about what the last switch cost. */
  readonly toasts: readonly SessionToastKind[];
}

/**
 * What the player page's controls do; the ways out and the transport come from
 * {@link TransportActions}, play and sound from {@link MediaActions}.
 */
export interface PlayerActions extends TransportActions, MediaActions {
  /** Moves to a file position in seconds. */
  readonly seek: (position: number) => void;
  readonly toggleFullscreen: () => void;
  readonly toggleStreamInfo: () => void;
  readonly closeStreamInfo: () => void;
  readonly retry: () => void;
  readonly options: PlaybackOptionsActions;
  readonly dismissToasts: () => void;
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
  /** The playback options button; focus returns there when the panel closes. */
  readonly optionsToggleRef: RefObject<HTMLButtonElement | null>;
  readonly streamInfoRegionRef: RefObject<HTMLElement | null>;
  readonly streamInfoHeadingRef: RefObject<HTMLHeadingElement | null>;
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

/** The audio tracks of the file other than the one that plays. */
function otherAudioTracks(
  playback: Pick<PlaybackOptionsInput, "tracks" | "selection" | "session">,
): AudioTrack[] {
  return alternativeAudioTracks(
    playback.tracks?.audio ?? [],
    currentAudioIndex(playback.selection, playback.session),
  );
}

/**
 * What the overlay shows of the session: the speed the open overlay polled replaces the one
 * the session was created with, which is only the first sample.
 */
function useStreamInfoView(
  playback: Pick<PlaybackOptionsInput, "session" | "tracks">,
  overlayOpen: boolean,
): StreamInfo | undefined {
  const { session, tracks } = playback;
  const liveSpeed = useLiveTranscodeSpeed(session?.id, overlayOpen);
  if (session === undefined) {
    return undefined;
  }
  const current = liveSpeed === undefined ? session : { ...session, transcodeSpeed: liveSpeed };
  return streamInfo(current, tracks?.subtitles ?? []);
}

/** Everything the stream-info overlay shows; sampled and polled only while it is open. */
function useStreamInfoData(
  video: HTMLVideoElement | null,
  playback: Pick<PlaybackOptionsInput, "session" | "tracks">,
  overlayOpen: boolean,
): Pick<PlayerView, "streamInfo" | "networkStats"> {
  const networkStats = useNetworkStats(video, playback.session?.streamUrl, overlayOpen);
  return { streamInfo: useStreamInfoView(playback, overlayOpen), networkStats };
}

/** What the full-screen button and the stream-info overlay do. */
function panelActions(
  fullscreen: Fullscreen,
  overlay: Pick<StreamInfoOverlay, "toggle" | "close">,
): Pick<PlayerActions, "toggleFullscreen" | "toggleStreamInfo" | "closeStreamInfo"> {
  return {
    toggleFullscreen: fullscreen.toggle,
    toggleStreamInfo: overlay.toggle,
    closeStreamInfo: overlay.close,
  };
}

/** The item the page shows, the file's duration and the queue-aware transport. */
function usePlayerNavigation(
  target: PlaybackTarget,
  playback: Pick<PlaybackSnapshot, "session" | "position">,
  seek: (position: number) => void,
) {
  const { itemId } = target;
  const item = usePlayerItem(itemId);
  const duration = playback.session === undefined ? undefined : knownDuration(playback.session);
  const transport = usePlayerTransport({ itemId, position: playback.position, duration, seek });
  return { item, duration, transport };
}

/** What the page shows of the full-screen state. */
function fullscreenView(fullscreen: Fullscreen) {
  return { fullscreenSupported: fullscreen.supported, fullscreenActive: fullscreen.active };
}

/**
 * What the file's tracks add to the page: the playback options panel, the subtitle the player
 * draws and the toasts about what a switch cost. Destructure the result: it holds a ref.
 */
function useTracks(
  playback: PlaybackOptionsInput & Pick<PlaybackSnapshot, "switchedSessionId">,
  video: HTMLVideoElement | null,
) {
  const preferences = usePlaybackPreferences(video);
  const options = usePlaybackOptions(playback, preferences);
  const toasts = useSessionToasts(playback);
  const subtitleText = useSubtitleCues(video, playback.session, options.sidecarTrack);
  return {
    view: { options: options.view, subtitleText, toasts: toasts.toasts },
    actions: { options: options.actions, dismissToasts: toasts.dismiss },
    optionsToggleRef: options.toggleRef,
  };
}

/** The playback of `target`, started with the tracks the detail page handed over. */
function usePlaybackWithChoice(target: PlaybackTarget) {
  const choice = useInitialPlayChoice(target.itemId);
  return usePlaybackSession({ ...target, choice });
}

/**
 * The player page's logic (task M2-30): the playback engine, the element's play state and
 * sound, full screen, auto-hiding controls, the stream-info overlay and the keyboard map.
 * Destructure the result: the React compiler treats an object holding refs as a ref.
 */
export function usePlayer(target: PlaybackTarget): Player {
  const { attachVideo, seek, retry, ...playback } = usePlaybackWithChoice(target);
  const [video, videoRef] = useVideoElement(attachVideo);
  const [container, setContainer] = useState<HTMLElement | null>(null);
  const { media, actions: mediaActions } = usePlayerMedia(video, playback);
  const fullscreen = useFullscreen(container);
  const overlay = useStreamInfoOverlay();
  const overlayData = useStreamInfoData(video, playback, overlay.open);
  const [playButtonRef, retryAndFocus] = useRetryWithFocus(retry);
  const { item, duration, transport } = usePlayerNavigation(target, playback, seek);
  const { view: trackView, actions: trackActions, optionsToggleRef } = useTracks(playback, video);
  const controlsVisible = useAutoHidingControls(
    playback.status === "playing" && !media.paused && !overlay.open && !trackView.options.open,
  );
  const actions: PlayerActions = {
    ...transport.actions,
    ...mediaActions,
    ...panelActions(fullscreen, { toggle: overlay.toggle, close: overlay.close }),
    seek,
    retry: retryAndFocus,
    ...trackActions,
  };
  usePlayerShortcuts(actions, playback.position, overlay.open ? overlay.close : fullscreen.exit);
  return {
    view: {
      item,
      queue: transport.queue,
      notice: playbackNotice(playback),
      ...overlayData,
      alternativeAudio: otherAudioTracks(playback),
      position: playback.position,
      duration,
      media,
      ...fullscreenView(fullscreen),
      controlsVisible,
      streamInfoOpen: overlay.open,
      ...trackView,
    },
    actions,
    videoRef,
    containerRef: setContainer,
    playButtonRef,
    optionsToggleRef,
    streamInfoToggleRef: overlay.toggleRef,
    streamInfoRegionRef: overlay.regionRef,
    streamInfoHeadingRef: overlay.headingRef,
  };
}
