const secondsPerMinute = 60;
const millisecondsPerSecond = 1000;

/** Seconds as `m:ss`, e.g. `1:05`, the way the throttling countdown shows them. */
export function formatCountdown(totalSeconds: number): string {
  const seconds = Math.max(0, Math.ceil(totalSeconds));
  const minutes = Math.floor(seconds / secondsPerMinute);
  const rest = seconds % secondsPerMinute;
  return `${String(minutes)}:${String(rest).padStart(2, "0")}`;
}

/** Whole seconds from `now` until `deadline` (both in milliseconds), never below zero. */
export function secondsUntil(deadline: number, now: number): number {
  return Math.max(0, Math.ceil((deadline - now) / millisecondsPerSecond));
}

/** The deadline `seconds` from `now`, in milliseconds. */
export function deadlineAfter(seconds: number, now: number): number {
  return now + seconds * millisecondsPerSecond;
}
