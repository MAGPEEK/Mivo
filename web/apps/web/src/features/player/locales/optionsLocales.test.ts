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

/** Placeholders such as `{{quality}}` of a string, sorted. */
function placeholders(text: string): string[] {
  return [...text.matchAll(/\{\{(\w+)\}\}/g)].map((match) => match[1] ?? "").sort();
}

function textAt(tree: unknown, path: string): string {
  const value = path
    .split(".")
    .reduce<unknown>(
      (node, key) =>
        typeof node === "object" && node !== null ? Reflect.get(node, key) : undefined,
      tree,
    );
  return typeof value === "string" ? value : "";
}

describe("playback options locales", () => {
  const optionPaths = keyPaths(en).filter(
    (path) =>
      path.startsWith("options.") || path === "status.switching" || path === "controls.options",
  );

  it("has strings for the panel", () => {
    expect(optionPaths.length).toBeGreaterThan(40);
  });

  it("has every string of the panel in German, too, and no extra ones", () => {
    const germanPaths = keyPaths(de).filter(
      (path) =>
        path.startsWith("options.") || path === "status.switching" || path === "controls.options",
    );

    expect(germanPaths.sort()).toEqual([...optionPaths].sort());
  });

  it.each(optionPaths)("keeps the placeholders of %s in German", (path) => {
    expect(placeholders(textAt(de, path))).toEqual(placeholders(textAt(en, path)));
    expect(textAt(de, path)).not.toBe("");
  });
});
