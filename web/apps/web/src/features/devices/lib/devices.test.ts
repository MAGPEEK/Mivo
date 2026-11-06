import { ApiError, clientErrorCodes, noResponseStatus } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { fakeDevices } from "../test/fakeDevicesServer";
import {
  devicesFailure,
  filterByClient,
  formatLastSeen,
  isClientFilter,
  lastSeen,
  type LastSeen,
} from "./devices";

const now = new Date("2026-11-06T12:00:00Z");

function minutesAgo(minutes: number): string {
  return new Date(now.getTime() - minutes * 60_000).toISOString();
}

function apiError(status: number, code: string): ApiError {
  return new ApiError({ status, code, title: code });
}

describe("lastSeen", () => {
  it.each<[string, string, LastSeen]>([
    ["under five minutes", minutesAgo(4), { kind: "justNow" }],
    ["in the future", minutesAgo(-3), { kind: "justNow" }],
    ["an unreadable time", "not a date", { kind: "justNow" }],
    ["minutes", minutesAgo(12), { kind: "ago", value: 12, unit: "minute" }],
    ["hours", minutesAgo(3 * 60 + 5), { kind: "ago", value: 3, unit: "hour" }],
    ["days", minutesAgo(2 * 24 * 60), { kind: "ago", value: 2, unit: "day" }],
    ["months", minutesAgo(65 * 24 * 60), { kind: "ago", value: 2, unit: "month" }],
  ])("rounds %s", (_name, lastSeenAt, expected) => {
    expect(lastSeen(lastSeenAt, now)).toEqual(expected);
  });
});

describe("formatLastSeen", () => {
  it("uses the caller's wording for just now", () => {
    expect(formatLastSeen({ kind: "justNow" }, "en", "just now")).toBe("just now");
  });

  it("formats in the given language", () => {
    const seen: LastSeen = { kind: "ago", value: 1, unit: "day" };
    expect(formatLastSeen(seen, "en", "")).toBe("yesterday");
    expect(formatLastSeen(seen, "de", "")).toBe("gestern");
  });
});

describe("devicesFailure", () => {
  it.each([
    ["notAvailable", apiError(501, "not_implemented")],
    ["forbidden", apiError(403, "forbidden")],
    ["network", apiError(noResponseStatus, clientErrorCodes.networkError)],
    ["generic", apiError(500, "internal_error")],
    ["generic", new Error("boom")],
  ])("maps to %s", (expected, error) => {
    expect(devicesFailure(error)).toBe(expected);
  });
});

describe("client filter", () => {
  it("knows its values", () => {
    expect(isClientFilter("tv")).toBe(true);
    expect(isClientFilter("dlna")).toBe(false);
  });

  it("keeps every device for all and the matching ones otherwise", () => {
    const devices = fakeDevices(now.getTime());
    expect(filterByClient(devices, "all")).toHaveLength(2);
    expect(filterByClient(devices, "tv").map((device) => device.name)).toEqual(["Living room TV"]);
    expect(filterByClient(devices, "cli")).toEqual([]);
  });
});
