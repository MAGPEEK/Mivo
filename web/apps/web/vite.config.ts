import { productId, productName } from "@mivo/ui-tokens";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import { defineConfig, loadEnv, type Plugin } from "vite";

const devServerPort = 5173;
const defaultApiTarget = "http://localhost:8460";

// Overrides the Go server the dev proxy forwards /api to, e.g. MIVO_DEV_API_TARGET. The name
// derives from the brand id (ADR 0002). Only this file reads it.
const apiTargetVariable = `${productId.toUpperCase()}_DEV_API_TARGET`;

// The Go binary embeds the build from here (ADR 0020). It lies outside the Vite root, so
// emptyOutDir must be set explicitly.
const embedDirectory = fileURLToPath(
  new URL("../../../internal/adapters/http/webui/dist", import.meta.url),
);
const appDirectory = fileURLToPath(new URL(".", import.meta.url));

/** Uses the configured proxy target; an unset, empty or blank variable means the default. */
function resolveApiTarget(configuredTarget: string | undefined): string {
  const trimmedTarget = configuredTarget?.trim() ?? "";
  return trimmedTarget === "" ? defaultApiTarget : trimmedTarget;
}

/** Injects the product name as the page title, so index.html never spells it. */
function brandTitle(): Plugin {
  return {
    name: "brand-title",
    transformIndexHtml: () => [{ tag: "title", children: productName, injectTo: "head" }],
  };
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, appDirectory, apiTargetVariable);
  return {
    plugins: [react(), tailwindcss(), brandTitle()],
    build: {
      outDir: embedDirectory,
      emptyOutDir: true,
    },
    server: {
      port: devServerPort,
      strictPort: true,
      proxy: {
        "/api": { target: resolveApiTarget(env[apiTargetVariable]) },
      },
    },
  };
});
