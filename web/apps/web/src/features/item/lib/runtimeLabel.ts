import { durationParts, type DurationParts } from "./duration";

/** Translates a split duration, e.g. `1 h 39 min` or `1 Std. 39 Min.`. */
export type DurationFormatter = (parts: DurationParts) => string;

/**
 * The runtime shown in the title block, e.g. `1 h 39 min`; `undefined` when the length is
 * unknown or rounds to nothing, so the page hides the fact instead of showing `0 s`.
 */
export function runtimeLabel(
  durationSeconds: number | undefined,
  format: DurationFormatter,
): string | undefined {
  if (durationSeconds === undefined || !Number.isFinite(durationSeconds)) {
    return undefined;
  }
  const parts = durationParts(durationSeconds);
  if (parts.unit === "seconds" && parts.seconds === 0) {
    return undefined;
  }
  return format(parts);
}
