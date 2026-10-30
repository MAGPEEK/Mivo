import { describe, expect, it } from "vitest";

import { decodingInfoTimeoutMilliseconds } from "./decodingCheck";
import { detectClientProfile, maxClientNameLength } from "./detectClientProfile";
// Reference profiles of ADR 0025; internal/domain/playback/playbacktest holds the same values.
import chromiumReference from "./fixtures/chromium-desktop.json";
import firefoxReference from "./fixtures/firefox-desktop.json";
import type { ClientProbes } from "./probes";
import {
  chromiumDesktop,
  fakeProbes,
  firefoxDesktop,
  probably,
  safariIos,
  safariMacos,
  withCodecs,
  withoutMediaSource,
  type FakeBrowserAnswers,
} from "./test/fakeBrowsers";

function detect(answers: FakeBrowserAnswers, name = "test-client") {
  return detectClientProfile(fakeProbes(answers), { name });
}

const h264Limit8Bit = {
  codec: "h264",
  maxBitDepth: 8,
  profiles: ["Constrained Baseline", "Baseline", "Main", "High"],
};

const safariDirectPlay = [
  {
    container: "mp4",
    videoCodecs: ["h264", "hevc"],
    audioCodecs: ["aac", "mp3", "flac", "ac3", "eac3"],
  },
  { container: "mov", videoCodecs: ["h264", "hevc"], audioCodecs: ["aac", "mp3", "ac3", "eac3"] },
  { container: "webm", videoCodecs: ["vp8", "vp9"], audioCodecs: ["opus", "vorbis"] },
  { container: "3gp", videoCodecs: ["h264"], audioCodecs: ["aac"] },
];

const safariCodecLimits = [
  h264Limit8Bit,
  { codec: "hevc", maxBitDepth: 10, profiles: ["Main", "Main 10"] },
  { codec: "vp9", maxBitDepth: 8 },
];

describe("detectClientProfile with reference browsers (ADR 0025)", () => {
  it("reproduces the ChromiumDesktop reference profile", async () => {
    await expect(detect(chromiumDesktop, "chromium-desktop")).resolves.toEqual(
      chromiumReference.profile,
    );
  });

  it("reproduces the FirefoxDesktop reference profile", async () => {
    await expect(detect(firefoxDesktop, "firefox-desktop")).resolves.toEqual(
      firefoxReference.profile,
    );
  });

  it("delivers HLS natively in iOS Safari, which has no MediaSource", async () => {
    await expect(detect(safariIos, "safari")).resolves.toEqual({
      name: "safari",
      directPlay: safariDirectPlay,
      hls: {
        delivery: "native",
        videoCodecs: ["h264", "hevc"],
        audioCodecs: ["aac", "mp3", "flac", "ac3", "eac3"],
      },
      codecLimits: safariCodecLimits,
    });
  });

  it("delivers HLS through MSE in macOS Safari although it plays HLS natively", async () => {
    await expect(detect(safariMacos, "safari")).resolves.toEqual({
      name: "safari",
      directPlay: safariDirectPlay,
      hls: {
        delivery: "mse",
        videoCodecs: ["h264", "hevc"],
        audioCodecs: ["aac", "mp3", "flac", "ac3", "eac3"],
      },
      codecLimits: safariCodecLimits,
    });
  });

  it("offers only direct play without Media Source Extensions", async () => {
    await expect(detect(withoutMediaSource)).resolves.toEqual({
      name: "test-client",
      directPlay: [{ container: "mp4", videoCodecs: ["h264"], audioCodecs: ["aac"] }],
      hls: { delivery: "none", videoCodecs: [], audioCodecs: [] },
      codecLimits: [h264Limit8Bit],
    });
  });
});

describe("detectClientProfile direct play", () => {
  it("counts a maybe answer as supported and ignores non-standard answers", async () => {
    const answers: Readonly<Record<string, string>> = {
      "video/webm": "maybe",
      'video/webm; codecs="vp8"': "maybe",
      'audio/webm; codecs="opus"': "no",
    };
    const probes: ClientProbes = {
      mediaElement: { canPlayType: (type) => answers[type] ?? "" },
      startTimer: () => () => undefined,
    };

    const profile = await detectClientProfile(probes, { name: "old" });

    expect(profile.directPlay).toEqual([
      { container: "webm", videoCodecs: ["vp8"], audioCodecs: [] },
    ]);
  });

  it("drops a container whose bare MIME type does not play although its codecs answer", async () => {
    const profile = await detect({
      canPlayType: probably([
        ...withCodecs("video/x-matroska", ["avc1.640028"]),
        ...withCodecs("audio/ogg", ["vorbis"]),
      ]),
    });

    expect(profile.directPlay).toEqual([]);
  });

  it("drops a container whose bare MIME type plays but none of its codecs", async () => {
    const profile = await detect({ canPlayType: { "video/mp2t": "maybe" } });

    expect(profile.directPlay).toEqual([]);
    expect(profile.codecLimits).toEqual([]);
  });

  it("drops a container in which only audio codecs play, like Ogg in Chrome", async () => {
    const profile = await detect(chromiumDesktop);

    expect(profile.directPlay.map(({ container }) => container)).toEqual(["mp4", "webm"]);
  });
});

