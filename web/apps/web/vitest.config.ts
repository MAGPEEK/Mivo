import react from "@vitejs/plugin-react";
import { defineProject } from "vitest/config";

// Kept apart from vite.config.ts, so tests load neither Tailwind nor the dev proxy.
export default defineProject({
  plugins: [react()],
  test: {
    name: "web",
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
});
