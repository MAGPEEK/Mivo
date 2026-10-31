// Runs axe-core on rendered components in jsdom.

// axe-core is a CommonJS module (`export =`): Node offers only its default export, while
// import-x reads the named members of its type declarations as ES module exports.
/* eslint-disable import-x/default, import-x/no-named-as-default-member -- CommonJS module */
import axe from "axe-core";

/** Impact levels that block a release; minor and moderate findings are reviewed by hand. */
const blockingImpacts: readonly string[] = ["serious", "critical"];

// jsdom has no layout and no computed colours, so the contrast rule cannot work here. The
// Playwright accessibility spec checks contrast in a real browser.
const rulesNeedingLayout = { "color-contrast": { enabled: false } };

/**
 * Runs axe on `container` and returns its serious and critical violations as readable lines,
 * e.g. `button-name: Buttons must have discernible text (2x)`. An empty array is a pass.
 */
export async function blockingAxeViolations(container: Element): Promise<string[]> {
  const results = await axe.run(container, { rules: rulesNeedingLayout });
  return results.violations
    .filter((violation) => blockingImpacts.includes(violation.impact ?? ""))
    .map((violation) => `${violation.id}: ${violation.help} (${String(violation.nodes.length)}x)`);
}
