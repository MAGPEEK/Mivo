import { ApiError, clientErrorCodes } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { fakeItem } from "../../../test/libraryFixtures";
import { errorCodeOf, errorKeyForCode } from "./errorKeys";
import { formatDuration } from "./formatDuration";
import { cardArtwork, cardEpisodeNumbers, toItemCardModel } from "./itemCard";
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
  it("derives year, poster, duration, chip and availability of a movie", () => {
    const poster = { id: "poster-1", tag: "tag-1" };
    const item = fakeItem({ title: "big buck bunny", year: 2008, poster });

    expect(toItemCardModel(item)).toEqual({
      id: item.id,
      title: "big buck bunny",
      isShow: false,
      year: "2008",
      poster,
      seasonCount: 0,
      duration: "9:56",
      chip: "MKV · H.264 · AAC",
      available: true,
    });
  });

  it("gives a show its season count and keeps it available without media", () => {
    const { id, libraryId } = fakeItem();
    const show = { id, libraryId, kind: "show", title: "Show", childCount: 3 } as const;

    expect(toItemCardModel(show)).toMatchObject({
      isShow: true,
      year: "",
      poster: undefined,
      seasonCount: 3,
      duration: "",
      chip: "",
      available: true,
    });
  });

  it("treats an item without media summary as unavailable", () => {
    const { id, libraryId, kind, title } = fakeItem();
    const withoutMedia = { id, libraryId, kind, title };

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

describe("cardEpisodeNumbers", () => {
  it.each([
    {
      name: "an episode with its season",
      item: { kind: "episode", number: 9 },
      season: 3,
      expected: { season: 3, number: 9, numberEnd: undefined },
    },
    {
      name: "a double episode",
      item: { kind: "episode", number: 1, numberEnd: 2 },
      season: 1,
      expected: { season: 1, number: 1, numberEnd: 2 },
    },
    {
      name: "an episode without a known season",
      item: { kind: "episode", number: 4 },
      season: undefined,
      expected: { season: undefined, number: 4, numberEnd: undefined },
    },
    { name: "a movie", item: { kind: "movie", number: 2 }, season: 1, expected: undefined },
    { name: "a season", item: { kind: "season", number: 2 }, season: 2, expected: undefined },
  ] as const)("reads $name", ({ item, season, expected }) => {
    expect(cardEpisodeNumbers(fakeItem(item), season)).toEqual(expected);
  });
});

describe("cardArtwork", () => {
  const poster = { id: "poster-1", tag: "tag-1" };
  const thumb = { id: "thumb-1", tag: "tag-2" };
  it.each([
    { format: "poster", item: { poster, thumb }, expected: { imageRef: poster, aspect: "poster" } },
    { format: "square", item: { poster, thumb }, expected: { imageRef: poster, aspect: "square" } },
    { format: "episode", item: { poster, thumb }, expected: { imageRef: thumb, aspect: "still" } },
    { format: "episode", item: { poster }, expected: { imageRef: poster, aspect: "still" } },
    { format: "poster", item: {}, expected: { imageRef: undefined, aspect: "poster" } },
  ] as const)("picks the $format artwork of $item", ({ format, item, expected }) => {
    expect(cardArtwork(fakeItem(item), format)).toEqual(expected);
  });
});
