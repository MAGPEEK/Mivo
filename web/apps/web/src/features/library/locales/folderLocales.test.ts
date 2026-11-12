import { describe, expect, it } from "vitest";

import de from "./de.json";
import en from "./en.json";

/** Dotted paths of every string in a locale tree. */
function keyPaths(tree: unknown, prefix = ""): string[] {
  if (typeof tree === "string") {
    return [prefix];
  }
  if (typeof tree !== "object" || tree === null) {
    return [];
  }
  return Object.entries(tree).flatMap(([key, value]) =>
    keyPaths(value, prefix === "" ? key : `${prefix}.${key}`),
  );
}

describe("folder view locales", () => {
  it("has the same keys in English and German", () => {
    expect(keyPaths(de.folders).toSorted()).toEqual(keyPaths(en.folders).toSorted());
  });

  it("has no empty text", () => {
    for (const text of [...keyPaths(en.folders), ...keyPaths(de.folders)]) {
      expect(text.length).toBeGreaterThan(0);
    }
  });
});
