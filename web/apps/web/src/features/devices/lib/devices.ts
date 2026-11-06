import { ApiError, clientErrorCodes, type AccountDevice, type Device } from "@mivo/api-client";

/**
 * One row of a device table. The own list has no account; the admin list adds it. Kept as one
 * shape so both pages share the table; columns such as the TV app version (later) join here.
 */
export type DeviceRow = Device & Partial<Pick<AccountDevice, "userId" | "userName">>;

/** Why a device list could not be shown. */
export type DevicesFailure = "notAvailable" | "forbidden" | "network" | "generic";

const forbiddenStatus = 403;
const notImplementedStatus = 501;

/** Sorts a failed list or revoke request into the text the page shows. */
export function devicesFailure(error: unknown): DevicesFailure {
  if (!(error instanceof ApiError)) {
    return "generic";
  }
  if (error.status === notImplementedStatus) {
    return "notAvailable";
  }
  if (error.status === forbiddenStatus) {
    return "forbidden";
  }
  return error.code === clientErrorCodes.networkError ? "network" : "generic";
}

/** The client types the admin filter offers; "all" shows every device. */
export const clientFilters = ["all", "web", "tv", "cli"] as const;

/** A value of the client-type filter. */
export type ClientFilter = (typeof clientFilters)[number];

/** Whether `value` is one of {@link clientFilters}. */
export function isClientFilter(value: string): value is ClientFilter {
  return (clientFilters as readonly string[]).includes(value);
}

/** The devices of the chosen client type, in their original order. */
export function filterByClient<Row extends DeviceRow>(
  devices: readonly Row[],
  filter: ClientFilter,
): readonly Row[] {
  return filter === "all" ? devices : devices.filter((device) => device.client === filter);
}

const millisecondsPerMinute = 60_000;
const minutesPerHour = 60;
const hoursPerDay = 24;
const daysPerMonth = 30;
// The server tracks the last request only to about five minutes (api/openapi.yaml), so a
// finer statement than "just now" would be invented precision.
const justNowMinutes = 5;

/** How long ago a device was last seen, ready for `Intl.RelativeTimeFormat`. */
export type LastSeen =
  | { readonly kind: "justNow" }
  | { readonly kind: "ago"; readonly value: number; readonly unit: Intl.RelativeTimeFormatUnit };

/** Rounds the time between `lastSeenAt` and `now` to the coarsest fitting unit. */
export function lastSeen(lastSeenAt: string, now: Date): LastSeen {
  const minutes = Math.floor((now.getTime() - Date.parse(lastSeenAt)) / millisecondsPerMinute);
  if (Number.isNaN(minutes) || minutes < justNowMinutes) {
    return { kind: "justNow" };
  }
  if (minutes < minutesPerHour) {
    return { kind: "ago", value: minutes, unit: "minute" };
  }
  const hours = Math.floor(minutes / minutesPerHour);
  if (hours < hoursPerDay) {
    return { kind: "ago", value: hours, unit: "hour" };
  }
  const days = Math.floor(hours / hoursPerDay);
  if (days < daysPerMonth) {
    return { kind: "ago", value: days, unit: "day" };
  }
  return { kind: "ago", value: Math.floor(days / daysPerMonth), unit: "month" };
}

/** {@link lastSeen} as text in `language`; `justNow` is the caller's own wording. */
export function formatLastSeen(seen: LastSeen, language: string, justNow: string): string {
  if (seen.kind === "justNow") {
    return justNow;
  }
  return new Intl.RelativeTimeFormat(language, { numeric: "auto" }).format(-seen.value, seen.unit);
}
