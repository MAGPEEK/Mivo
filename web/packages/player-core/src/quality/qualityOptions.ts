// The quality choices of a playback (ADR 0030 §6): the original, and the steps of the fixed
// bit-rate ladder below the source. The server only honours the number it is given
// (`PlaybackRequest.maxBitRate`); offering and picking steps is the client's job.

import { originalQualityBitRate } from "../session/sessionRequest";

const bitsPerMegabit = 1_000_000;

/** One step of the ladder: a limit of the whole stream and the video height it is made for. */
export interface QualityStep {
  /** Bits per second, the value of `PlaybackRequest.maxBitRate`. */
  readonly maxBitRate: number;
  /** Video height in pixels the step is labelled with, e.g. 720 for "720p". */
  readonly height: number;
}

/**
 * The ladder of ADR 0030 §6, best first: 20, 12, 8, 4, 2 and 1 Mbit/s with heights 2160,
 * 1080, 1080, 720, 480 and 480. The table mirrors the server's ladder
 * (internal/domain/playback/ladder.go) and must change together with it. The heights only
 * label a step; the server decides the size it encodes.
 */
export const qualityLadder: readonly QualityStep[] = [
  { maxBitRate: 20 * bitsPerMegabit, height: 2160 },
  { maxBitRate: 12 * bitsPerMegabit, height: 1080 },
  { maxBitRate: 8 * bitsPerMegabit, height: 1080 },
  { maxBitRate: 4 * bitsPerMegabit, height: 720 },
  { maxBitRate: 2 * bitsPerMegabit, height: 480 },
  { maxBitRate: 1 * bitsPerMegabit, height: 480 },
];

/** The source as it is: no limit, so direct play stays possible. */
export interface OriginalQuality {
  readonly kind: "original";
  /** Always {@link originalQualityBitRate}. */
  readonly maxBitRate: number;
  /** Bit rate of the source in bits per second; absent when unknown. */
  readonly sourceBitRate?: number | undefined;
  /** Height of the source video in pixels; absent when unknown. */
  readonly height?: number | undefined;
}

/** A ladder step below the source; choosing it makes the server transcode. */
export interface LimitedQuality extends QualityStep {
  readonly kind: "limited";
}

/** A quality the viewer can choose. */
export type QualityOption = OriginalQuality | LimitedQuality;

function positive(value: number | undefined): number | undefined {
  return value !== undefined && Number.isFinite(value) && value > 0 ? value : undefined;
}

/**
 * The quality options for a source, best first: the original, then every ladder step whose
 * bit rate is below the source's. A step never claims more height than the source has. With
 * an unknown source bit rate every step is offered, because none can be ruled out.
 */
export function qualityOptions(
  sourceBitRate: number | undefined,
  height: number | undefined,
): QualityOption[] {
  const knownBitRate = positive(sourceBitRate);
  const knownHeight = positive(height);
  const original: OriginalQuality = {
    kind: "original",
    maxBitRate: originalQualityBitRate,
    sourceBitRate: knownBitRate,
    height: knownHeight,
  };
  const steps = qualityLadder
    .filter((step) => knownBitRate === undefined || step.maxBitRate < knownBitRate)
    .map((step): LimitedQuality => ({
      kind: "limited",
      maxBitRate: step.maxBitRate,
      height: knownHeight === undefined ? step.height : Math.min(step.height, knownHeight),
    }));
  return [original, ...steps];
}
