import { streamInfo, streamInfoTranslationKeys, type PlaybackSession } from "@mivo/player-core";
import { describe, expect, it } from "vitest";

import { createAppI18n } from "../../../app/i18n";
import { supportedLanguages } from "../../../shared/i18n";
import { remuxSession, transcodeSession } from "../test/playerFixtures";
import { playerNamespace } from "./index";

/** A word that names a playback mode; a reason or warning must not claim one (task M2-41). */
const modeWord = /remux|transcode/i;

/** The reasons of `session` as the stream-info overlay renders them in `language`. */
function renderedReasons(session: PlaybackSession, language: string): string[] {
  const i18n = createAppI18n([language]);
  return streamInfo(session).reasons.map((reason) =>
    i18n.t(reason.key, { ns: playerNamespace, replace: reason.params }),
  );
}

/** Every reason and warning text of `language` as written in its locale file. */
function reasonAndWarningTexts(language: string): unknown[] {
  const i18n = createAppI18n([language]);
  return streamInfoTranslationKeys
    .filter((key) => key.startsWith("streamInfo.reason.") || key.startsWith("streamInfo.warning."))
    .map((key): unknown => i18n.getResource(language, playerNamespace, key));
}

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

  // The server reports container_not_supported for every mode, also for a transcode.
  it.each(
    supportedLanguages.flatMap((language) => [
      { language, session: transcodeSession },
      { language, session: remuxSession },
    ]),
  )("names no mode in the reasons of a $session.mode in $language", ({ language, session }) => {
    const reasons = renderedReasons(session, language);

    expect(reasons[0]).toMatch(/^MKV .+ → MP4$/);
    expect(reasons.filter((text) => modeWord.test(text))).toEqual([]);
  });

  it.each(supportedLanguages)("names no mode in any reason or warning in %s", (language) => {
    const texts = reasonAndWarningTexts(language);

    expect(texts.length).toBeGreaterThan(0);
    expect(texts.every((text) => typeof text === "string")).toBe(true);
    expect(texts.filter((text) => typeof text === "string" && modeWord.test(text))).toEqual([]);
  });
});
