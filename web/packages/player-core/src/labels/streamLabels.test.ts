import { describe, expect, it } from "vitest";

import { bitDepthLabel, channelLabel, hdrLabel, resolutionLabel } from "./streamLabels";

describe("channelLabel", () => {
  it.each([
    [1, "1.0"],
    [2, "2.0"],
    [3, "2.1"],
    [4, "4.0"],
    [5, "5.0"],
    [6, "5.1"],
    [7, "6.1"],
    [8, "7.1"],
    [10, "10"],
    [0, "0"],
  ])("labels %i channels as %s", (channels, expected) => {
    expect(channelLabel(channels)).toBe(expected);
  });
});

describe("hdrLabel", () => {
  it.each([
    ["hdr10", "HDR10"],
    ["hlg", "HLG"],
    ["future_format", "FUTURE_FORMAT"],
    [undefined, "SDR"],
  ])("labels %s as %s", (hdr, expected) => {
    expect(hdrLabel(hdr)).toBe(expected);
  });
});

describe("bitDepthLabel", () => {
  it.each([
    [8, "8-bit"],
    [10, "10-bit"],
    [12, "12-bit"],
  ])("labels %i as %s", (bitDepth, expected) => {
    expect(bitDepthLabel(bitDepth)).toBe(expected);
  });
});

describe("resolutionLabel", () => {
  it.each([
    { height: 480, expected: "480p" },
    { height: 576, expected: "576p" },
    { height: 540, expected: "540p" },
    { height: 720, expected: "720p" },
    { height: 1080, expected: "1080p" },
    { height: 1072, expected: "1080p" },
    { height: 1440, expected: "1440p" },
    { height: 2160, expected: "4K" },
    { height: 4320, expected: "8K" },
    { height: 360, expected: "360p" },
    { height: 240, expected: "240p" },
    { height: 800, expected: "720p" },
  ])("labels a height of $height as $expected", ({ height, expected }) => {
    expect(resolutionLabel(height)).toBe(expected);
  });

  it.each([
    { width: 1920, height: 1080, expected: "1080p" },
    { width: 1920, height: 800, expected: "1080p" },
    { width: 1440, height: 1080, expected: "1080p" },
    { width: 3840, height: 1600, expected: "4K" },
    { width: 3840, height: 2076, expected: "4K" },
    { width: 1280, height: 534, expected: "720p" },
    { width: 720, height: 576, expected: "576p" },
    { width: 720, height: 480, expected: "480p" },
    { width: 960, height: 540, expected: "540p" },
    { width: 640, height: 360, expected: "360p" },
    { width: 426, height: 240, expected: "240p" },
    { width: 1080, height: 1920, expected: "1080p" },
    { width: 720, height: 1280, expected: "720p" },
    { width: 2160, height: 3840, expected: "4K" },
    { width: 0, height: 1080, expected: "1080p" },
    { width: Number.NaN, height: 720, expected: "720p" },
    { width: -1920, height: 1080, expected: "1080p" },
  ])("labels $width×$height as $expected", ({ width, height, expected }) => {
    expect(resolutionLabel(height, width)).toBe(expected);
  });

  it.each([0, -720, Number.NaN, Number.POSITIVE_INFINITY])(
    "gives an empty label for a height of %d",
    (height) => {
      expect(resolutionLabel(height)).toBe("");
      expect(resolutionLabel(height, 1920)).toBe("");
    },
  );
});