describe("detectClientProfile HLS delivery", () => {
  it("prefers MSE when it plays an HLS video codec, even if the playlist type plays", async () => {
    const profile = await detect({
      canPlayType: {
        "application/vnd.apple.mpegurl": "probably",
        ...probably(withCodecs("video/mp4", ["avc1.640028"])),
      },
      mediaSourceTypes: withCodecs("video/mp4", ["av01.0.08M.08"]),
    });

    expect(profile.hls).toEqual({ delivery: "mse", videoCodecs: ["av1"], audioCodecs: [] });
    expect(profile.codecLimits).toEqual([{ codec: "av1", maxBitDepth: 8 }]);
  });

  it("falls back to native HLS when MSE plays no video codec", async () => {
    const profile = await detect({
      canPlayType: {
        "application/vnd.apple.mpegurl": "maybe",
        ...probably(withCodecs("video/mp4", ["avc1.640028"])),
      },
      mediaSourceTypes: withCodecs("audio/mp4", ["mp4a.40.2"]),
    });

    expect(profile.hls).toEqual({ delivery: "native", videoCodecs: ["h264"], audioCodecs: [] });
  });

  it("keeps MSE when neither MSE nor the media element plays HLS video", async () => {
    const profile = await detect({
      canPlayType: {},
      mediaSourceTypes: withCodecs("audio/mp4", ["mp4a.40.2"]),
    });

    expect(profile.hls).toEqual({ delivery: "mse", videoCodecs: [], audioCodecs: ["aac"] });
  });
});

describe("detectClientProfile codec limits", () => {
  it("keeps the strictest limit when progressive playback allows more than MSE", async () => {
    const profile = await detect({
      canPlayType: {
        "video/mp4": "maybe",
        ...probably(
          withCodecs("video/mp4", [
            "avc1.42E01E",
            "avc1.42001E",
            "avc1.4D401E",
            "avc1.640028",
            "av01.0.08M.08",
            "av01.0.08M.10",
          ]),
        ),
      },
      mediaSourceTypes: withCodecs("video/mp4", ["avc1.4D401E", "avc1.640028", "av01.0.08M.08"]),
    });

    expect(profile.directPlay).toEqual([
      { container: "mp4", videoCodecs: ["h264", "av1"], audioCodecs: [] },
    ]);
    expect(profile.hls.videoCodecs).toEqual(["h264", "av1"]);
    expect(profile.codecLimits).toEqual([
      { codec: "h264", maxBitDepth: 8, profiles: ["Main", "High"] },
      { codec: "av1", maxBitDepth: 8 },
    ]);
  });

  it("lists a codec nowhere when its contexts share no variant", async () => {
    const profile = await detect({
      canPlayType: {
        "video/mp4": "maybe",
        ...probably(withCodecs("video/mp4", ["avc1.640028", "av01.0.08M.10"])),
      },
      mediaSourceTypes: withCodecs("video/mp4", ["avc1.640028", "av01.0.08M.08"]),
    });

    expect(profile.directPlay).toEqual([
      { container: "mp4", videoCodecs: ["h264"], audioCodecs: [] },
    ]);
    expect(profile.hls.videoCodecs).toEqual(["h264"]);
    expect(profile.codecLimits).toEqual([{ codec: "h264", maxBitDepth: 8, profiles: ["High"] }]);
  });
});

