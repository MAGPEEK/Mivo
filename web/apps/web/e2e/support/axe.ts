import { AxeBuilder } from "@axe-core/playwright";
import type { Page } from "@playwright/test";

// Minor and moderate findings are reviewed by hand; these two levels block a release.
const blockingImpacts: readonly (string | null | undefined)[] = ["serious", "critical"];

/** The serious and critical axe findings on `page`, one line per rule. */
export async function blockingAxeViolations(page: Page): Promise<string[]> {
  const results = await new AxeBuilder({ page }).analyze();
  return results.violations
    .filter((violation) => blockingImpacts.includes(violation.impact))
    .map((violation) => `${violation.id}: ${violation.help} (${String(violation.nodes.length)}x)`);
}
