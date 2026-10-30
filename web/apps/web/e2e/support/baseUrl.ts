import { productId } from "@mivo/ui-tokens";

/**
 * Environment variable through which the global setup hands the server origin to the test
 * workers. Playwright starts the workers after the global setup and they inherit its
 * environment, so `playwright.config.ts` reads it when a worker loads the config.
 */
export const baseUrlVariable = `${productId.toUpperCase()}_E2E_BASE_URL`;
