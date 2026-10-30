const secondsPerMinute = 60;
const secondsPerHour = 3_600;
const twoDigits = 2;

function padded(value: number): string {
  return String(value).padStart(twoDigits, "0");
}

/**
 * Clock notation of a duration, the same in every language: `9:56`, `1:30:23`. Fractions are
 * dropped. A negative or non-finite duration gives an empty string.
 */
export function formatDuration(totalSeconds: number): string {
  if (!Number.isFinite(totalSeconds) || totalSeconds < 0) {
    return "";
  }
  const wholeSeconds = Math.floor(totalSeconds);
  const hours = Math.floor(wholeSeconds / secondsPerHour);
  const minutes = Math.floor((wholeSeconds % secondsPerHour) / secondsPerMinute);
  const seconds = wholeSeconds % secondsPerMinute;
  return hours > 0
    ? `${String(hours)}:${padded(minutes)}:${padded(seconds)}`
    : `${String(minutes)}:${padded(seconds)}`;
}
