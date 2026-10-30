import { AxeBuilder } from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

import { abortSystemInfo, openApp } from "./support/shell";

// Minor and moderate findings are reviewed by hand; these two levels block a release.
const blockingImpacts: readonly (string | null | undefined)[] = ["serious", "critical"];

interface ScanCase {
  readonly name: string;
  readonly collapsed: boolean;
  readonly serverReachable: boolean;
}

const scanCases: readonly ScanCase[] = [
  { name: "sidebar expanded", collapsed: false, serverReachable: true },
  { name: "sidebar collapsed", collapsed: true, serverReachable: true },
  { name: "sidebar collapsed, server unreachable", collapsed: true, serverReachable: false },
];

async function blockingViolations(page: Page): Promise<string[]> {
  const results = await new AxeBuilder({ page }).analyze();
  return results.violations
    .filter((violation) => blockingImpacts.includes(violation.impact))
    .map((violation) => `${violation.id}: ${violation.help} (${String(violation.nodes.length)}x)`);
}

for (const scanCase of scanCases) {
  test(`home has no serious or critical axe violations (${scanCase.name})`, async ({ page }) => {
    if (!scanCase.serverReachable) {
      await abortSystemInfo(page);
    }
    const { sidebarToggle, serverStatus } = await openApp(page);
    // Scan the settled page: the footer swaps its loading skeleton for the version or error.
    await expect(serverStatus).toContainText(
      scanCase.serverReachable ? "Server version" : "Server unreachable",
    );
    if (scanCase.collapsed) {
      await sidebarToggle.click();
      await expect(sidebarToggle).toHaveAttribute("aria-expanded", "false");
    }

    expect(await blockingViolations(page)).toEqual([]);
  });
}
