import { describe, expect, it } from "vitest";

import {
  isLanguageTag,
  languageRank,
  maxPreferredLanguages,
  normalizeLanguage,
  preferredLanguageList,
  sameLanguage,
} from "./language";

describe("normalizeLanguage", () => {
  it.each<[string | undefined, string]>([
    ["de", "deu"],
    ["ger", "deu"],
    ["deu", "deu"],
    ["DE", "deu"],
    ["de-AT", "deu"],
    ["pt_BR", "por"],
    [" en ", "eng"],
    ["iw", "heb"],
    ["nb", "nor"],
    ["nn", "nor"],
    ["no", "nor"],
    ["tlh", "tlh"],
    ["und", ""],
    ["", ""],
    [undefined, ""],
  ])("normalises %s to %s", (tag, expected) => {
    expect(normalizeLanguage(tag)).toBe(expected);
  });
});

describe("sameLanguage", () => {
  it.each<[string | undefined, string | undefined, boolean]>([
    ["de", "ger", true],
    ["en-US", "eng", true],
    ["de", "en", false],
    ["und", "und", false],
    [undefined, undefined, false],
    ["", "de", false],
  ])("%s and %s: %s", (first, second, expected) => {
    expect(sameLanguage(first, second)).toBe(expected);
  });
});

describe("languageRank", () => {
  const preferred = ["de", "en", "und"];

  it.each<[string | undefined, number]>([
    ["ger", 0],
    ["eng", 1],
    ["fra", 3],
    ["und", 3],
    [undefined, 3],
  ])("ranks %s at %i", (language, expected) => {
    expect(languageRank(language, preferred)).toBe(expected);
  });

  it("ranks everything at 0 for an empty list", () => {
    expect(languageRank("deu", [])).toBe(0);
  });
});

describe("isLanguageTag", () => {
  it.each<[string, boolean]>([
    ["de", true],
    ["deu", true],
    ["pt-BR", true],
    ["zh_Hant", true],
    ["d", false],
    ["deut", false],
    ["d3", false],
    ["de-", false],
    ["", false],
    ["de-abcdefghijklmnop", false],
  ])("%s: %s", (tag, expected) => {
    expect(isLanguageTag(tag)).toBe(expected);
  });
});

describe("preferredLanguageList", () => {
  it("keeps the order, trims, and drops invalid, undetermined and repeated languages", () => {
    expect(preferredLanguageList([" de-AT ", "de", "x", "und", "en-US", "ger", "en"])).toEqual([
      "de-AT",
      "en-US",
    ]);
  });

  it("cuts the list to what the API accepts", () => {
    const many = ["de", "en", "fr", "it", "es", "pt", "nl", "sv", "da", "fi"];
    expect(preferredLanguageList(many)).toEqual(many.slice(0, maxPreferredLanguages));
  });

  it("returns an empty list for no tags", () => {
    expect(preferredLanguageList([])).toEqual([]);
  });
});
