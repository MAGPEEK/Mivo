import { describe, expect, it } from "vitest";

import { mediaStream } from "../test/fakeItemServer";
import {
  audioStreamLabel,
  hdrFormatLabel,
  subtitleFormatLabel,
  subtitleStreamLabel,
  videoStreamLabel,
} from "./formatLabels";

describe("subtitleFormatLabel", () => {
  it.each([
    ["subrip", "SRT"],
    ["webvtt", "WebVTT"],
    ["mov_text", "TX3G"],
    ["hdmv_pgs_subtitle", "PGS"],
    ["dvd_subtitle", "VobSub"],
    ["dvb_subtitle", "DVB"],
    ["ass", "ASS"],
    ["constructor", "CONSTRUCTOR"],
  ])("labels %s as %s", (codec, expected) => {
    expect(subtitleFormatLabel(codec)).toBe(expected);
  });
});

describe("hdrFormatLabel", () => {
  it.each([
    ["hdr10", "HDR10"],
    ["hlg", "HLG"],
    ["dolby_vision", "Dolby Vision"],
    ["hdr10plus", "HDR10+"],
    // Formats of later API versions read as plain HDR, never as an upper-cased identifier.
    ["some_future_format", "HDR"],
    ["constructor", "HDR"],
  ])("labels %s as %s", (hdr, expected) => {
    expect(hdrFormatLabel(hdr)).toBe(expected);
  });
});

describe("stream labels", () => {
  it.each([
    { name: "codec only", fields: { codec: "h264" }, expected: "H.264" },
    { name: "8 bit stays hidden", fields: { codec: "h264", bitDepth: 8 }, expected: "H.264" },
    { name: "12 bit", fields: { codec: "av1", bitDepth: 12 }, expected: "AV1 12-bit" },
    { name: "HDR without bit depth", fields: { codec: "hevc", hdr: "hlg" }, expected: "HEVC HLG" },
    { name: "nothing known", fields: {}, expected: "" },
  ] as const)("labels video: $name", ({ fields, expected }) => {
    expect(videoStreamLabel(mediaStream({ kind: "video", ...fields }))).toBe(expected);
  });

  it.each([
    { name: "codec and channels", fields: { codec: "flac", channels: 2 }, expected: "FLAC 2.0" },
    { name: "codec only", fields: { codec: "mp3" }, expected: "MP3" },
    { name: "nothing known", fields: {}, expected: "" },
  ] as const)("labels audio: $name", ({ fields, expected }) => {
    expect(audioStreamLabel(mediaStream({ kind: "audio", ...fields }))).toBe(expected);
  });

  it("labels a subtitle stream by its format and leaves an unknown codec empty", () => {
    expect(subtitleStreamLabel(mediaStream({ kind: "subtitle", codec: "subrip" }))).toBe("SRT");
    expect(subtitleStreamLabel(mediaStream({ kind: "subtitle" }))).toBe("");
  });
});
