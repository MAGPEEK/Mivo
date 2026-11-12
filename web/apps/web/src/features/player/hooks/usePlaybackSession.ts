import type { ClientProfile, StartTimer, SwitchPlan } from "@mivo/player-core";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState, useSyncExternalStore } from "react";

import { useApiClient } from "../../../shared/api";
import { browserLocalStorage } from "../../../shared/lib/persistentFlag";
import { createPlaybackApi } from "../api/playbackApi";
import { clientProfileQueryOptions, playerItemQueryOptions } from "../api/playerQueries";
import { browserStartTimer, detectBrowserProfile } from "../lib/browserProfile";
import { choiceSelection, type PlayChoice } from "../lib/playChoiceHandoff";
import { playerTracks, type PlayerTracks } from "../lib/playerTracks";
import {
  availableQualityChoice,
  readQualityChoice,
  startingMaxBitRate,
} from "../lib/qualityPreference";
import { createHlsJsPlayer } from "../lib/hlsJs";
import type { HlsFactory } from "../lib/hlsPlayer";
import { genericPlaybackError } from "../lib/playbackErrors";
import { PlaybackEngine, type PlaybackSnapshot } from "../lib/playbackEngine";
import { resolvePlaybackStart, type PlaybackStart } from "../lib/playbackStart";
import { progressTarget } from "../lib/progressTarget";
import { useProgressReporting } from "./useProgressReporting";

/** What the player opens: the route parameters of `/items/$itemId/play`. */
export interface PlaybackTarget {
  readonly itemId: string;
  /** File to play; `undefined` plays the item's primary file. */
  readonly fileId: string | undefined;
  /** Start position in seconds from the URL; clamped to the file. */
  readonly startAt: number | undefined;
  /**
   * The tracks the detail page handed over. Tracks the file does not have are ignored. The
   * object must stay the same between renders: a new one reopens the playback.
   */
  readonly choice?: PlayChoice | undefined;
}

/** Replaceable services, for tests; the defaults use the real browser and hls.js. */
export interface PlaybackServices {
  readonly createHls?: HlsFactory<HTMLVideoElement>;
  readonly detectProfile?: () => Promise<ClientProfile>;
  readonly startTimer?: StartTimer;
}

/** State and controls of the playback for the player page. */
export interface PlaybackSessionView extends PlaybackSnapshot {
  /** Pass as the `<video>` element's `ref`; playback starts once the element exists. */
  readonly attachVideo: (element: HTMLVideoElement | null) => void;
  /** Moves to a file position in seconds (locally or with a new session). */
  readonly seek: (position: number) => void;
  /** Tries again after an error. */
  readonly retry: () => void;
  /** Sets whether playback should run, also before a stream is attached (`autoplay`). */
  readonly setAutoplay: (autoplay: boolean) => void;
  /** Carries out a switch of audio, subtitle or quality the viewer chose (`plan*Switch`). */
  readonly applySwitch: (plan: SwitchPlan) => void;
  /** Turns automatic quality on or off. */
  readonly setAutomaticQuality: (enabled: boolean) => void;
  /** The tracks and quality steps of the file that plays; `undefined` until the item loads. */
  readonly tracks: PlayerTracks | undefined;
  /** What this client plays; `undefined` until it is detected. */
  readonly profile: ClientProfile | undefined;
}

function usePlaybackEngine(
  createHls: HlsFactory<HTMLVideoElement>,
  startTimer: StartTimer,
): PlaybackEngine<HTMLVideoElement> {
  const client = useApiClient();
  return useMemo(
    () =>
      new PlaybackEngine<HTMLVideoElement>({
        api: createPlaybackApi(client),
        createHls,
        startTimer,
      }),
    [client, createHls, startTimer],
  );
}

/**
 * Stops the session when the page is hidden (`pagehide`: closed, reloaded, navigated away or
 * put into the back-forward cache) and restarts it when a cached page is shown again.
 */
function usePageLifecycle(engine: PlaybackEngine<HTMLVideoElement>): void {
  useEffect(() => {
    const onPageHide = () => {
      engine.suspend();
    };
    const onPageShow = (event: PageTransitionEvent) => {
      if (event.persisted) {
        engine.resume();
      }
    };
    window.addEventListener("pagehide", onPageHide);
    window.addEventListener("pageshow", onPageShow);
    return () => {
      window.removeEventListener("pagehide", onPageHide);
      window.removeEventListener("pageshow", onPageShow);
    };
  }, [engine]);
}

/** What playback needs before the first session: the profile and the file to play. */
interface PlaybackPreconditions {
  readonly profile: ClientProfile | undefined;
  readonly start: PlaybackStart | undefined;
  /**
   * Seconds to add to the file position for progress reports; `undefined` when the item
   * does not report its position (`progressTarget`).
   */
  readonly progressOffsetSeconds: number | undefined;
  /** The audio and subtitle tracks and quality steps of the file to play. */
  readonly tracks: PlayerTracks | undefined;
  /** The profile or the item could not be loaded, or the item has no file. */
  readonly failed: boolean;
  /** Loads what failed again. */
  readonly reload: () => void;
}

