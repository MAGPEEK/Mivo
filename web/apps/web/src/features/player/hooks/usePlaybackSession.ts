import type { ClientProfile, StartTimer } from "@mivo/player-core";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState, useSyncExternalStore } from "react";

import { useApiClient } from "../../../shared/api";
import { createPlaybackApi } from "../api/playbackApi";
import { clientProfileQueryOptions, playerItemQueryOptions } from "../api/playerQueries";
import { browserStartTimer, detectBrowserProfile } from "../lib/browserProfile";
import { createHlsJsPlayer } from "../lib/hlsJs";
import type { HlsFactory } from "../lib/hlsPlayer";
import { genericPlaybackError } from "../lib/playbackErrors";
import { PlaybackEngine, type PlaybackSnapshot } from "../lib/playbackEngine";
import { resolvePlaybackStart, type PlaybackStart } from "../lib/playbackStart";

/** What the player opens: the route parameters of `/items/$itemId/play`. */
export interface PlaybackTarget {
  readonly itemId: string;
  /** File to play; `undefined` plays the item's primary file. */
  readonly fileId: string | undefined;
  /** Start position in seconds from the URL; clamped to the file. */
  readonly startAt: number | undefined;
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
}

function usePlaybackEngine(services: PlaybackServices): PlaybackEngine<HTMLVideoElement> {
  const client = useApiClient();
  const { createHls = createHlsJsPlayer, startTimer = browserStartTimer } = services;
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
  return {
    profile: profileQuery.data,
    start,
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

/**
 * Plays `target` in the `<video>` element passed to `attachVideo` (task M2-27): detects the client
 * profile once per page load, creates a playback session for the item's file and attaches its
 * stream (file, hls.js or native HLS). Unmounting or hiding the page stops the session.
 */
export function usePlaybackSession(
  target: PlaybackTarget,
  services: PlaybackServices = {},
): PlaybackSessionView {
  const engine = usePlaybackEngine(services);
  const snapshot = useSyncExternalStore(engine.subscribe, engine.getSnapshot);
  const { profile, start, failed, reload } = usePlaybackPreconditions(
    target,
    services.detectProfile ?? detectBrowserProfile,
  );
  const [media, setMedia] = useState<HTMLVideoElement | null>(null);
  const fileId = start?.fileId;
  const startPositionSeconds = start?.startPositionSeconds ?? 0;

  useEffect(() => {
    if (media === null || profile === undefined || fileId === undefined) {
      return;
    }
    engine.open({ media, profile, fileId, startPositionSeconds });
    return () => {
      engine.close();
    };
  }, [engine, media, profile, fileId, startPositionSeconds]);
  usePageLifecycle(engine);

  const controls = {
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
