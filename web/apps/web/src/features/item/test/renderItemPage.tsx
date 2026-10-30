import { createApiClient } from "@mivo/api-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
  useParams,
  type AnyRouter,
} from "@tanstack/react-router";
import { render } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";

import { ApiClientProvider } from "../../../shared/api";
import { createI18n } from "../../../shared/i18n";
import { ItemPage } from "../components/ItemPage";
import { itemLocales, itemNamespace } from "../locales";
import type { ItemServerFetch } from "./fakeItemServer";

// Absolute, because Node's Request cannot resolve the client's default relative URLs.
const fakeServerUrl = "http://server.test";

/** Options of {@link renderItemPage}. */
export interface RenderItemPageOptions {
  /** Answers the API requests. */
  readonly fetch: ItemServerFetch;
  /** Languages the browser prefers, most preferred first. Defaults to English. */
  readonly languages?: readonly string[];
}

// The route component reads the param loosely: the typed route ids belong to the app router.
function ItemRouteForTest() {
  const { itemId = "" } = useParams({ strict: false });
  return <ItemPage itemId={itemId} />;
}

function PlayerRouteForTest() {
  return <h1>Player</h1>;
}

function HomeRouteForTest() {
  return <h1>Home</h1>;
}

// The item page without the app shell, so these tests depend only on the item feature, the
// router and the API client; the full app is covered by the routing test in ItemPage.test.
function createItemTestRouter(path: string): AnyRouter {
  const rootRoute = createRootRoute({
    component: () => (
      <main>
        <Outlet />
      </main>
    ),
  });
  const routeTree = rootRoute.addChildren([
    createRoute({ getParentRoute: () => rootRoute, path: "/", component: HomeRouteForTest }),
    createRoute({
      getParentRoute: () => rootRoute,
      path: "/items/$itemId",
      component: ItemRouteForTest,
    }),
    createRoute({
      getParentRoute: () => rootRoute,
      path: "/items/$itemId/play",
      component: PlayerRouteForTest,
    }),
  ]);
  return createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [path] }) });
}

/** Services of a page rendered by {@link renderItemPage}, for tests that drive them. */
export interface RenderedItemPage {
  readonly router: AnyRouter;
  readonly queryClient: QueryClient;
}

/**
 * Renders the item page at `path` (e.g. `/items/item-1`) with its own router, query client,
 * API client and i18n instance. Retries keep their count but run without delay.
 */
export function renderItemPage(path: string, options: RenderItemPageOptions): RenderedItemPage {
  const { fetch, languages = ["en"] } = options;
  const router = createItemTestRouter(path);
  const i18n = createI18n(languages, { [itemNamespace]: itemLocales });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retryDelay: 0 } } });
  render(
    <I18nextProvider i18n={i18n}>
      <ApiClientProvider client={createApiClient({ baseUrl: fakeServerUrl, fetch })}>
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </ApiClientProvider>
    </I18nextProvider>,
  );
  return { router, queryClient };
}
