const dateOnlyPattern = /^(\d{4})-(\d{2})-(\d{2})$/;
const monthIndexOffset = 1;

/**
 * Formats a date-only value (`YYYY-MM-DD`, the API's air date) in the UI language, e.g.
 * `May 20, 2008`. The date is a calendar day, not an instant, so it is formatted in UTC and
 * never shifts with the viewer's time zone. Anything else gives `undefined`.
 */
export function formatAirDate(value: string | undefined, language: string): string | undefined {
  const match = value === undefined ? null : dateOnlyPattern.exec(value);
  if (match === null) {
    return undefined;
  }
  const [, year = "", month = "", day = ""] = match;
  const date = new Date(Date.UTC(Number(year), Number(month) - monthIndexOffset, Number(day)));
  if (Number.isNaN(date.getTime())) {
    return undefined;
  }
  return new Intl.DateTimeFormat(language, { dateStyle: "long", timeZone: "UTC" }).format(date);
}
