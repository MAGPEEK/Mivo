import { knownReasonCodes, knownWarningCodes } from "@mivo/player-core";
import { describe, expect, it } from "vitest";

import openApiText from "../../../../../../../api/openapi.yaml?raw";
import { createAppI18n } from "../../../app/i18n";
import { supportedLanguages } from "../../../shared/i18n";
import { playerNamespace } from "../locales";
import { alternativeAudioTracks, reasonHintKey, reasonHintKeys } from "./reasonHints";

const aacStereo = { index: 2, codec: "aac", channels: 2, default: false, commentary: false };
const ac3Surround = { index: 1, codec: "ac3", channels: 6, default: true, commentary: false };
const dtsEight = { index: 3, codec: "dts", channels: 8, default: false, commentary: false };

/**
 * The codes the API documents for `PlaybackReason.code`: the quoted words of that property's
 * description (the contract lists them in prose, the field stays an open string).
 */
function documentedCodes(): string[] {
  const start = openApiText.indexOf("    PlaybackReason:\n");
  const end = openApiText.indexOf("    PlaybackSession:\n", start);
  const block = openApiText.slice(start, end);
  const description = block.slice(block.indexOf("code:"), block.indexOf("example:"));
  return [...description.matchAll(/"([a-z_]+)"/g)].map((match) => match[1] ?? "");
}

describe("reasonHints", () => {
  it("reads the documented codes from the OpenAPI contract", () => {
    expect(documentedCodes()).toContain("container_not_supported");
    expect(documentedCodes()).toContain("subtitle_burn_in_unavailable");
  });

  it.each(documentedCodes())("maps %s and translates it", (code) => {
    // `in`: a code without a hint is listed with `undefined`, a missing code is a gap.
    expect(code in reasonHintKeys, `${code} is missing in reasonHintKeys`).toBe(true);
    // The why text of a known code is covered by streamInfoLocales.test.ts (every key, every
    // language); here the code must be one player-core knows, or it would read "Other reason".
    const known: readonly string[] = [...knownReasonCodes, ...knownWarningCodes];
    expect(known, `${code} is unknown to player-core`).toContain(code);
    for (const language of supportedLanguages) {
      const i18n = createAppI18n([language]);
      const hint = reasonHintKeys[code as keyof typeof reasonHintKeys];
      if (hint !== undefined) {
        expect(i18n.getResource(language, playerNamespace, hint), `${hint} in ${language}`).toEqual(
          expect.any(String),
        );
      }
    }
  });

  it("has no entry for a code the contract does not document", () => {
    expect(Object.keys(reasonHintKeys).sort()).toEqual([...documentedCodes()].sort());
  });

  it("knows no hint for an unknown code", () => {
    expect(reasonHintKey("constructor")).toBeUndefined();
    expect(reasonHintKey("from_the_future")).toBeUndefined();
  });

  describe("audio hints need an alternative track", () => {
    it.each([
      { code: "audio_codec_not_supported", tracks: [aacStereo], shown: true },
      { code: "audio_codec_not_supported", tracks: [ac3Surround], shown: false },
      { code: "audio_codec_not_supported", tracks: [], shown: false },
      { code: "audio_channels_not_supported", tracks: [ac3Surround], shown: true },
      { code: "audio_channels_not_supported", tracks: [dtsEight], shown: false },
      { code: "audio_channels_not_supported", tracks: [], shown: false },
      { code: "subtitle_burn_in", tracks: [], shown: true },
    ])(" with .length alternatives: shown ", ({ code, tracks, shown }) => {
      expect(reasonHintKey(code, tracks) !== undefined).toBe(shown);
    });

    it("leaves out the track that plays", () => {
      expect(alternativeAudioTracks([ac3Surround, aacStereo], 2)).toEqual([ac3Surround]);
      expect(alternativeAudioTracks([ac3Surround], undefined)).toEqual([ac3Surround]);
    });
  });
});
