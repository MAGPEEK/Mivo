import { ApiError } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { fakeLibrary, finishedScan, runningScan } from "../../../test/libraryFixtures";
import {
  isScanRunning,
  libraryItemsQueryKey,
  libraryListQueryKey,
  libraryQueryKey,
  librariesQueryKey,
  shouldRetryLibraryQuery,
  withScanStatus,
} from "./libraryQueries";

function apiError(status: number): ApiError {
  return new ApiError({ status, title: "title", code: "code" });
}

describe("library query keys", () => {
  it("share the libraries prefix, so one invalidation refreshes them all", () => {
    for (const key of [libraryListQueryKey, libraryQueryKey("a"), libraryItemsQueryKey("a")]) {
      expect(key.slice(0, librariesQueryKey.length)).toEqual([...librariesQueryKey]);
    }
    expect(libraryItemsQueryKey("a").slice(0, 3)).toEqual([...libraryQueryKey("a")]);
  });
});

describe("shouldRetryLibraryQuery", () => {
  it.each([
    { name: "a server error, first failure", error: apiError(500), failures: 0, expected: true },
    { name: "a server error, second failure", error: apiError(503), failures: 1, expected: false },
    { name: "a network error", error: apiError(0), failures: 0, expected: true },
    { name: "an unknown library", error: apiError(404), failures: 0, expected: false },
    { name: "an invalid cursor", error: apiError(400), failures: 0, expected: false },
    { name: "a plain error", error: new Error("boom"), failures: 0, expected: true },
  ])("decides for $name", ({ error, failures, expected }) => {
    expect(shouldRetryLibraryQuery(failures, error)).toBe(expected);
  });
});

describe("withScanStatus", () => {
  it("replaces the scan status of a cached library", () => {
    const library = fakeLibrary();

    expect(withScanStatus(library, runningScan)).toEqual({ ...library, scan: runningScan });
  });

  it("keeps an absent library absent", () => {
    expect(withScanStatus(undefined, runningScan)).toBeUndefined();
  });
});

describe("isScanRunning", () => {
  it.each([
    { scan: runningScan, expected: true },
    { scan: finishedScan, expected: false },
    { scan: undefined, expected: false },
  ])("is $expected for $scan.state", ({ scan, expected }) => {
    expect(isScanRunning(scan)).toBe(expected);
  });
});
