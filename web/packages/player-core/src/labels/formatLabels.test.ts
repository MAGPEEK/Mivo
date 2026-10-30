import { describe, expect, it } from "vitest";

import { codecLabel, containerLabel } from "./formatLabels";

describe("containerLabel", () => {
  it.each([
    ["mkv", "MKV"],
    ["mp4", "MP4"],
    ["m4v", "M4V"],
    ["mov", "MOV"],
    ["3gp", "3GP"],
    ["avi", "AVI"],
    ["webm", "WebM"],
    ["ogg", "Ogg"],
    ["mpegts", "MPEG-TS"],
    ["m2ts", "M2TS"],
    ["mpeg", "MPEG-PS"],
    ["rm", "RealMedia"],
    ["newformat", "NEWFORMAT"],
    ["constructor", "CONSTRUCTOR"],
  ])("labels %s as %s", (container, expected) => {
    expect(containerLabel(container)).toBe(expected);
  });
});

describe("codecLabel", () => {
  it.each([
    ["h264", "H.264"],
    ["h263", "H.263"],
    ["hevc", "HEVC"],
    ["av1", "AV1"],
    ["vp8", "VP8"],
    ["vp9", "VP9"],
    ["mpeg1video", "MPEG-1"],
    ["mpeg2video", "MPEG-2"],
    ["mpeg4", "MPEG-4"],
    ["vc1", "VC-1"],
    ["theora", "Theora"],
    ["prores", "ProRes"],
    ["aac", "AAC"],
    ["mp3", "MP3"],
    ["opus", "Opus"],
    ["vorbis", "Vorbis"],
    ["flac", "FLAC"],
    ["ac3", "AC-3"],
    ["eac3", "E-AC-3"],
    ["dts", "DTS"],
    ["truehd", "TrueHD"],
    ["alac", "ALAC"],
    ["wmav2", "WMA"],
    ["pcm_s16le", "PCM"],
    ["pcm_s24be", "PCM"],
    ["pcm_f32le", "PCM"],
    ["wmv3", "WMV3"],
    ["toString", "TOSTRING"],
  ])("labels %s as %s", (codec, expected) => {
    expect(codecLabel(codec)).toBe(expected);
  });
});
