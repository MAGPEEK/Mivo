import { describe, expect, it } from "vitest";

import { episodeCodeMessage, episodeRange, isMultiEpisode } from "./episodeCode";

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

describe("episodeCodeMessage", () => {
  it.each([
    {
      numbers: { season: 3, number: 9 },
      expected: { key: "episodeCode.single", values: { season: 3, number: 9 } },
    },
    {
      numbers: { season: 0, number: 1 },
      expected: { key: "episodeCode.single", values: { season: 0, number: 1 } },
    },
    {
      numbers: { season: 1, number: 1, numberEnd: 2 },
      expected: { key: "episodeCode.range", values: { season: 1, first: 1, last: 2 } },
    },
    { numbers: { season: undefined, number: 4 }, expected: undefined },
    { numbers: { season: 2, number: undefined }, expected: undefined },
  ])("formats $numbers", ({ numbers, expected }) => {
    expect(episodeCodeMessage(numbers)).toEqual(expected);
  });
});
