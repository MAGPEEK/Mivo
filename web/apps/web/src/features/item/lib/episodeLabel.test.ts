import { describe, expect, it } from "vitest";

import { episodeRange, isMultiEpisode } from "./episodeLabel";

describe("episodeRange", () => {
  it.each([
    { number: 3, numberEnd: undefined, expected: { first: 3, last: 3 } },
    { number: 1, numberEnd: 2, expected: { first: 1, last: 2 } },
    { number: 4, numberEnd: 4, expected: { first: 4, last: 4 } },
    { number: 4, numberEnd: 2, expected: { first: 4, last: 4 } },
    { number: undefined, numberEnd: 2, expected: undefined },
    { number: undefined, numberEnd: undefined, expected: undefined },
  ])("reads $number to $numberEnd as $expected", ({ number, numberEnd, expected }) => {
    expect(episodeRange(number, numberEnd)).toEqual(expected);
  });

  it("tells multi-episode ranges", () => {
    expect(isMultiEpisode({ first: 1, last: 2 })).toBe(true);
    expect(isMultiEpisode({ first: 1, last: 1 })).toBe(false);
  });
});
