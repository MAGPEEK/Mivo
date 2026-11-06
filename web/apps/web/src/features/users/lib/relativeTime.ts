const secondMs = 1000;
const minuteMs = 60 * secondMs;
const hourMs = 60 * minuteMs;
const dayMs = 24 * hourMs;
const weekMs = 7 * dayMs;
const monthMs = 30 * dayMs;
const yearMs = 365 * dayMs;

// Largest unit first; a span uses the first unit it fills at least once.
const units: readonly (readonly [Intl.RelativeTimeFormatUnit, number])[] = [
  ["year", yearMs],
  ["month", monthMs],
  ["week", weekMs],
  ["day", dayMs],
  ["hour", hourMs],
  ["minute", minuteMs],
];

/**
 * "5 minutes ago", "yesterday" in `language`. The server knows the last activity only to
 * about 5 minutes, so anything below a minute reads "now".
 */
export function formatRelativeTime(when: Date, now: Date, language: string): string {
  const format = new Intl.RelativeTimeFormat(language, { numeric: "auto" });
  const elapsed = when.getTime() - now.getTime();
  for (const [unit, size] of units) {
    if (Math.abs(elapsed) >= size) {
      return format.format(Math.round(elapsed / size), unit);
    }
  }
  return format.format(0, "second");
}
