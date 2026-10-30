import {
  createRootRoute,
  createRoute,
  createRouter,
  type RouterHistory,
} from "@tanstack/react-router";

import { HomePage } from "../features/home";
import { NotFoundPage } from "./NotFoundPage";
import { RootLayout } from "./RootLayout";

// Code-based route tree: no generated file in milestone 1. Switching to file-based routing
// needs an ADR note (ADR 0005).
function createRouteTree() {
  const rootRoute = createRootRoute({
    component: RootLayout,
    notFoundComponent: NotFoundPage,
  });
  const homeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: HomePage,
  });
  return rootRoute.addChildren([homeRoute]);
}

/** Options for {@link createAppRouter}. */
export interface AppRouterOptions {
  /** Navigation history. Defaults to the browser history; tests pass a memory history. */
  readonly history?: RouterHistory;
}

/** Creates the router of the browser app. Each call returns an independent router. */
export function createAppRouter(options: AppRouterOptions = {}) {
  return createRouter({ routeTree: createRouteTree(), ...options });
}

/** The router type of the browser app. */
export type AppRouter = ReturnType<typeof createAppRouter>;

declare module "@tanstack/react-router" {
  interface Register {
    router: AppRouter;
  }
}
