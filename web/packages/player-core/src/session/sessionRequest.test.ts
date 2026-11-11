import { describe, expect, it } from "vitest";

import type { ClientProfile } from "../profile/clientProfile";
import { restartEndMarginSeconds } from "../seek/planSeek";
import {
  buildRestartRequest,
  buildSessionRequest,
  isSubtitleTrackId,
  type PlaybackRequestBody,
  type RestartableSession,
  type TrackSelection,
} from "./sessionRequest";

const profile: ClientProfile = {
  name: "Test",
  directPlay: [],
  hls: { delivery: "mse", videoCodecs: ["h264"], audioCodecs: ["aac"] },
};
const fileId = "1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f";
const sessionId = "5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a";
const base: PlaybackRequestBody = { fileId, profile };

describe("isSubtitleTrackId", () => {
  it.each<[string | undefined, boolean]>([
    ["s:3", true],
    ["s:123456789", true],
    ["x:0a1b2c", true],
    ["", false],
    [undefined, false],
    ["s:", false],
    ["s:1234567890", false],
    ["x:XYZ", false],
    ["3", false],
    ["s:3\n", false],
  ])("%j: %s", (id, expected) => {
    expect(isSubtitleTrackId(id)).toBe(expected);
  });
});

describe("buildSessionRequest", () => {
  it.each<{
    name: string;
    selection?: TrackSelection;
    start?: number;
    replaces?: string;
    expected: PlaybackRequestBody;
  }>([
    { name: "nothing chosen leaves everything to the server", expected: base },
    { name: "a start position", start: 600, expected: { ...base, startPositionSeconds: 600 } },
    { name: "position 0 is omitted", start: 0, expected: base },
    { name: "a negative position is omitted", start: -5, expected: base },
    {
      name: "an audio track",
      selection: { audioStreamIndex: 2 },
      expected: { ...base, audioStreamIndex: 2 },
    },
    {
      name: "audio stream 0",
      selection: { audioStreamIndex: 0 },
      expected: { ...base, audioStreamIndex: 0 },
    },
    {
      name: "a negative audio index is omitted",
      selection: { audioStreamIndex: -1 },
      expected: base,
    },
    {
      name: "a fractional audio index is omitted",
      selection: { audioStreamIndex: 1.5 },
      expected: base,
    },
    {
      name: "a subtitle on the first request does not allow degrading",
      selection: { subtitleTrackId: "s:3" },
      expected: { ...base, subtitleTrackId: "s:3" },
    },
    {
      name: "subtitles off is the empty string",
      selection: { subtitleTrackId: "" },
      expected: { ...base, subtitleTrackId: "" },
    },
    {
      name: "a malformed subtitle id is omitted",
      selection: { subtitleTrackId: "3" },
      expected: base,
    },
    {
      name: "a quality limit",
      selection: { maxBitRate: 8_000_000.7 },
      expected: { ...base, maxBitRate: 8_000_000 },
    },
    { name: "original quality is omitted", selection: { maxBitRate: 0 }, expected: base },
    { name: "a negative limit is omitted", selection: { maxBitRate: -1 }, expected: base },
    { name: "an infinite limit is omitted", selection: { maxBitRate: Infinity }, expected: base },
    {
      name: "a replaced session",
      replaces: sessionId,
      expected: { ...base, replacesSessionId: sessionId },
    },
    {
      name: "a subtitle during playback allows degrading",
      selection: { subtitleTrackId: "x:ab12" },
      replaces: sessionId,
      expected: {
        ...base,
        subtitleTrackId: "x:ab12",
        allowDegradeSubtitles: true,
        replacesSessionId: sessionId,
      },
    },
    {
      name: "subtitles off during playback has nothing to degrade",
      selection: { subtitleTrackId: "" },
      replaces: sessionId,
      expected: { ...base, subtitleTrackId: "", replacesSessionId: sessionId },
    },
    {
      name: "everything at once",
      selection: { audioStreamIndex: 1, subtitleTrackId: "s:4", maxBitRate: 4_000_000 },
      start: 12.5,
      replaces: sessionId,
      expected: {
        fileId,
        profile,
        startPositionSeconds: 12.5,
        audioStreamIndex: 1,
        subtitleTrackId: "s:4",
        allowDegradeSubtitles: true,
        maxBitRate: 4_000_000,
        replacesSessionId: sessionId,
      },
    },
  ])("$name", ({ selection, start, replaces, expected }) => {
    const request = buildSessionRequest({
      fileId,
      profile,
      startPositionSeconds: start,
      selection,
      replacesSessionId: replaces,
    });
    expect(request).toStrictEqual(expected);
  });
});

describe("buildRestartRequest", () => {
  const duration = 100;
  const hls: RestartableSession = {
    id: sessionId,
    fileId,
    delivery: "hls",
    startPositionSeconds: 30,
    durationSeconds: duration,
  };

  it("replaces the session at the position it plays now", () => {
    const request = buildRestartRequest({
      session: hls,
      currentTime: 12,
      profile,
      selection: { audioStreamIndex: 2 },
    });
    expect(request).toStrictEqual({
      ...base,
      startPositionSeconds: 42,
      audioStreamIndex: 2,
      replacesSessionId: sessionId,
    });
  });

  it("uses the element clock as the position of a progressive file", () => {
    const request = buildRestartRequest({
      session: { ...hls, delivery: "file", startPositionSeconds: 0 },
      currentTime: 12,
      profile,
      selection: { maxBitRate: 2_000_000 },
    });
    expect(request.startPositionSeconds).toBe(12);
    expect(request.maxBitRate).toBe(2_000_000);
  });

  it("keeps the restart margin before the end of the file", () => {
    const request = buildRestartRequest({
      session: hls,
      currentTime: 500,
      profile,
      selection: {},
    });
    expect(request.startPositionSeconds).toBe(duration - restartEndMarginSeconds);
  });

  it("allows degrading a subtitle, so playback never stops over it", () => {
    const request = buildRestartRequest({
      session: hls,
      currentTime: 1,
      profile,
      selection: { subtitleTrackId: "s:5" },
    });
    expect(request.allowDegradeSubtitles).toBe(true);
    expect(request.subtitleTrackId).toBe("s:5");
  });
});
