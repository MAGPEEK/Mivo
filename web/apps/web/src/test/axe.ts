// Runs axe-core on rendered components in jsdom. axe-core reaches this app only as the
// dependency of @axe-core/playwright: the lockfile is frozen in this wave, so it is loaded
// through that package's own module resolution instead of a direct devDependency. Node APIs
// are not typed in src (browser types only), hence the small structural types below.

/** Impact levels that block a release; minor and moderate findings are reviewed by hand. */
const blockingImpacts: readonly string[] = ["serious", "critical"];

// jsdom has no layout and no computed colours, so the contrast rule cannot work here. The
// Playwright accessibility spec checks contrast in a real browser.
const rulesNeedingLayout = { "color-contrast": { enabled: false } };

interface AxeViolation {
  readonly id: string;
  readonly impact?: string | null;
  readonly help: string;
  readonly nodes: readonly unknown[];
}

interface AxeEngine {
  run(
    context: Element,
    options: { readonly rules: Readonly<Record<string, { readonly enabled: boolean }>> },
  ): Promise<{ readonly violations: readonly AxeViolation[] }>;
}

interface NodeModuleApi {
  createRequire(from: string): NodeRequire;
}

interface NodeRequire {
  (id: string): unknown;
  resolve(id: string): string;
}

// A variable specifier keeps TypeScript from resolving Node types that src does not have.
const nodeModuleSpecifier = "node:module";

let enginePromise: Promise<AxeEngine> | undefined;

async function loadAxe(): Promise<AxeEngine> {
  const nodeModule = (await import(/* @vite-ignore */ nodeModuleSpecifier)) as NodeModuleApi;
  const fromHere = nodeModule.createRequire(import.meta.url);
  const fromPlaywrightAdapter = nodeModule.createRequire(fromHere.resolve("@axe-core/playwright"));
  return fromPlaywrightAdapter("axe-core") as AxeEngine;
}

/**
 * Runs axe on `container` and returns its serious and critical violations as readable lines,
 * e.g. `button-name: Buttons must have discernible text (2x)`. An empty array is a pass.
 */
export async function blockingAxeViolations(container: Element): Promise<string[]> {
  enginePromise ??= loadAxe();
  const axe = await enginePromise;
  const results = await axe.run(container, { rules: rulesNeedingLayout });
  return results.violations
    .filter((violation) => blockingImpacts.includes(violation.impact ?? ""))
    .map((violation) => `${violation.id}: ${violation.help} (${String(violation.nodes.length)}x)`);
}
