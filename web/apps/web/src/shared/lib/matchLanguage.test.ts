import { describe, expect, it } from "vitest";

import { matchLanguage } from "./matchLanguage";

const supported = ["en", "de", "pt-BR"] as const;

describe("matchLanguage", () => {
  it.each([
    { preferred: ["de"], expected: "de" },
    { preferred: ["de-AT", "en"], expected: "de" },
    { preferred: ["en-US", "de"], expected: "en" },
    { preferred: ["fr-FR", "de-CH"], expected: "de" },
    { preferred: ["DE"], expected: "de" },
    { preferred: [" de "], expected: "de" },
    { preferred: ["pt-br"], expected: "pt-BR" },
    { preferred: ["fr", "es"], expected: "en" },
    { preferred: [], expected: "en" },
    { preferred: [""], expected: "en" },
  ])("returns $expected for $preferred", ({ preferred, expected }) => {
    expect(matchLanguage(preferred, supported, "en")).toBe(expected);
  });

  it("returns the fallback even when it is not the first supported language", () => {
    expect(matchLanguage(["fr"], supported, "de")).toBe("de");
  });
});
