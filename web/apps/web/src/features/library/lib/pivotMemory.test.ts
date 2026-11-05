import { describe, expect, it } from "vitest";

import {
  defaultPivot,
  pivotStorageKey,
  readRememberedPivot,
  rememberPivot,
  resolvePivot,
} from "./pivotMemory";

const libraryId = "lib-1";

// A storage whose every access fails, as in a privacy mode or with a full quota.
function failingStorage(): Storage {
  const fail = (): never => {
    throw new Error("blocked");
  };
  return {
    length: 0,
    clear: fail,
    getItem: fail,
    key: fail,
    removeItem: fail,
    setItem: fail,
  };
}

describe("pivot memory", () => {
  it("remembers the pivot per library", () => {
    rememberPivot(window.localStorage, libraryId, "overview");

    expect(readRememberedPivot(window.localStorage, libraryId)).toBe("overview");
    expect(readRememberedPivot(window.localStorage, "lib-2")).toBeUndefined();
    expect(window.localStorage.getItem(pivotStorageKey(libraryId))).toBe("overview");
  });

  it("ignores unknown stored values", () => {
    window.localStorage.setItem(pivotStorageKey(libraryId), "grid");

    expect(readRememberedPivot(window.localStorage, libraryId)).toBeUndefined();
  });

  it("works without storage and when storage fails", () => {
    expect(readRememberedPivot(undefined, libraryId)).toBeUndefined();
    expect(() => {
      rememberPivot(undefined, libraryId, "browse");
    }).not.toThrow();
    expect(readRememberedPivot(failingStorage(), libraryId)).toBeUndefined();
    expect(() => {
      rememberPivot(failingStorage(), libraryId, "browse");
    }).not.toThrow();
  });

  it.each([
    ["the URL first", "overview", "browse", "overview"],
    ["the remembered pivot without one in the URL", undefined, "overview", "overview"],
    ["Browse for a new library", undefined, undefined, "browse"],
  ] as const)("resolves %s", (_, requested, remembered, expected) => {
    expect(resolvePivot(requested, remembered)).toBe(expected);
  });

  it("opens a new library on Browse", () => {
    expect(defaultPivot).toBe("browse");
  });
});
