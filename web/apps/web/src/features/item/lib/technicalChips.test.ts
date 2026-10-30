import type { MediaFile } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { mediaFile, stream } from "../test/fakeItemServer";
import { chipSeparator, technicalChips } from "./technicalChips";

interface ChipCase {
  readonly name: string;
  readonly file: MediaFile;
  readonly expected: string;
}

// Containers and codecs of the test corpus (scripts/gen-testmedia), plus the edge cases.
const chipCases: readonly ChipCase[] = [
  {
    name: "the plan's example: HEVC 10-bit HDR10 with AC-3 5.1 and SRT in MKV",
    file: mediaFile({
      container: "mkv",
      streams: [
        stream({ kind: "video", codec: "hevc", bitDepth: 10, hdr: "hdr10" }),
        stream({ kind: "audio", codec: "ac3", channels: 6 }),
        stream({ kind: "subtitle", codec: "subrip" }),
      ],
    }),
    expected: "MKV · HEVC 10-bit HDR10 · AC-3 5.1 · SRT",
  },
  {
    name: "8-bit SDR video shows the codec alone",
    file: mediaFile({
      container: "mp4",
      streams: [
        stream({ kind: "video", codec: "h264", bitDepth: 8 }),
        stream({ kind: "audio", codec: "aac", channels: 2 }),
      ],
    }),
    expected: "MP4 · H.264 · AAC 2.0",
  },
  {
    name: "HLG video",
    file: mediaFile({
      container: "mkv",
      streams: [
        stream({ kind: "video", codec: "hevc", bitDepth: 10, hdr: "hlg" }),
        stream({ kind: "audio", codec: "eac3", channels: 6 }),
      ],
    }),
    expected: "MKV · HEVC 10-bit HLG · E-AC-3 5.1",
  },
  {
    name: "TrueHD 7.1 with PGS subtitles",
    file: mediaFile({
      container: "m2ts",
      streams: [
        stream({ kind: "video", codec: "h264" }),
        stream({ kind: "audio", codec: "truehd", channels: 8 }),
        stream({ kind: "subtitle", codec: "hdmv_pgs_subtitle" }),
      ],
    }),
    expected: "M2TS · H.264 · TrueHD 7.1 · PGS",
  },
  {
    name: "WebM with VP9 and Opus",
    file: mediaFile({
      container: "webm",
      streams: [
        stream({ kind: "video", codec: "vp9" }),
        stream({ kind: "audio", codec: "opus", channels: 2 }),
      ],
    }),
    expected: "WebM · VP9 · Opus 2.0",
  },
  {
    name: "Ogg with Theora and Vorbis",
    file: mediaFile({
      container: "ogg",
      streams: [
        stream({ kind: "video", codec: "theora" }),
        stream({ kind: "audio", codec: "vorbis", channels: 1 }),
      ],
    }),
    expected: "Ogg · Theora · Vorbis 1.0",
  },
  {
    name: "MPEG-TS with MPEG-2 video, MP2 audio and DVB subtitles",
    file: mediaFile({
      container: "mpegts",
      streams: [
        stream({ kind: "video", codec: "mpeg2video" }),
        stream({ kind: "audio", codec: "mp2", channels: 2 }),
        stream({ kind: "subtitle", codec: "dvb_subtitle" }),
      ],
    }),
    expected: "MPEG-TS · MPEG-2 · MP2 2.0 · DVB",
  },
  {
    name: "PCM audio in MOV",
    file: mediaFile({
      container: "mov",
      streams: [
        stream({ kind: "video", codec: "prores", bitDepth: 10 }),
        stream({ kind: "audio", codec: "pcm_s24le", channels: 2 }),
      ],
    }),
    expected: "MOV · ProRes 10-bit · PCM 2.0",
  },
  {
    name: "several subtitle formats share one chip, each format once",
    file: mediaFile({
      container: "mkv",
      streams: [
        stream({ kind: "video", codec: "h264" }),
        stream({ kind: "subtitle", codec: "subrip" }),
        stream({ kind: "subtitle", codec: "ass" }),
        stream({ kind: "subtitle", codec: "subrip" }),
      ],
    }),
    expected: "MKV · H.264 · SRT/ASS",
  },
  {
    name: "the default audio track wins over the first one",
    file: mediaFile({
      container: "mkv",
      streams: [
        stream({ kind: "video", codec: "h264" }),
        stream({ kind: "audio", codec: "aac", channels: 2 }),
        stream({ kind: "audio", codec: "dts", channels: 6, default: true }),
      ],
    }),
    expected: "MKV · H.264 · DTS 5.1",
  },
  {
    name: "cover art is not the video stream",
    file: mediaFile({
      container: "mp4",
      streams: [
        stream({ kind: "video", codec: "mjpeg", attachedPicture: true }),
        stream({ kind: "audio", codec: "alac", channels: 2 }),
      ],
    }),
    expected: "MP4 · ALAC 2.0",
  },
  {
    name: "missing codec, channels and container leave only what is known",
    file: mediaFile({
      streams: [
        stream({ kind: "video", bitDepth: 10 }),
        stream({ kind: "audio", channels: 6 }),
        stream({ kind: "subtitle" }),
        stream({ kind: "other", codec: "bin_data" }),
      ],
    }),
    expected: "10-bit · 5.1",
  },
  {
    name: "a file whose probing failed has no chips",
    file: mediaFile({ available: false, errorCode: "probe_failed" }),
    expected: "",
  },
];

describe("technicalChips", () => {
  it.each(chipCases)("$name", ({ file, expected }) => {
    expect(technicalChips(file).join(chipSeparator)).toBe(expected);
  });

  it("returns one chip per part, in the plan's order", () => {
    const file = mediaFile({
      container: "mkv",
      streams: [
        stream({ kind: "subtitle", codec: "subrip" }),
        stream({ kind: "audio", codec: "ac3", channels: 6 }),
        stream({ kind: "video", codec: "hevc" }),
      ],
    });

    expect(technicalChips(file)).toEqual(["MKV", "HEVC", "AC-3 5.1", "SRT"]);
  });
});
