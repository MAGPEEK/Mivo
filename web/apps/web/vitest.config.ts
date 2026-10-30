import react from "@vitejs/plugin-react";
import { configDefaults, defineProject } from "vitest/config";

// Whole-app tests (router, providers, axe in jsdom) run slowly on a loaded machine. Kept above
// Testing Library's async timeout in src/test/setup.ts, so a slow findBy reports its own
// error instead of a bare test timeout.
const testTimeoutMs = 15_000;

// Kept apart from vite.config.ts, so tests load neither Tailwind nor the dev proxy.
export default defineProject({
  plugins: [react()],
  test: {
    name: "web",
    environment: "jsdom",
    // Playwright runs the specs in e2e/ against the real server (`pnpm e2e`).
    exclude: [...configDefaults.exclude, "e2e/**"],
    setupFiles: ["./src/test/setup.ts"],
    testTimeout: testTimeoutMs,
  },
});
