import { describe, expect, it } from "vitest";

import type { PlaybackSession, StreamDecision } from "./playbackSession";
import { bitRateText, streamInfo, type StreamInfo } from "./streamInfo";
import { copied, directPlaySession, remuxSession, transcodeSession } from "./test/sessions";

describe("streamInfo", () => {
  it("describes a direct-play session", () => {
    expect(streamInfo(directPlaySession)).toEqual<StreamInfo>({
      mode: { id: "direct_play", key: "streamInfo.mode.direct_play", colorToken: "directPlay" },
      reasons: [],
      warnings: [],
      streams: [
        {
          kind: "video",
          kindKey: "streamInfo.streamKind.video",
          sourceIndex: 0,
          action: "copy",
          actionKey: "streamInfo.action.copy",
          sourceCodec: "H.264",
          targetCodec: "H.264",
          targetResolution: undefined,
          targetChannels: undefined,
          encoder: undefined,
        },
        {
          kind: "audio",
          kindKey: "streamInfo.streamKind.audio",
          sourceIndex: 1,
          action: "copy",
          actionKey: "streamInfo.action.copy",
          sourceCodec: "AAC",
          targetCodec: "AAC",
          targetResolution: undefined,
          targetChannels: undefined,
          encoder: undefined,
        },
      ],
      container: { source: "MP4", target: "MP4", changed: false },
      encoder: undefined,
      sourceBitRate: { key: "streamInfo.bitRate.megabits", params: { value: 4.1 } },
    });
  });

  it("describes a remux with the container change as its reason", () => {
    const info = streamInfo(remuxSession);

    expect(info.mode).toEqual({
      id: "remux",
      key: "streamInfo.mode.remux",
      colorToken: "directStream",
    });
    expect(info.reasons).toEqual([
      {
        code: "container_not_supported",
        scope: "container",
        key: "streamInfo.reason.container_not_supported",
        params: { code: "container_not_supported", sourceContainer: "MKV", targetContainer: "MP4" },
      },
    ]);
    expect(info.container).toEqual({ source: "MKV", target: "MP4", changed: true });
    expect(info.encoder).toBeUndefined();
  });

  it("describes a transcode with per-stream reasons, warnings and the encoder", () => {
    const info = streamInfo(transcodeSession);

    expect(info.mode).toEqual({
      id: "transcode",
      key: "streamInfo.mode.transcode",
      colorToken: "transcode",
    });
    expect(info.reasons.map(({ key, params }) => ({ key, params }))).toEqual([
      {
        key: "streamInfo.reason.container_not_supported",
        params: { code: "container_not_supported", sourceContainer: "MKV", targetContainer: "MP4" },
      },
      {
        key: "streamInfo.reason.video_codec_not_supported",
        params: {
          code: "video_codec_not_supported",
          sourceCodec: "HEVC",
          targetCodec: "H.264",
          resolution: "1080p",
        },
      },
      {
        key: "streamInfo.reason.audio_codec_not_supported",
        params: {
          code: "audio_codec_not_supported",
          sourceCodec: "AC-3",
          targetCodec: "AAC",
          channels: "2.0",
        },
      },
    ]);
    expect(info.warnings.map(({ key, scope }) => ({ key, scope }))).toEqual([
      { key: "streamInfo.warning.hdr_not_tone_mapped", scope: "video" },
      { key: "streamInfo.warning.video_downscaled", scope: "video" },
      { key: "streamInfo.warning.subtitles_not_supported", scope: "subtitle" },
    ]);
    expect(info.warnings[2]?.params).toEqual({ code: "subtitles_not_supported" });
    expect(
      info.streams.map((line) => [line.sourceCodec, line.targetCodec, line.actionKey]),
    ).toEqual([
      ["HEVC", "H.264", "streamInfo.action.transcode"],
      ["AC-3", "AAC", "streamInfo.action.transcode"],
    ]);
    expect(info.streams[0]).toMatchObject({ targetResolution: "1080p", encoder: "libx264" });
    expect(info.streams[1]).toMatchObject({ targetChannels: "2.0", encoder: "aac" });
    expect(info.encoder).toEqual({
      key: "streamInfo.encoder.software",
      params: { encoder: "libx264" },
    });
    expect(info.sourceBitRate).toEqual({
      key: "streamInfo.bitRate.megabits",
      params: { value: 18.5 },
    });
  });

  it("falls back to generic keys and a muted colour for values of a newer server", () => {
    const future = {
      ...transcodeSession,
      mode: "transcode_hardware",
      streams: [
        { ...copied({ kind: "video", sourceIndex: 0, codec: "h264" }), action: "passthrough" },
        { ...copied({ kind: "audio", sourceIndex: 1, codec: "aac" }), kind: "data" },
      ],
      reasons: [{ code: "bandwidth_too_low", scope: "network" }],
      warnings: [{ code: "audio_downmixed", scope: "audio" }],
    } as unknown as PlaybackSession;

    const info = streamInfo(future);

    expect(info.mode).toEqual({
      id: "transcode_hardware",
      key: "streamInfo.mode.unknown",
      colorToken: "textMuted",
    });
    expect(info.reasons).toEqual([
      {
        code: "bandwidth_too_low",
        scope: "network",
        key: "streamInfo.reason.unknown",
        params: { code: "bandwidth_too_low" },
      },
    ]);
    expect(info.warnings).toEqual([
      {
        code: "audio_downmixed",
        scope: "audio",
        key: "streamInfo.warning.unknown",
        params: { code: "audio_downmixed" },
      },
    ]);
    expect(info.streams.map(({ kindKey, actionKey }) => ({ kindKey, actionKey }))).toEqual([
      { kindKey: "streamInfo.streamKind.video", actionKey: "streamInfo.action.unknown" },
      { kindKey: "streamInfo.streamKind.unknown", actionKey: "streamInfo.action.copy" },
    ]);
  });

  it("treats inherited object keys as unknown codes", () => {
    const session: PlaybackSession = {
      ...remuxSession,
      reasons: [{ code: "constructor", scope: "container" }],
      warnings: [{ code: "toString", scope: "video" }],
    };

    const info = streamInfo(session);

    expect(info.reasons[0]?.key).toBe("streamInfo.reason.unknown");
    expect(info.warnings[0]?.key).toBe("streamInfo.warning.unknown");
  });

  it("gives a stream-scoped reason only its code when the session has no such stream", () => {
    const audioOnly: PlaybackSession = {
      ...transcodeSession,
      streams: transcodeSession.streams.filter((decision) => decision.kind === "audio"),
      reasons: [{ code: "video_codec_not_supported", scope: "video" }],
      warnings: [],
    };

    expect(streamInfo(audioOnly).reasons[0]?.params).toEqual({
      code: "video_codec_not_supported",
    });
  });
});

