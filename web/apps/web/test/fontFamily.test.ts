import { readFileSync } from "node:fs";
import { createRequire } from "node:module";

import { selfHostedSansFamily } from "@mivo/ui-tokens";
import { describe, expect, it } from "vitest";

// main.tsx imports this stylesheet. Read from disk instead of importing: Vitest replaces CSS
// imports with empty modules. Resolved through the package, so it follows the installed version.
const interCssPath = createRequire(import.meta.url).resolve("@fontsource-variable/inter/index.css");
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