function usePlaybackPreconditions(
  target: PlaybackTarget,
  detectProfile: () => Promise<ClientProfile>,
): PlaybackPreconditions {
  const client = useApiClient();
  const profileQuery = useQuery(clientProfileQueryOptions(detectProfile));
  const itemQuery = useQuery(playerItemQueryOptions(client, target.itemId));
  const start =
    itemQuery.data === undefined
      ? undefined
      : resolvePlaybackStart(itemQuery.data, target.fileId, target.startAt);
  const progress =
    itemQuery.data === undefined || start === undefined
      ? undefined
      : progressTarget(itemQuery.data, start.fileId);
  const item = itemQuery.data;
  const fileId = start?.fileId;
  // Stable between renders: a new object would reopen the playback.
  const tracks = useMemo(
    () => (item === undefined || fileId === undefined ? undefined : playerTracks(item, fileId)),
    [item, fileId],
  );
  return {
    profile: profileQuery.data,
    start,
    tracks,
    progressOffsetSeconds: progress?.offsetSeconds,
    failed:
      profileQuery.isError || itemQuery.isError || (itemQuery.isSuccess && start === undefined),
    reload: () => {
      if (profileQuery.isError) {
        void profileQuery.refetch();
      }
      void itemQuery.refetch();
    },
  };
}

/** What the engine opens once all of it is known. */
interface PlaybackOpening {
  readonly media: HTMLVideoElement | null;
  readonly profile: ClientProfile | undefined;
  readonly start: PlaybackStart | undefined;
  readonly tracks: PlayerTracks | undefined;
  readonly choice: PlayChoice | undefined;
}

/**
 * Opens the file in the engine once element, profile and file are known; closes it again. The
 * quality remembered for this device is read when the file opens, not while it plays: a change
 * in the options panel switches the running session itself.
 */
function useOpenPlayback(engine: PlaybackEngine<HTMLVideoElement>, opening: PlaybackOpening): void {
  const { media, profile, tracks, choice } = opening;
  const fileId = opening.start?.fileId;
  const startPositionSeconds = opening.start?.startPositionSeconds ?? 0;
  useEffect(() => {
    if (media === null || profile === undefined || fileId === undefined) {
      return;
    }
    const quality = availableQualityChoice(
      readQualityChoice(browserLocalStorage()),
      tracks?.quality ?? [],
    );
    const maxBitRate = startingMaxBitRate(quality);
    engine.open({
      media,
      profile,
      fileId,
      startPositionSeconds,
      selection: { ...choiceSelection(choice, tracks), maxBitRate },
      ...(tracks === undefined
        ? {}
        : { subtitleTracks: tracks.subtitles, qualityOptions: tracks.quality }),
    });
    engine.setAutomaticQuality(quality.kind === "automatic");
    return () => {
      engine.close();
    };
  }, [engine, media, profile, fileId, startPositionSeconds, tracks, choice]);
}

/**
 * Plays `target` in the `<video>` element passed to `attachVideo` (task M2-27): detects the client
 * profile once per page load, creates a playback session for the item's file and attaches its
 * stream (file, hls.js or native HLS). Unmounting or hiding the page stops the session. While
 * a movie, an episode or an audiobook plays, the server learns the position (task M7-35).
 */
export function usePlaybackSession(
  target: PlaybackTarget,
  services: PlaybackServices = {},
): PlaybackSessionView {
  const { createHls = createHlsJsPlayer, startTimer = browserStartTimer } = services;
  const engine = usePlaybackEngine(createHls, startTimer);
  const snapshot = useSyncExternalStore(engine.subscribe, engine.getSnapshot);
  const { profile, start, tracks, progressOffsetSeconds, failed, reload } =
    usePlaybackPreconditions(target, services.detectProfile ?? detectBrowserProfile);
  const [media, setMedia] = useState<HTMLVideoElement | null>(null);

  // Before the effect that opens the engine, so that its last report precedes the close.
  useProgressReporting({
    itemId: target.itemId,
    source: engine,
    media,
    offsetSeconds: progressOffsetSeconds,
    startTimer,
  });
  useOpenPlayback(engine, { media, profile, start, tracks, choice: target.choice });
  usePageLifecycle(engine);

  const controls = {
    tracks,
    profile,
    applySwitch: (plan: SwitchPlan) => {
      engine.applySwitch(plan);
    },
    setAutomaticQuality: (enabled: boolean) => {
      engine.setAutomaticQuality(enabled);
    },
    attachVideo: setMedia,
    seek: (position: number) => {
      engine.seek(position);
    },
    retry: () => {
      engine.retry();
    },
    setAutoplay: (autoplay: boolean) => {
      engine.setAutoplay(autoplay);
    },
  };
  if (failed) {
    // Nothing to play yet; retrying loads what failed instead of creating a session.
    return {
      ...snapshot,
      status: "error",
      error: genericPlaybackError,
      ...controls,
      retry: reload,
    };
  }
  return { ...snapshot, ...controls };
}
