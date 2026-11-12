import { knownReasonCodes, knownWarningCodes } from "@mivo/player-core";
import { describe, expect, it } from "vitest";

import openApiText from "../../../../../../../api/openapi.yaml?raw";
import { createAppI18n } from "../../../app/i18n";
import { supportedLanguages } from "../../../shared/i18n";
import { playerNamespace } from "../locales";
import { reasonHintKey, reasonHintKeys } from "./reasonHints";

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
      const hint = reasonHintKey(code);
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
});
