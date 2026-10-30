import { describe, expect, it } from "vitest";

import { durationParts } from "./duration";

describe("durationParts", () => {
  it.each([
    { seconds: 5423.042, expected: { unit: "hours", hours: 1, minutes: 30 } },
    { seconds: 3600, expected: { unit: "hours", hours: 1, minutes: 0 } },
    { seconds: 3599.6, expected: { unit: "hours", hours: 1, minutes: 0 } },
    { seconds: 3599.4, expected: { unit: "minutes", minutes: 59 } },
    { seconds: 90, expected: { unit: "minutes", minutes: 1 } },
    { seconds: 60, expected: { unit: "minutes", minutes: 1 } },
    { seconds: 10.2, expected: { unit: "seconds", seconds: 10 } },
    { seconds: 0, expected: { unit: "seconds", seconds: 0 } },
    { seconds: -3, expected: { unit: "seconds", seconds: 0 } },
  ])("splits $seconds s", ({ seconds, expected }) => {
    expect(durationParts(seconds)).toEqual(expected);
  });
});
