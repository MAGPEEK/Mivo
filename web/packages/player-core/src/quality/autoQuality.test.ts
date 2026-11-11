import { describe, expect, it } from "vitest";

import {
  createAutoQuality,
  minimumSampleBytes,
  stepUpHoldDownMilliseconds,
  throughputSafetyFactor,
  throughputSmoothingFactor,
  type ThroughputSample,
} from "./autoQuality";
import { qualityOptions } from "./qualityOptions";

const megabit = 1_000_000;
const sourceBitRate = 10 * megabit;
const options = qualityOptions(sourceBitRate, 1080);

/** A download of one second that measures exactly `bitsPerSecond`. */
function sample(bitsPerSecond: number): ThroughputSample {
  return { bytes: bitsPerSecond / 8, milliseconds: 1000 };
}

describe("createAutoQuality", () => {
  it("starts with the original and no measurement", () => {
    const auto = createAutoQuality({ options });
    expect(auto.maxBitRate).toBe(0);
    expect(auto.throughput).toBeUndefined();
  });

  it("starts with the given limit", () => {
    expect(createAutoQuality({ options, initialMaxBitRate: 4 * megabit }).maxBitRate).toBe(
      4 * megabit,
    );
  });

  it.each<{ name: string; throughput: number; expected: number }>([
    { name: "a fast line keeps the original", throughput: 50 * megabit, expected: 0 },
    {
      name: "the original needs its bit rate after the safety factor",
      throughput: sourceBitRate / throughputSafetyFactor,
      expected: 0,
    },
    {
      name: "just below takes the best step that fits",
      throughput: 12 * megabit,
      expected: 8 * megabit,
    },
    {
      name: "the safety factor rules a step out",
      throughput: 9.9 * megabit,
      expected: 4 * megabit,
    },
    { name: "a slow line", throughput: 3 * megabit, expected: 2 * megabit },
    {
      name: "below the ladder takes the cheapest step",
      throughput: 0.5 * megabit,
      expected: 1 * megabit,
    },
  ])("$name", ({ throughput, expected }) => {
    const auto = createAutoQuality({ options });
    expect(auto.report(sample(throughput), 0)).toBe(expected);
    expect(auto.maxBitRate).toBe(expected);
    expect(auto.throughput).toBe(throughput);
  });

  it("smooths the throughput with a moving average", () => {
    const auto = createAutoQuality({ options });
    auto.report(sample(10 * megabit), 0);
    auto.report(sample(20 * megabit), 1000);
    expect(auto.throughput).toBeCloseTo(
      throughputSmoothingFactor * 20 * megabit + (1 - throughputSmoothingFactor) * 10 * megabit,
    );
  });

  it.each<{ name: string; sample: ThroughputSample }>([
    { name: "too small", sample: { bytes: minimumSampleBytes - 1, milliseconds: 1 } },
    { name: "no duration", sample: { bytes: 1_000_000, milliseconds: 0 } },
    { name: "a negative duration", sample: { bytes: 1_000_000, milliseconds: -5 } },
    { name: "no number of bytes", sample: { bytes: Number.NaN, milliseconds: 100 } },
    { name: "no number of milliseconds", sample: { bytes: 1_000_000, milliseconds: Number.NaN } },
  ])("ignores a sample with $name", ({ sample: unusable }) => {
    const auto = createAutoQuality({ options, initialMaxBitRate: 4 * megabit });
    expect(auto.report(unusable, 0)).toBe(4 * megabit);
    expect(auto.throughput).toBeUndefined();
  });

  it("steps down at once and up only after the hold-down", () => {
    const auto = createAutoQuality({ options });
    expect(auto.report(sample(3 * megabit), 0)).toBe(2 * megabit);

    // The line recovers; the average needs a few samples, and then the hold-down still applies.
    for (let second = 1; second <= 20; second += 1) {
      auto.report(sample(50 * megabit), second * 1000);
    }
    expect(auto.maxBitRate).toBe(2 * megabit);
    expect(auto.report(sample(50 * megabit), stepUpHoldDownMilliseconds - 1)).toBe(2 * megabit);

    expect(auto.report(sample(50 * megabit), stepUpHoldDownMilliseconds)).toBe(0);
  });

  it("restarts the hold-down with every change", () => {
    const auto = createAutoQuality({ options, initialMaxBitRate: 8 * megabit });
    expect(auto.report(sample(6 * megabit), 0)).toBe(4 * megabit);
    expect(auto.report(sample(0.5 * megabit), 20_000)).toBe(2 * megabit);
    for (let step = 0; step < 30; step += 1) {
      auto.report(sample(50 * megabit), 20_000 + step);
    }
    // 30 s after the first change, but only 10 s after the second one.
    expect(auto.report(sample(50 * megabit), stepUpHoldDownMilliseconds)).toBe(2 * megabit);
    expect(auto.report(sample(50 * megabit), 20_000 + stepUpHoldDownMilliseconds)).toBe(0);
  });

  it("lets the first decision go up without a hold-down", () => {
    const auto = createAutoQuality({ options, initialMaxBitRate: 1 * megabit });
    expect(auto.report(sample(50 * megabit), 0)).toBe(0);
  });

  it("keeps direct play for a source of unknown bit rate when the line carries the ladder", () => {
    const unknown = qualityOptions(undefined, 1080);
    expect(createAutoQuality({ options: unknown }).report(sample(30 * megabit), 0)).toBe(0);
    expect(createAutoQuality({ options: unknown }).report(sample(20 * megabit), 0)).toBe(
      12 * megabit,
    );
  });

  it("keeps a limit that is not among the options until a decision replaces it", () => {
    const auto = createAutoQuality({ options, initialMaxBitRate: 3 * megabit });
    expect(auto.report(sample(3 * megabit), 0)).toBe(2 * megabit);
  });

  it("changes nothing without options", () => {
    const auto = createAutoQuality({ options: [], initialMaxBitRate: 4 * megabit });
    expect(auto.report(sample(50 * megabit), 0)).toBe(4 * megabit);
  });
});
