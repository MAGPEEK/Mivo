// Clock texts of the player: elapsed and total time next to the scrubber, and the scrubber's
// `aria-valuetext`. Digits and colons read the same in every supported language.

const secondsPerMinute = 60;
const secondsPerHour = 3600;
const twoDigits = 2;

/** Whether a duration needs an hour field, so elapsed and total time use the same layout. */
export function needsHours(durationSeconds: number | undefined): boolean {
  return durationSeconds !== undefined && durationSeconds >= secondsPerHour;
}

/**
 * Formats a position as `m:ss`, or `h:mm:ss` with `withHours`. Fractions are cut, so the
 * clock ticks like the video's; `NaN`, `Infinity` and negative values read 0.
 */
export function clockTime(seconds: number, withHours: boolean): string {
  const total = Number.isFinite(seconds) && seconds > 0 ? Math.floor(seconds) : 0;
  const hours = Math.floor(total / secondsPerHour);
  const minutes = Math.floor((total % secondsPerHour) / secondsPerMinute);
  const secondsPart = String(total % secondsPerMinute).padStart(twoDigits, "0");
  if (!withHours && hours === 0) {
    return `${String(minutes)}:${secondsPart}`;
  }
  return `${String(hours)}:${String(minutes).padStart(twoDigits, "0")}:${secondsPart}`;
}

/** Position and duration as clock texts; `duration` is absent while it is unknown. */
export interface ClockTexts {
  readonly position: string;
  readonly duration: string | undefined;
}

/** Texts for the time display: both in one layout, with hours when the duration has them. */
export function displayedClock(position: number, durationSeconds: number | undefined): ClockTexts {
  const withHours = needsHours(durationSeconds);
  return {
    position: clockTime(position, withHours),
    duration: durationSeconds === undefined ? undefined : clockTime(durationSeconds, withHours),
  };
}

/** Texts for the scrubber's `aria-valuetext`: always `h:mm:ss` (task M2-30). */
export function spokenClock(position: number, durationSeconds: number | undefined): ClockTexts {
  return {
    position: clockTime(position, true),
    duration: durationSeconds === undefined ? undefined : clockTime(durationSeconds, true),
  };
}
