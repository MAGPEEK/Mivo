import { describe, expect, it } from "vitest";

import { createI18n } from "../../../shared/i18n";
import { durationText } from "../hooks/durationText";
import { itemLocales, itemNamespace } from "../locales";
import { runtimeLabel } from "./runtimeLabel";

// The real locale files, so the table also pins the en and de unit spellings.
function labelIn(language: string, seconds: number | undefined): string | undefined {
  const i18n = createI18n([language], { [itemNamespace]: itemLocales });
  const t = i18n.getFixedT(language, itemNamespace);
  return runtimeLabel(seconds, (parts) => durationText(parts, t));
}

describe("runtimeLabel", () => {
  it.each([
    { language: "en", seconds: 5940, expected: "1 h 39 min" },
    { language: "de", seconds: 5940, expected: "1 Std. 39 Min." },
    { language: "en", seconds: 7200, expected: "2 h 0 min" },
    { language: "en", seconds: 2820.4, expected: "47 min" },
    { language: "de", seconds: 2820.4, expected: "47 Min." },
    { language: "en", seconds: 42, expected: "42 s" },
    { language: "de", seconds: 42, expected: "42 Sek." },
    { language: "en", seconds: 0.4, expected: undefined },
    { language: "en", seconds: 0, expected: undefined },
    { language: "en", seconds: -5, expected: undefined },
    { language: "en", seconds: Number.NaN, expected: undefined },
    { language: "en", seconds: undefined, expected: undefined },
  ])("labels $seconds s in $language as $expected", ({ language, seconds, expected }) => {
    expect(labelIn(language, seconds)).toBe(expected);
  });
});
