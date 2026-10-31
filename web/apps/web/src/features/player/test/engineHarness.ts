// Wires a PlaybackEngine to the fake server (through the real API client and adapter), a fake
// media element, fake hls.js and manual timers.

import type { ClientProfile, PlaybackSession } from "@mivo/player-core";
import { expect, vi } from "vitest";

import { createFakeApiClient } from "../../../test/fakeApi";
import { createPlaybackApi } from "../api/playbackApi";
import type { HlsFactory, HlsStreamError } from "../lib/hlsPlayer";
import { PlaybackEngine, type PlaybackSnapshot } from "../lib/playbackEngine";
import type { PlaybackApi } from "../lib/playbackPorts";
import {
  createPlaybackServer,
  type PlaybackServer,
  type PlaybackServerOptions,
} from "./fakePlaybackServer";
import {
  createFakeHls,
  createManualTimers,
  FakeMedia,
  type FakeHls,
  type ManualTimers,
} from "./fakeMedia";
import { mseProfile, playerFileId } from "./playerFixtures";

/** Options of {@link createEngineHarness}. */
export interface EngineHarnessOptions extends PlaybackServerOptions {
  readonly profile?: ClientProfile;
  /** Makes the hls.js factory reject, like a chunk that failed to load. */
  readonly hlsFailsToLoad?: boolean;
  /** Replaces the recording hls.js stand-ins, e.g. to control when hls.js finishes loading. */
  readonly createHls?: HlsFactory<FakeMedia>;
  /** Wraps the API adapter, e.g. to break one of its promises. */
  readonly wrapApi?: (api: PlaybackApi) => PlaybackApi;
}

/** An engine with its fakes. */
export interface EngineHarness {
  readonly engine: PlaybackEngine<FakeMedia>;
  readonly server: PlaybackServer;
  readonly media: FakeMedia;
  readonly hls: FakeHls;
  readonly timers: ManualTimers;
  /** Opens the player's file at `startPositionSeconds` (default 0). */
  readonly open: (startPositionSeconds?: number) => void;
  /** The current snapshot. */
  readonly snapshot: () => PlaybackSnapshot;
  /** Waits until the engine uses the session with `sessionId`. */
  readonly untilSession: (sessionId: string) => Promise<PlaybackSession>;
  /** Waits until the snapshot satisfies `check`, e.g. a status. */
  readonly until: (check: (snapshot: PlaybackSnapshot) => boolean) => Promise<void>;
}

/** Creates an engine with fakes; the profile defaults to hls.js playback (MSE). */
export function createEngineHarness(options: EngineHarnessOptions = {}): EngineHarness {
  const { profile = mseProfile, hlsFailsToLoad = false, wrapApi = (api) => api } = options;
  const server = createPlaybackServer(options);
  const media = new FakeMedia();
  const hls = createFakeHls(hlsFailsToLoad);
  const timers = createManualTimers();
  const engine = new PlaybackEngine<FakeMedia>({
    api: wrapApi(createPlaybackApi(createFakeApiClient(server.fetch))),
    createHls: options.createHls ?? hls.factory,
    startTimer: timers.startTimer,
  });
  const snapshot = () => engine.getSnapshot();
  return {
    engine,
    server,
    media,
    hls,
    timers,
    snapshot,
    open: (startPositionSeconds = 0) => {
      engine.open({ media, profile, fileId: playerFileId, startPositionSeconds });
    },
    untilSession: async (sessionId) => {
      await vi.waitFor(() => {
        expect(snapshot().session?.id).toBe(sessionId);
      });
      const session = snapshot().session;
      if (session === undefined) {
        throw new Error("the session disappeared while waiting for it");
      }
      return session;
    },
    until: async (check) => {
      await vi.waitFor(() => {
        expect(check(snapshot())).toBe(true);
      });
    },
  };
}

/** A fatal hls.js error with the given details and HTTP status. */
export function fatalHlsError(
  details: string,
  status?: number,
  retryAfterSeconds?: number,
): HlsStreamError {
  return {
    error: {
      type: "networkError",
      details,
      fatal: true,
      response: status === undefined ? undefined : { code: status },
    },
    retryAfterSeconds,
  };
}
