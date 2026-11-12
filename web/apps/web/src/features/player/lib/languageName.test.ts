import { describe, expect, it } from "vitest";

import { languageName } from "./languageName";

describe("languageName", () => {
  it.each([
    ["ger", "en", "German"],
    ["deu", "de", "Deutsch"],
    ["eng", "de", "Englisch"],
    ["fra", "en", "French"],
    ["de-AT", "en", "German"],
  ])("names %s in %s as %s", (code, locale, expected) => {
    expect(languageName(code, locale)).toBe(expected);
  });

  it.each([undefined, "", "und"])("gives no name for %j", (code) => {
    expect(languageName(code, "en")).toBeUndefined();
  });

  it("falls back to the code for a language the browser does not know", () => {
    expect(languageName("zzz", "en")?.toLowerCase()).toBe("zzz");
  });
});
