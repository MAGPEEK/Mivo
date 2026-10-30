import { defineConfig } from "vitest/config";

// Thresholds from ADR 0019. They are never lowered to get a green run.
const globalCoverageThreshold = 70;
const playerCoreCoverageThreshold = 90;

const allMetrics = (threshold: number) => ({
  lines: threshold,
  branches: threshold,
  functions: threshold,
  statements: threshold,
});

export default defineConfig({
  test: {
    projects: ["packages/*", "apps/*"],
    coverage: {
      // Enabled by default so every `pnpm test` enforces the thresholds. Partial runs (e.g.
      // pre-commit `vitest related`) pass `--coverage.enabled=false`.
      enabled: true,
      provider: "v8",
      // Listing sources makes untested files count as uncovered instead of being skipped.
      include: ["packages/*/src/**/*.{ts,tsx}", "apps/*/src/**/*.{ts,tsx}"],
      exclude: [
        "**/generated/**",
        "**/*.gen.ts",
        "**/*.d.ts",
        // App entry points only wire providers into the DOM; the providers are tested directly.
        "apps/*/src/main.tsx",
        "**/*.{test,spec}.{ts,tsx}",
        "**/test/**",
        "**/e2e/**",
      ],
      reporter: ["text", "html", "json-summary"],
      reportsDirectory: "./coverage",
      thresholds: {
        ...allMetrics(globalCoverageThreshold),
        "packages/player-core/src/**": allMetrics(playerCoreCoverageThreshold),
      },
    },
  },
});
