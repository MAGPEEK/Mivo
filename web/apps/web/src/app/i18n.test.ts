import { productName } from "@mivo/ui-tokens";
import { describe, expect, it } from "vitest";

import {
  sharedLocales,
  sharedNamespace,
  supportedLanguages,
  type NamespaceLocales,
} from "../shared/i18n";
import { createAppI18n, featureNamespaces } from "./i18n";

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

const namespaces: readonly (readonly [name: string, locales: NamespaceLocales])[] = [
  [sharedNamespace, sharedLocales],
  ...Object.entries(featureNamespaces),
];

// One row per namespace and language, e.g. "library (de)".
const localeFiles = namespaces.flatMap(([name, locales]) =>
  supportedLanguages.map((language) => ({
    file: `${name} (${language})`,
    strings: locales[language],
  })),
);

describe("locales", () => {
  it("registers the shared namespace and one namespace per feature", () => {
    expect(namespaces.map(([name]) => name).toSorted()).toEqual([
      "item",
      "library",
      "player",
      "translation",
    ]);
  });

  it.each(namespaces)(
    "has a resource file for every supported language in %s",
    (_name, locales) => {
      expect(Object.keys(locales).toSorted()).toEqual([...supportedLanguages].toSorted());
    },
  );

  it.each(namespaces)("gives German exactly the keys of English in %s", (_name, locales) => {
    expect(sortedKeys(locales.de)).toEqual(sortedKeys(locales.en));
  });

  it.each(localeFiles)("has only non-empty strings in $file", ({ strings }) => {
    expect(flatten(strings)).not.toEqual([]);
    for (const [key, value] of flatten(strings)) {
      expect.soft(typeof value === "string" && value.trim() !== "", key).toBe(true);
    }
  });

  // Translations must interpolate the configurable name instead of spelling it (ADR 0002).
  it.each(localeFiles)("never spells the product name in $file", ({ strings }) => {
    for (const [key, value] of flatten(strings)) {
      expect.soft(String(value), key).not.toContain(productName);
    }
  });
});

describe("createAppI18n", () => {
  it.each([
    { language: "en", expected: ["Home", "Library", "Item details", "Player"] },
    { language: "de", expected: ["Startseite", "Bibliothek", "Eintragsdetails", "Wiedergabe"] },
  ])("translates every namespace in $language", ({ language, expected }) => {
    const i18n = createAppI18n([language]);

    expect([
      i18n.t("home.heading"),
      i18n.t("heading", { ns: "library" }),
      i18n.t("heading", { ns: "item" }),
      i18n.t("heading", { ns: "player" }),
    ]).toEqual(expected);
  });
});
