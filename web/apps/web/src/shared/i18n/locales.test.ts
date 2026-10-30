import { productName } from "@mivo/ui-tokens";
import { describe, expect, it } from "vitest";

import { supportedLanguages } from "./languages";
import de from "./locales/de.json";
import en from "./locales/en.json";

type LocaleEntry = readonly [key: string, value: unknown];

// Flattens nested resources into dotted keys ("home.heading") with their leaf values.
function flatten(resource: object, prefix = ""): LocaleEntry[] {
  return Object.entries(resource).flatMap(([key, value]: [string, unknown]) => {
    const path = prefix === "" ? key : `${prefix}.${key}`;
    return typeof value === "object" && value !== null && !Array.isArray(value)
      ? flatten(value, path)
      : [[path, value] as const];
  });
}

// Sorted, so the key order inside the JSON files does not matter.
function sortedKeys(resource: object): string[] {
  return flatten(resource)
    .map(([key]) => key)
    .toSorted();
}

const locales = { en, de } as const;

describe("locales", () => {
  it("has a resource file for every supported language", () => {
    expect(Object.keys(locales).toSorted()).toEqual([...supportedLanguages].toSorted());
  });

  it("gives German exactly the keys of English", () => {
    expect(sortedKeys(de)).toEqual(sortedKeys(en));
  });

  it.each(Object.entries(locales))("has only non-empty strings in %s", (_language, locale) => {
    for (const [key, value] of flatten(locale)) {
      expect.soft(typeof value === "string" && value.trim() !== "", key).toBe(true);
    }
  });

  // Translations must interpolate the configurable name instead of spelling it (ADR 0002).
  it.each(Object.entries(locales))("never spells the product name in %s", (_language, locale) => {
    for (const [key, value] of flatten(locale)) {
      expect.soft(String(value), key).not.toContain(productName);
    }
  });
});
