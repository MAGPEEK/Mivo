import { describe, expect, it } from "vitest";

import { streamPartsPrefix, transferSamples, type ResourceEntry } from "./resourceTransfers";

const base = "http://localhost/api/v1/playback/sessions/s1/hls/";

function entry(name: string, encodedBodySize: number, duration: number): ResourceEntry {
  return { name, encodedBodySize, duration };
}

describe("streamPartsPrefix", () => {
  it.each([
    {
      name: "HLS playlist directory",
      url: "/api/v1/playback/sessions/s1/hls/index.m3u8",
      expected: base,
    },
    {
      name: "direct play file",
      url: "/api/v1/playback/sessions/s1/file",
      expected: "http://localhost/api/v1/playback/sessions/s1/",
    },
  ])("is the directory of the $name", ({ url, expected }) => {
    expect(streamPartsPrefix(url, "http://localhost/play")).toBe(expected);
  });
});

describe("transferSamples", () => {
  it("keeps only transfers under the prefix, in milliseconds converted to seconds", () => {
    const samples = transferSamples(
      [
        entry(`${base}seg-1.m4s`, 1000, 500),
        entry("http://localhost/api/v1/items", 50, 20),
        entry(`${base}seg-2.m4s`, 2000, 250),
      ],
      base,
    );

    expect(samples).toEqual([
      { bytes: 1000, durationSeconds: 0.5 },
      { bytes: 2000, durationSeconds: 0.25 },
    ]);
  });

  it("returns nothing for an empty list", () => {
    expect(transferSamples([], base)).toEqual([]);
  });
});