describe("stream lines", () => {
  const video: StreamDecision = {
    kind: "video",
    sourceIndex: 0,
    action: "transcode",
    sourceCodec: "mpeg2video",
    targetCodec: "h264",
    encoder: "libx264",
  };

  it.each<{ name: string; decision: StreamDecision; expected: string | undefined }>([
    {
      name: "a letterboxed frame",
      decision: { ...video, targetWidth: 1920, targetHeight: 800 },
      expected: "1080p",
    },
    {
      name: "a height without a width",
      decision: { ...video, targetHeight: 720 },
      expected: "720p",
    },
    { name: "no height", decision: { ...video, targetWidth: 1920 }, expected: undefined },
    { name: "an unusable height", decision: { ...video, targetHeight: 0 }, expected: undefined },
  ])("labels the target resolution of $name", ({ decision, expected }) => {
    const info = streamInfo({ ...directPlaySession, streams: [decision] });

    expect(info.streams[0]?.targetResolution).toBe(expected);
  });

  it.each<{ encoder: string; expected: string }>([
    { encoder: "libx264", expected: "streamInfo.encoder.software" },
    { encoder: "aac", expected: "streamInfo.encoder.software" },
    { encoder: "h264_nvenc", expected: "streamInfo.encoder.hardware" },
    { encoder: "hevc_qsv", expected: "streamInfo.encoder.hardware" },
    { encoder: "h264_vaapi", expected: "streamInfo.encoder.hardware" },
    { encoder: "h264_videotoolbox", expected: "streamInfo.encoder.hardware" },
    { encoder: "h264_amf", expected: "streamInfo.encoder.hardware" },
    { encoder: "h264_v4l2m2m", expected: "streamInfo.encoder.hardware" },
    { encoder: "h264_rkmpp", expected: "streamInfo.encoder.hardware" },
    { encoder: "h264_vulkan", expected: "streamInfo.encoder.hardware" },
  ])("classifies the encoder $encoder", ({ encoder, expected }) => {
    const info = streamInfo({ ...directPlaySession, streams: [{ ...video, encoder }] });

    expect(info.encoder).toEqual({ key: expected, params: { encoder } });
  });

  it("names the audio encoder when only audio is re-encoded", () => {
    const audioOnlyTranscode: PlaybackSession = {
      ...transcodeSession,
      streams: [
        copied({ kind: "video", sourceIndex: 0, codec: "h264" }),
        ...transcodeSession.streams.filter((decision) => decision.kind === "audio"),
      ],
    };

    expect(streamInfo(audioOnlyTranscode).encoder).toEqual({
      key: "streamInfo.encoder.software",
      params: { encoder: "aac" },
    });
  });

  it("has no encoder when a re-encoded stream does not name one", () => {
    const withoutEncoder: StreamDecision = {
      kind: "video",
      sourceIndex: 0,
      action: "transcode",
      sourceCodec: "hevc",
      targetCodec: "h264",
    };

    expect(streamInfo({ ...directPlaySession, streams: [withoutEncoder] }).encoder).toBeUndefined();
  });

  it("labels any channel count", () => {
    const surround: StreamDecision = {
      ...copied({ kind: "audio", sourceIndex: 1, codec: "eac3" }),
      targetChannels: 6,
    };

    expect(streamInfo({ ...directPlaySession, streams: [surround] }).streams[0]).toMatchObject({
      sourceCodec: "E-AC-3",
      targetChannels: "5.1",
    });
  });
});

