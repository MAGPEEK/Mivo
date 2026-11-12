import { beforeEach, describe, expect, it } from "vitest";

import { readScroll, scrollMemoryStorageKey, writeScroll } from "./scrollMemory";

describe("scroll memory", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("returns the position of the same listing", () => {
    writeScroll(window.sessionStorage, { key: "lib|q", top: 1234 });
    expect(readScroll(window.sessionStorage, "lib|q")).toBe(1234);
  });

  it("forgets the position when another listing is opened", () => {
    writeScroll(window.sessionStorage, { key: "lib|q", top: 1234 });
    expect(readScroll(window.sessionStorage, "lib|other")).toBeUndefined();
  });

  it.each([
    ["nothing stored", null],
    ["not JSON", "{oops"],
    ["a number", "5"],
    ["a negative position", JSON.stringify({ key: "k", top: -1 })],
    ["a text position", JSON.stringify({ key: "k", top: "9" })],
  ])("ignores %s", (_name, stored) => {
    if (stored !== null) {
      window.sessionStorage.setItem(scrollMemoryStorageKey, stored);
    }
    expect(readScroll(window.sessionStorage, "k")).toBeUndefined();
  });

  it("copes without storage", () => {
    expect(() => {
      writeScroll(undefined, { key: "k", top: 1 });
    }).not.toThrow();
    expect(readScroll(undefined, "k")).toBeUndefined();
  });
});
