import { describe, expect, it } from "vitest";

import {
  audioCodecIds,
  audioCodecVariants,
  containerProbes,
  hlsPlaylistMimeType,
  hlsSegmentProbe,
  videoCodecIds,
  videoCodecVariants,
} from "./probeTables";

function codecStrings(variants: readonly { codecString: string }[]): string[] {
  return variants.map(({ codecString }) => codecString);
}

describe("probe tables (ADR 0023)", () => {
  it("lists the direct-play container keys with their MIME types in ADR order", () => {
    expect(
      containerProbes.map(({ container, videoMimeType }) => [container, videoMimeType]),
    ).toEqual([
      ["mp4", "video/mp4"],
      ["mov", "video/quicktime"],
      ["webm", "video/webm"],
      ["mkv", "video/x-matroska"],
      ["ogg", "video/ogg"],
      ["mpegts", "video/mp2t"],
      ["3gp", "video/3gpp"],
    ]);
  });

  it("probes audio with audio MIME types only", () => {
    for (const context of [...containerProbes, hlsSegmentProbe]) {
      expect(context.audioMimeType).toMatch(/^audio\//);
    }
  });

  it("lists the probed codecs as ffprobe names them", () => {
    // Also the order of the codec limits, aligned with the Go reference profiles.
    expect(videoCodecIds).toEqual(["h264", "hevc", "vp9", "av1", "vp8", "theora"]);
    expect(audioCodecIds).toEqual(["aac", "mp3", "opus", "vorbis", "flac", "ac3", "eac3"]);
  });

  it.each([
    {
      codec: "h264",
      expected: ["avc1.42E01E", "avc1.42001E", "avc1.4D401E", "avc1.640028", "avc1.6E0028"],
    },
    { codec: "hevc", expected: ["hvc1.1.6.L120.B0", "hvc1.2.4.L120.B0"] },
    { codec: "av1", expected: ["av01.0.08M.08", "av01.0.08M.10"] },
    { codec: "vp9", expected: ["vp09.00.40.08", "vp09.02.40.10"] },
    { codec: "vp8", expected: ["vp8"] },
    { codec: "theora", expected: ["theora"] },
  ] as const)("uses the representative strings for video codec $codec", ({ codec, expected }) => {
    expect(codecStrings(videoCodecVariants[codec])).toEqual(expected);
  });

  it.each([
    { codec: "h264", depths: [8, 8, 8, 8, 10] },
    { codec: "hevc", depths: [8, 10] },
    { codec: "av1", depths: [8, 10] },
    { codec: "vp9", depths: [8, 10] },
  ] as const)("has an 8-bit and a 10-bit variant for $codec", ({ codec, depths }) => {
    expect(videoCodecVariants[codec].map(({ bitDepth }) => bitDepth)).toEqual(depths);
  });

  it.each([
    { codec: "aac", expected: "mp4a.40.2" },
    { codec: "mp3", expected: "mp4a.6B" },
    { codec: "opus", expected: "opus" },
    { codec: "vorbis", expected: "vorbis" },
    { codec: "flac", expected: "flac" },
    { codec: "ac3", expected: "ac-3" },
    { codec: "eac3", expected: "ec-3" },
  ] as const)("uses $expected for audio codec $codec", ({ codec, expected }) => {
    expect(codecStrings(audioCodecVariants[codec])).toEqual([expected]);
  });

  it("probes HLS with fMP4 segment types and the Apple playlist type", () => {
    expect(hlsPlaylistMimeType).toBe("application/vnd.apple.mpegurl");
    expect(hlsSegmentProbe).toMatchObject({
      videoMimeType: "video/mp4",
      audioMimeType: "audio/mp4",
    });
  });
});
