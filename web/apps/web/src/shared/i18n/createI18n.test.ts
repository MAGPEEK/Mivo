import { describe, expect, it } from "vitest";

import { createI18n } from "./createI18n";

describe("createI18n", () => {
  it.each([
    { preferred: ["de-DE", "en"], expected: "de" },
    { preferred: ["fr-FR", "en-GB"], expected: "en" },
    { preferred: ["fr-FR"], expected: "en" },
    { preferred: [], expected: "en" },
  ])("starts in $expected for $preferred", ({ preferred, expected }) => {
    const i18n = createI18n(preferred);

    expect(i18n.isInitialized).toBe(true);
    expect(i18n.resolvedLanguage).toBe(expected);
  });

  it("translates with the bundled resources", () => {
    expect(createI18n(["de"]).t("home.heading")).toBe("Startseite");
    expect(createI18n(["en"]).t("home.heading")).toBe("Home");
  });

  it("returns independent instances", async () => {
    const first = createI18n(["en"]);
    const second = createI18n(["en"]);

    await second.changeLanguage("de");

    expect(first.resolvedLanguage).toBe("en");
  });
});
