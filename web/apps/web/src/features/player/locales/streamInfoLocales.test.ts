import { streamInfoTranslationKeys } from "@mivo/player-core";
import { describe, expect, it } from "vitest";

import { createAppI18n } from "../../../app/i18n";
import { supportedLanguages } from "../../../shared/i18n";
import { playerNamespace } from "./index";

describe("stream-info translations", () => {
  it.each(supportedLanguages)("translate every key player-core exports in %s", (language) => {
    const i18n = createAppI18n([language]);

    for (const key of streamInfoTranslationKeys) {
      // getResource reads the language itself, without falling back to English.
      expect
        .soft(i18n.getResource(language, playerNamespace, key), key)
        .toEqual(expect.any(String));
    }
  });

  it("formats bit rates for the language", () => {
    const english = createAppI18n(["en"]);
    const german = createAppI18n(["de"]);
    const params = { value: 4.1, ns: playerNamespace } as const;

    expect(english.t("streamInfo.bitRate.megabits", params)).toBe("4.1 Mbit/s");
    expect(german.t("streamInfo.bitRate.megabits", params)).toBe("4,1 Mbit/s");
  });
});