describe("bitRateText", () => {
  it.each([
    {
      bitsPerSecond: 4_073_000,
      expected: { key: "streamInfo.bitRate.megabits", params: { value: 4.1 } },
    },
    {
      bitsPerSecond: 1_000_000,
      expected: { key: "streamInfo.bitRate.megabits", params: { value: 1 } },
    },
    {
      bitsPerSecond: 999_600,
      expected: { key: "streamInfo.bitRate.megabits", params: { value: 1 } },
    },
    {
      bitsPerSecond: 999_400,
      expected: { key: "streamInfo.bitRate.kilobits", params: { value: 999 } },
    },
    {
      bitsPerSecond: 128_000,
      expected: { key: "streamInfo.bitRate.kilobits", params: { value: 128 } },
    },
    { bitsPerSecond: 400, expected: { key: "streamInfo.bitRate.kilobits", params: { value: 0 } } },
    {
      bitsPerSecond: 85_250_000,
      expected: { key: "streamInfo.bitRate.megabits", params: { value: 85.3 } },
    },
  ])("shows $bitsPerSecond bit/s as $expected.params.value", ({ bitsPerSecond, expected }) => {
    expect(bitRateText(bitsPerSecond)).toEqual(expected);
  });

  it.each([undefined, 0, -1, Number.NaN, Number.POSITIVE_INFINITY])(
    "treats %s as unknown",
    (bitsPerSecond) => {
      expect(bitRateText(bitsPerSecond)).toBeUndefined();
    },
  );

  it("is absent from the overlay when the session has no bit rate", () => {
    const withoutBitRate: PlaybackSession = { ...directPlaySession };
    delete withoutBitRate.sourceBitRate;

    expect(streamInfo(withoutBitRate).sourceBitRate).toBeUndefined();
  });
});
