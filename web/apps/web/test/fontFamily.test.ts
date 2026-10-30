import { readFileSync } from "node:fs";
import { createRequire } from "node:module";

import { selfHostedSansFamily } from "@mivo/ui-tokens";
import { describe, expect, it } from "vitest";

// Resolves the bare specifier exactly as main.tsx imports it, so the test reads the stylesheet
// the package's export map selects, even if a font update points it at another file. Read
// from disk instead of importing: Vitest replaces CSS imports with empty modules.
const interCssPath = createRequire(import.meta.url).resolve("@fontsource-variable/inter");
const interCss = readFileSync(interCssPath, "utf8");

const fontFamilyDeclaration = /font-family:\s*["']?([^"';]+?)["']?\s*;/g;

// The token font stack only reaches the bundled font if both agree on the family name, so a
// renamed family after a font package update fails here instead of silently falling back.
describe("self-hosted Inter", () => {
  it("registers the family name the font stack expects", () => {
    const families = new Set(
      Array.from(interCss.matchAll(fontFamilyDeclaration), (match) => match[1]),
    );

    expect([...families]).toEqual([selfHostedSansFamily]);
  });
});
