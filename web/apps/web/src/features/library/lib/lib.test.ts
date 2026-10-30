import { ApiError, clientErrorCodes } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { fakeItem } from "../../../test/libraryFixtures";
import { errorCodeOf, errorKeyForCode } from "./errorKeys";
import { formatDuration } from "./formatDuration";
import { toItemCardModel } from "./itemCard";
import { mediaChipLabel } from "./mediaChip";

describe("formatDuration", () => {
  it.each([
    { seconds: 0, expected: "0:00" },
    { seconds: 5.999, expected: "0:05" },
    { seconds: 596.458, expected: "9:56" },
    { seconds: 3_600, expected: "1:00:00" },
    { seconds: 5_423.042, expected: "1:30:23" },
    { seconds: 36_061, expected: "10:01:01" },
    { seconds: -1, expected: "" },
    { seconds: Number.NaN, expected: "" },
    { seconds: Number.POSITIVE_INFINITY, expected: "" },
  ])("formats $seconds s as '$expected'", ({ seconds, expected }) => {
    expect(formatDuration(seconds)).toBe(expected);
  });
});

describe("mediaChipLabel", () => {
  it.each([
    {
      name: "container, video and audio",
      media: { available: true, container: "mkv", videoCodec: "h264", audioCodec: "aac" },
      expected: "MKV · H.264 · AAC",
    },
    {
      name: "a file without audio",
      media: { available: true, container: "webm", videoCodec: "vp9" },
      expected: "WebM · VP9",
    },
    {
      name: "only the codecs",
      media: { available: true, videoCodec: "hevc", audioCodec: "pcm_s16le" },
      expected: "HEVC · PCM",
    },
    { name: "an unprobed file", media: { available: false }, expected: "" },
    { name: "no media summary", media: undefined, expected: "" },
  ])("labels $name", ({ media, expected }) => {
    expect(mediaChipLabel(media)).toBe(expected);
  });
});

describe("toItemCardModel", () => {
  it("derives initial, duration, chip and availability", () => {
    const item = fakeItem({ title: "big buck bunny" });

    expect(toItemCardModel(item)).toEqual({
      id: item.id,
      title: "big buck bunny",
      initial: "B",
      duration: "9:56",
      chip: "MKV · H.264 · AAC",
      available: true,
    });
  });

  it("keeps a whole code point as the initial", () => {
    expect(toItemCardModel(fakeItem({ title: "  𝒜lpha" })).initial).toBe("𝒜");
  });

  it("has no initial for an empty title", () => {
    expect(toItemCardModel(fakeItem({ title: " " })).initial).toBe("");
  });

  it("treats an item without media summary as unavailable", () => {
    const { id, libraryId, title } = fakeItem();
    const withoutMedia = { id, libraryId, title };

    expect(toItemCardModel(withoutMedia)).toMatchObject({
      duration: "",
      chip: "",
      available: false,
    });
  });

  it("shows no duration when the probe did not report one", () => {
    const item = fakeItem({ media: { available: false, container: "avi" } });

    expect(toItemCardModel(item)).toMatchObject({ duration: "", chip: "AVI", available: false });
  });
});

describe("errorKeyForCode", () => {
  it.each([
    { code: "root_unavailable", expected: "errors.rootUnavailable" },
    { code: "prober_unavailable", expected: "errors.proberUnavailable" },
    { code: "catalog_failed", expected: "errors.catalogFailed" },
    { code: "scan_canceled", expected: "errors.scanCanceled" },
    { code: "scan_failed", expected: "errors.scanFailed" },
    { code: "probe_failed", expected: "errors.probeFailed" },
    { code: "probe_timed_out", expected: "errors.probeTimedOut" },
    { code: "not_found", expected: "errors.notFound" },
    { code: "unavailable", expected: "errors.unavailable" },
    { code: clientErrorCodes.networkError, expected: "errors.networkError" },
    { code: "disk_on_fire", expected: "errors.generic" },
    { code: "constructor", expected: "errors.generic" },
    { code: undefined, expected: "errors.generic" },
  ])("maps $code to $expected", ({ code, expected }) => {
    expect(errorKeyForCode(code)).toBe(expected);
  });
});

describe("errorCodeOf", () => {
  it("returns the code of an ApiError", () => {
    const error = new ApiError({ status: 404, title: "Not Found", code: "not_found" });

    expect(errorCodeOf(error)).toBe("not_found");
  });

  it("returns undefined for any other error", () => {
    expect(errorCodeOf(new Error("boom"))).toBeUndefined();
  });
});
