// "Automatic" quality (ADR 0030 §6) is a client decision: the player measures how fast
// segments arrive and asks the server for the best ladder step the connection carries. Every
// change replaces the session, so the choice is deliberately calm: down at once, up only after
// a hold-down.

import { originalQualityBitRate } from "../session/sessionRequest";
import { qualityLadder, type QualityOption } from "./qualityOptions";

const topLadderBitRate = Math.max(...qualityLadder.map((step) => step.maxBitRate));

/** Share of the measured throughput a stream may use; the rest absorbs fluctuation. */
export const throughputSafetyFactor = 0.8;

/** How long a choice must have lasted before a better one replaces it, in milliseconds. */
export const stepUpHoldDownMilliseconds = 30_000;

/** Weight of the newest sample in the moving average (EMA); the rest stays with the history. */
export const throughputSmoothingFactor = 0.3;

/**
 * Smallest download that counts as a measurement, in bytes. Playlists, init segments and
 * answers from the browser cache are too small or too fast to say anything about the line.
 */
export const minimumSampleBytes = 32_768;

const bitsPerByte = 8;
const millisecondsPerSecond = 1000;

/** One finished download of a media segment. */
export interface ThroughputSample {
  readonly bytes: number;
  /** Time from the request to the last byte, in milliseconds. */
  readonly milliseconds: number;
}

/** What automatic quality chooses from. */
export interface AutoQualityOptions {
  /** The options of `qualityOptions`, the original included. */
  readonly options: readonly QualityOption[];
  /**
   * `maxBitRate` playback starts with; the original by default, so a fast connection keeps
   * direct play and only a measured shortage costs a transcode.
   */
  readonly initialMaxBitRate?: number | undefined;
}

/** Picks the quality from measured throughput. It holds state but no timers and no clock. */
export interface AutoQuality {
  /**
   * Feeds one measurement taken at `nowMilliseconds` (any monotonic clock) and returns the
   * `maxBitRate` to play with. A value that differs from the previous one asks for a restart
   * through `planQualitySwitch`.
   */
  report(sample: ThroughputSample, nowMilliseconds: number): number;
  /** The current choice: {@link originalQualityBitRate} or the bit rate of a ladder step. */
  readonly maxBitRate: number;
  /** Smoothed throughput in bits per second; `undefined` before the first usable sample. */
  readonly throughput: number | undefined;
}

/**
 * Bits per second an option needs. The original needs what the source has. A source of
 * unknown bit rate is taken to need what the best step of the ladder needs: a line that
 * carries the whole ladder keeps direct play instead of transcoding for nothing.
 */
function requiredBitRate(option: QualityOption): number {
  if (option.kind === "limited") {
    return option.maxBitRate;
  }
  return option.sourceBitRate ?? topLadderBitRate;
}

/** Cheapest first; at the same cost the original comes last, because it needs no transcode. */
function byRequirement(first: QualityOption, second: QualityOption): number {
  const difference = requiredBitRate(first) - requiredBitRate(second);
  if (difference !== 0) {
    return difference;
  }
  return Number(first.kind === "original") - Number(second.kind === "original");
}

function isUsable(sample: ThroughputSample): boolean {
  return (
    Number.isFinite(sample.bytes) &&
    Number.isFinite(sample.milliseconds) &&
    sample.bytes >= minimumSampleBytes &&
    sample.milliseconds > 0
  );
}

/** The best option within the budget, else the cheapest one; `undefined` without options. */
function affordable(options: readonly QualityOption[], budget: number): QualityOption | undefined {
  const cheapestFirst = [...options].sort(byRequirement);
  return cheapestFirst.findLast((option) => requiredBitRate(option) <= budget) ?? cheapestFirst[0];
}

/**
 * Creates automatic quality for one playback. The throughput is an exponential moving average
 * of the samples; the budget is {@link throughputSafetyFactor} of it. A cheaper option than
 * the current one is taken at once, a better one only when the current choice has lasted
 * {@link stepUpHoldDownMilliseconds}. The first decision is free in both directions.
 */
export function createAutoQuality(settings: AutoQualityOptions): AutoQuality {
  const { options } = settings;
  let maxBitRate = settings.initialMaxBitRate ?? originalQualityBitRate;
  let throughput: number | undefined;
  let changedAt: number | undefined;

  const currentRequirement = (): number => {
    const current = options.find((option) => option.maxBitRate === maxBitRate);
    return current === undefined ? maxBitRate : requiredBitRate(current);
  };

  const decide = (smoothed: number, nowMilliseconds: number): void => {
    const target = affordable(options, smoothed * throughputSafetyFactor);
    if (target === undefined || target.maxBitRate === maxBitRate) {
      return;
    }
    const isStepUp = requiredBitRate(target) > currentRequirement();
    const held =
      changedAt !== undefined && nowMilliseconds - changedAt < stepUpHoldDownMilliseconds;
    if (isStepUp && held) {
      return;
    }
    maxBitRate = target.maxBitRate;
    changedAt = nowMilliseconds;
  };

  return {
    report(sample, nowMilliseconds) {
      if (isUsable(sample)) {
        const measured = (sample.bytes * bitsPerByte * millisecondsPerSecond) / sample.milliseconds;
        throughput =
          throughput === undefined
            ? measured
            : throughputSmoothingFactor * measured + (1 - throughputSmoothingFactor) * throughput;
        decide(throughput, nowMilliseconds);
      }
      return maxBitRate;
    },
    get maxBitRate() {
      return maxBitRate;
    },
    get throughput() {
      return throughput;
    },
  };
}