describe("detectClientProfile with Media Capabilities", () => {
  it("requires decodingInfo to report supported", async () => {
    const profile = await detect({
      ...chromiumDesktop,
      mediaCapabilities: {
        unsupported: [
          ...withCodecs("video/mp4", ["av01.0.08M.10"]),
          ...withCodecs("video/webm", ["av01.0.08M.10"]),
        ],
      },
    });

    expect(profile.directPlay[0]?.videoCodecs).toEqual(["h264", "av1", "vp9"]);
    expect(profile.codecLimits).toContainEqual({ codec: "av1", maxBitDepth: 8 });
  });

  it("treats a rejected or throwing decodingInfo as unsupported", async () => {
    const profile = await detect({
      ...chromiumDesktop,
      mediaCapabilities: {
        rejected: withCodecs("audio/mp4", ["flac"]),
        throwing: withCodecs("audio/mp4", ["mp4a.6B"]),
      },
    });

    expect(profile.directPlay[0]?.audioCodecs).toEqual(["aac", "opus"]);
    expect(profile.hls.audioCodecs).toEqual(["aac", "opus"]);
  });

  it("treats a decodingInfo answer that never arrives as unsupported after the timeout", async () => {
    const probes = fakeProbes({
      canPlayType: { "video/webm": "maybe", ...probably(withCodecs("video/webm", ["vp8"])) },
      mediaCapabilities: { pending: withCodecs("video/webm", ["vp8"]) },
    });

    const detection = detectClientProfile(probes, { name: "hanging" });
    expect(probes.timers.map(({ milliseconds }) => milliseconds)).toEqual([
      decodingInfoTimeoutMilliseconds,
    ]);
    probes.timers[0]?.fire();

    await expect(detection).resolves.toMatchObject({ directPlay: [], codecLimits: [] });
  });

  it("cancels the timeout of every answer that arrives in time", async () => {
    const probes = fakeProbes(chromiumDesktop);

    await detectClientProfile(probes, { name: "chromium" });

    expect(probes.timers.length).toBeGreaterThan(0);
    expect(probes.timers.every(({ cancelled }) => cancelled)).toBe(true);
  });

  it("asks decodingInfo once per decoding type and content type", async () => {
    const probes = fakeProbes(safariIos);

    await detectClientProfile(probes, { name: "safari" });

    const keys = probes.decodingQueries.map(
      (query) => `${query.type} ${query.video?.contentType ?? query.audio?.contentType ?? ""}`,
    );
    expect(new Set(keys).size).toBe(keys.length);
    expect(keys).toContain('file audio/mp4; codecs="mp4a.40.2"');
    expect(probes.timers).toHaveLength(keys.length);
  });

  it("asks decodingInfo only for types that passed the type check, with matching query types", async () => {
    const probes = fakeProbes(chromiumDesktop);

    await detectClientProfile(probes, { name: "chromium" });

    const queried = probes.decodingQueries.map((query) => ({
      type: query.type,
      contentType: query.video?.contentType ?? query.audio?.contentType,
    }));
    expect(queried).toContainEqual({ type: "file", contentType: 'video/webm; codecs="vp8"' });
    expect(queried).toContainEqual({
      type: "media-source",
      contentType: 'video/mp4; codecs="avc1.640028"',
    });
    expect(queried).toContainEqual({
      type: "media-source",
      contentType: 'audio/mp4; codecs="mp4a.40.2"',
    });
    expect(queried.map(({ contentType }) => contentType)).not.toContain(
      'video/mp4; codecs="hvc1.2.4.L120.B0"',
    );
  });

  it("describes a 1080p stream in video decoding queries and only the type in audio queries", async () => {
    const probes = fakeProbes(chromiumDesktop);

    await detectClientProfile(probes, { name: "chromium" });

    expect(probes.decodingQueries).toContainEqual({
      type: "file",
      video: {
        contentType: 'video/mp4; codecs="avc1.640028"',
        width: 1920,
        height: 1080,
        bitrate: 8_000_000,
        framerate: 30,
      },
    });
    expect(probes.decodingQueries).toContainEqual({
      type: "file",
      audio: { contentType: 'audio/webm; codecs="opus"' },
    });
  });
});

describe("detectClientProfile name and result ownership", () => {
  it("returns arrays that the caller may change without affecting later profiles", async () => {
    const first = await detect(withoutMediaSource);
    first.hls.videoCodecs.push("h264");
    first.hls.audioCodecs.push("aac");

    const second = await detect(withoutMediaSource);

    expect(second.hls).toEqual({ delivery: "none", videoCodecs: [], audioCodecs: [] });
  });

  it.each([
    { case: "a short name", name: "chromium-desktop", expected: "chromium-desktop" },
    {
      case: "a long name",
      name: "x".repeat(maxClientNameLength + 1),
      expected: "x".repeat(maxClientNameLength),
    },
    {
      case: "a name ending in emoji",
      name: `${"a".repeat(maxClientNameLength - 1)}\u{1F3AC}\u{1F3AC}`,
      expected: `${"a".repeat(maxClientNameLength - 1)}\u{1F3AC}`,
    },
    { case: "a line break", name: "a\nb", expected: "ab" },
    { case: "other control characters", name: "\u0000x\u007Fy\u0085\tz", expected: "xyz" },
    {
      case: "control characters before the cut",
      name: `\r\n${"y".repeat(maxClientNameLength)}`,
      expected: "y".repeat(maxClientNameLength),
    },
  ])("sanitises $case to fit the API's name rules", async ({ name, expected }) => {
    const profile = await detect(withoutMediaSource, name);

    expect(profile.name).toBe(expected);
  });
});
