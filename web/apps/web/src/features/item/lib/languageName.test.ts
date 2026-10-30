import { describe, expect, it } from "vitest";

import { languageName } from "./languageName";

describe("languageName", () => {
  it.each([
    { code: "eng", uiLanguage: "en", expected: "English" },
    { code: "ger", uiLanguage: "en", expected: "German" },
    { code: "ger", uiLanguage: "de", expected: "Deutsch" },
    { code: "fre", uiLanguage: "de", expected: "Französisch" },
    { code: "pt-BR", uiLanguage: "en", expected: "Brazilian Portuguese" },
    // Private-use codes have no name in Intl; languageName falls back to the code itself.
    { code: "qaa", uiLanguage: "en", expected: "qaa" },
    // No BCP 47 language tag at all: Intl throws, the code is shown as it is.
    { code: "x", uiLanguage: "en", expected: "x" },
    { code: "e1", uiLanguage: "de", expected: "e1" },
  ])("names $code in $uiLanguage as $expected", ({ code, uiLanguage, expected }) => {
    expect(languageName(code, uiLanguage)).toBe(expected);
  });

  it.each([undefined, "", "und"])("treats %j as an unknown language", (code) => {
    expect(languageName(code, "en")).toBeUndefined();
  });
});
