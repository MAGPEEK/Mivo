import {
  planAudioSwitch,
  planQualitySwitch,
  planSubtitleSwitch,
  type PlaybackSession,
  type SwitchState,
} from "@mivo/player-core";
import { describe, expect, it, vi } from "vitest";

import { createEngineHarness, type EngineHarness } from "../test/engineHarness";
import type { FakeMedia } from "../test/fakeMedia";
import { createdSession, sessionResponse } from "../test/fakePlaybackServer";
import {
  englishSubtitleId,
  imageSubtitleId,
  multiTrackItem,
  multiTrackSession,
  playerFileId,
} from "../test/playerFixtures";
import type { PlaybackOpenRequest } from "./playbackEngine";
import { playerTracks } from "./playerTracks";

const tracks = playerTracks(multiTrackItem(), playerFileId);
const openWithTracks: Partial<PlaybackOpenRequest<FakeMedia>> = {
  subtitleTracks: tracks.subtitles,
  qualityOptions: tracks.quality,
};

/** The state a switch is planned from: what the engine shows now. */
function switchState(harness: EngineHarness): SwitchState {
  const { selection, session } = harness.snapshot();
  if (session === undefined) {
    throw new Error("no session yet");
  }
  return { selection, session };
}

/** An HLS stream that plays: session-1 created, attached, loaded. */
async function playingHls(
  harness: EngineHarness,
  startPositionSeconds = 0,
  extra: Partial<PlaybackOpenRequest<FakeMedia>> = openWithTracks,
): Promise<void> {
  harness.open(startPositionSeconds, extra);
  await harness.untilSession("session-1");
  await vi.waitFor(() => {
    expect(harness.media.calls).toContain("play");
  });
  harness.media.emit("loadeddata");
}

describe("PlaybackEngine switches audio, subtitle and quality", () => {
  it("starts with the selection it was opened with", async () => {
    const harness = createEngineHarness({ delivery: "hls" });

    await playingHls(harness, 0, { ...openWithTracks, selection: { maxBitRate: 4_000_000 } });

    expect(harness.server.sessionRequests[0]).toMatchObject({ maxBitRate: 4_000_000 });
    expect(harness.snapshot().selection).toEqual({ maxBitRate: 4_000_000 });
  });

  it("replaces the session at the current position when the audio track changes", async () => {
    const harness = createEngineHarness({ delivery: "hls" });
    await playingHls(harness, 30);
    harness.media.currentTime = 45;

    harness.engine.applySwitch(planAudioSwitch(switchState(harness), tracks.audio, 2));

    expect(harness.snapshot()).toMatchObject({ status: "loading", switching: true });
    await harness.untilSession("session-2");
    // The HLS stream began at 30 s of the file, so the element's 45 s are file position 75 s.
    expect(harness.server.sessionRequests[1]).toMatchObject({
      audioStreamIndex: 2,
      replacesSessionId: "session-1",
      startPositionSeconds: 75,
    });
    expect(harness.snapshot()).toMatchObject({
      selection: { audioStreamIndex: 2 },
      switchedSessionId: "session-2",
    });
    harness.media.emit("loadeddata");
    expect(harness.snapshot()).toMatchObject({ status: "playing", switching: false });
  });

  it("repeats the choice in every later request, a seek included", async () => {
    const harness = createEngineHarness({ delivery: "hls" });
    await playingHls(harness);
    harness.engine.applySwitch(planAudioSwitch(switchState(harness), tracks.audio, 2));
    await harness.untilSession("session-2");
    harness.media.emit("loadeddata");

    harness.engine.seek(100);

    await harness.untilSession("session-3");
    expect(harness.server.sessionRequests[2]).toMatchObject({
      audioStreamIndex: 2,
      startPositionSeconds: 100,
    });
    expect(harness.snapshot().switching).toBe(false);
    expect(harness.snapshot().switchedSessionId).toBeUndefined();
  });

  it("swaps a text subtitle without a new session", async () => {
    const harness = createEngineHarness({ delivery: "hls" });
    await playingHls(harness);

    const plan = planSubtitleSwitch(switchState(harness), tracks.subtitles, englishSubtitleId);
    harness.engine.applySwitch(plan);

    expect(plan.kind).toBe("sidecar");
    expect(harness.server.sessionRequests).toHaveLength(1);
    expect(harness.snapshot()).toMatchObject({
      status: "playing",
      selection: { subtitleTrackId: englishSubtitleId },
    });
  });

  it("ignores a switch that changes nothing", async () => {
    const harness = createEngineHarness({ delivery: "hls" });
    await playingHls(harness);

    harness.engine.applySwitch(planAudioSwitch(switchState(harness), tracks.audio, 99));
    harness.engine.applySwitch(planQualitySwitch(switchState(harness), 0));

    expect(harness.server.sessionRequests).toHaveLength(1);
    expect(harness.snapshot().switching).toBe(false);
  });

  it("keeps subtitles off after the server refused a burn-in, also in later restarts", async () => {
    const refused: PlaybackSession = {
      ...multiTrackSession,
      id: "session-2",
      delivery: "hls",
      streamUrl: "/api/v1/playback/sessions/session-2/hls/index.m3u8",
      subtitle: { mode: "off" },
      warnings: [{ code: "subtitle_burn_in_unavailable", scope: "subtitle" }],
    };
    const harness = createEngineHarness({
      createSession: (request, index) =>
        sessionResponse(index === 1 ? refused : createdSession(request, index, "hls")),
    });
    await playingHls(harness);

    harness.engine.applySwitch(
      planSubtitleSwitch(switchState(harness), tracks.subtitles, imageSubtitleId),
    );
    await harness.untilSession("session-2");

    expect(harness.server.sessionRequests[1]).toMatchObject({ subtitleTrackId: imageSubtitleId });
    expect(harness.snapshot().selection.subtitleTrackId).toBe("");
    harness.media.emit("loadeddata");
    harness.engine.seek(100);
    await vi.waitFor(() => {
      expect(harness.server.sessionRequests).toHaveLength(3);
    });
    expect(harness.server.sessionRequests[2]).toMatchObject({ subtitleTrackId: "" });
  });

  it("lets a throughput that is too low switch to a cheaper step while Automatic is on", async () => {
    const harness = createEngineHarness({ delivery: "hls" });
    await playingHls(harness);
    harness.engine.setAutomaticQuality(true);

    // 1 MB in 4 s is 2 Mbit/s; with the safety margin only the 1 Mbit/s step fits.
    harness.hls.players[0]?.download({ bytes: 1_000_000, milliseconds: 4000 });

    await harness.untilSession("session-2");
    expect(harness.server.sessionRequests[1]).toMatchObject({
      maxBitRate: 1_000_000,
      replacesSessionId: "session-1",
    });
    expect(harness.snapshot().selection.maxBitRate).toBe(1_000_000);
  });

  it("does not measure when Automatic is off", async () => {
    const harness = createEngineHarness({ delivery: "hls" });
    await playingHls(harness);
    harness.engine.setAutomaticQuality(true);
    harness.engine.setAutomaticQuality(false);

    harness.hls.players[0]?.download({ bytes: 1_000_000, milliseconds: 4000 });

    expect(harness.server.sessionRequests).toHaveLength(1);
  });

  it("forgets the selection when it closes", async () => {
    const harness = createEngineHarness({ delivery: "hls" });
    await playingHls(harness);
    harness.engine.applySwitch(planAudioSwitch(switchState(harness), tracks.audio, 2));
    await harness.untilSession("session-2");

    harness.engine.close();

    expect(harness.snapshot().selection).toEqual({});
  });
});
