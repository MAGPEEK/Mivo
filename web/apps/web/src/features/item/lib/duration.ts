const secondsPerMinute = 60;
const secondsPerHour = 3600;

/**
 * A duration split for display. Long media shows hours and minutes ("1 h 30 min"), shorter
 * media whole minutes, and clips below a minute their seconds; finer detail is noise on a
 * detail page.
 */
export type DurationParts =
  | { readonly unit: "hours"; readonly hours: number; readonly minutes: number }
  | { readonly unit: "minutes"; readonly minutes: number }
  | { readonly unit: "seconds"; readonly seconds: number };

/** Splits a duration in seconds for display; fractions are rounded to whole seconds first. */
export function durationParts(durationSeconds: number): DurationParts {
  const totalSeconds = Math.max(0, Math.round(durationSeconds));
  if (totalSeconds >= secondsPerHour) {
    return {
      unit: "hours",
      hours: Math.floor(totalSeconds / secondsPerHour),
      minutes: Math.floor((totalSeconds % secondsPerHour) / secondsPerMinute),
    };
  }
  if (totalSeconds >= secondsPerMinute) {
    return { unit: "minutes", minutes: Math.floor(totalSeconds / secondsPerMinute) };
  }
  return { unit: "seconds", seconds: totalSeconds };
}
