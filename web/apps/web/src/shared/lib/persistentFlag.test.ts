import { afterEach, describe, expect, it, vi } from "vitest";

import { browserLocalStorage, readFlag, writeFlag } from "./persistentFlag";

const key = "test.flag";

/** A storage whose every access fails, like Safari with site data blocked. */
function failingStorage(): Storage {
  const fail = () => {
    throw new DOMException("blocked", "SecurityError");
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

describe("readFlag", () => {
  it.each([
    { stored: "true", fallback: false, expected: true },
    { stored: "false", fallback: true, expected: false },
    { stored: "yes", fallback: true, expected: true },
    { stored: "", fallback: false, expected: false },
  ])("reads $stored as $expected (fallback $fallback)", ({ stored, fallback, expected }) => {
    window.localStorage.setItem(key, stored);

    expect(readFlag(window.localStorage, key, fallback)).toBe(expected);
  });

  it("falls back when the key is missing", () => {
    expect(readFlag(window.localStorage, key, true)).toBe(true);
  });

  it("falls back without storage", () => {
    expect(readFlag(undefined, key, true)).toBe(true);
  });

  it("falls back when reading fails", () => {
    expect(readFlag(failingStorage(), key, true)).toBe(true);
  });
});

describe("writeFlag", () => {
  it("round-trips through readFlag", () => {
    writeFlag(window.localStorage, key, true);
    expect(readFlag(window.localStorage, key, false)).toBe(true);

    writeFlag(window.localStorage, key, false);
    expect(readFlag(window.localStorage, key, true)).toBe(false);
  });

  it("ignores a failing or missing storage", () => {
    expect(() => {
      writeFlag(failingStorage(), key, true);
      writeFlag(undefined, key, true);
    }).not.toThrow();
  });
});

describe("browserLocalStorage", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns localStorage when the browser allows it", () => {
    expect(browserLocalStorage()).toBe(window.localStorage);
  });

  it("returns undefined when accessing localStorage throws", () => {
    vi.spyOn(window, "localStorage", "get").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });

    expect(browserLocalStorage()).toBeUndefined();
  });
});
