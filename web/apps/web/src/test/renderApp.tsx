import { QueryClient } from "@tanstack/react-query";
import { createMemoryHistory } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import type { i18n as I18n } from "i18next";

import { App } from "../app/App";
import { createAppRouter, type AppRouter } from "../app/router";
import { createI18n } from "../shared/i18n";
import { createFakeApiClient, healthyServer, type FakeFetch } from "./fakeApi";

/** Options of {@link renderApp}. */
export interface RenderAppOptions {
  /** Languages the browser prefers, most preferred first. Defaults to English. */
  readonly languages?: readonly string[];
  /** Answers the API requests. Defaults to a healthy server. */
  readonly fetch?: FakeFetch;
}

/** Services of an app rendered by {@link renderApp}, for tests that drive them. */
export interface RenderedApp {
  readonly i18n: I18n;
  readonly router: AppRouter;
}

/**
 * Creates a query client for one test. Retries keep the count the queries ask for but run
 * without delay, so error states appear within the default `findBy` timeout.
 */
function createTestQueryClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retryDelay: 0 } } });
}

/** Renders the whole app at `path` with its own router, query client, API and i18n instance. */
export function renderApp(path = "/", options: RenderAppOptions = {}): RenderedApp {
  const { languages = ["en"], fetch = healthyServer } = options;
  const i18n = createI18n(languages);
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) });
  render(
    <App
      apiClient={createFakeApiClient(fetch)}
      i18n={i18n}
      queryClient={createTestQueryClient()}
      router={router}
    />,
  );
  return { i18n, router };
}
