import react from "@vitejs/plugin-react";
import { configDefaults, defineProject } from "vitest/config";

// Kept apart from vite.config.ts, so tests load neither Tailwind nor the dev proxy.
export default defineProject({
  plugins: [react()],
  test: {
    name: "web",
    environment: "jsdom",
    // Playwright runs the specs in e2e/ against the real server (`pnpm e2e`).
    exclude: [...configDefaults.exclude, "e2e/**"],
    setupFiles: ["./src/test/setup.ts"],
  },
});
