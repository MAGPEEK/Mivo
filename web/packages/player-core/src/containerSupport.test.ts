import { describe, expect, it } from "vitest";

import {
  defaultContainerCandidates,
  detectContainerSupport,
  toSupportLevel,
  type ContainerCandidate,
  type MediaTypeProbe,
} from "./index";

function probeAnswering(answers: Readonly<Record<string, string>>): MediaTypeProbe {
  return {
    canPlayType: (mimeType) => answers[mimeType] ?? "",
  };
}

describe("toSupportLevel", () => {
  it.each([
    { answer: "probably", expected: "probably" },
    { answer: "maybe", expected: "maybe" },
    { answer: "", expected: "unsupported" },
    { answer: "no", expected: "unsupported" },
    { answer: "PROBABLY", expected: "unsupported" },
  ] as const)('maps "$answer" to $expected', ({ answer, expected }) => {
    expect(toSupportLevel(answer)).toBe(expected);
  });
});

describe("detectContainerSupport", () => {
  it("asks the probe for each candidate and keeps the candidate order", () => {
    const candidates: ContainerCandidate[] = [
      { container: "webm", mimeType: "video/webm" },
      { container: "mp4", mimeType: 'video/mp4; codecs="avc1.42E01E"' },
      { container: "hls", mimeType: "application/vnd.apple.mpegurl" },
    ];
    const probe = probeAnswering({
      "video/webm": "maybe",
      'video/mp4; codecs="avc1.42E01E"': "probably",
    });

    expect(detectContainerSupport(probe, candidates)).toEqual([
      { container: "webm", mimeType: "video/webm", level: "maybe" },
      { container: "mp4", mimeType: 'video/mp4; codecs="avc1.42E01E"', level: "probably" },
      { container: "hls", mimeType: "application/vnd.apple.mpegurl", level: "unsupported" },
    ]);
  });

  it("probes the default candidates when none are given", () => {
    const askedMimeTypes: string[] = [];
    const probe: MediaTypeProbe = {
      canPlayType: (mimeType) => {
        askedMimeTypes.push(mimeType);
        return "maybe";
      },
    };

    const support = detectContainerSupport(probe);

    expect(askedMimeTypes).toEqual(defaultContainerCandidates.map(({ mimeType }) => mimeType));
    expect(support.map(({ container }) => container)).toEqual([
      "mp4",
      "webm",
      "matroska",
      "mpegts",
      "hls",
    ]);
    expect(support.every(({ level }) => level === "maybe")).toBe(true);
  });

  it("returns an empty list for no candidates without calling the probe", () => {
    const probe: MediaTypeProbe = {
      canPlayType: () => {
        throw new Error("probe must not be called");
      },
    };

    expect(detectContainerSupport(probe, [])).toEqual([]);
  });
});
