import {
  createRootRoute,
  createRoute,
  createRouter,
  useNavigate,
  useParams,
  useSearch,
  type RouterHistory,
} from "@tanstack/react-router";
import { createElement } from "react";

import { HomePage } from "../features/home";
import { ItemPage, validateItemSearch } from "../features/item";
import { LibraryPage } from "../features/library";
import { PlayerPage, validatePlayerSearch } from "../features/player";
import { LibrariesSettingsPage, validateSettingsSearch } from "../features/settings";
import { NotFoundPage } from "./NotFoundPage";
import { AppLayout } from "./layout/AppLayout";
import { FullScreenLayout } from "./layout/FullScreenLayout";

// Ids of the pathless layout routes. They never appear in a URL; the routes below them get
// ids such as "/shell/libraries/$libraryId".
const shellLayoutId = "shell";
const fullScreenLayoutId = "full-screen";

// The root is the only route with a notFoundComponent, so every unmatched path ends there; it
// renders outside the shell layout route and therefore brings its own shell.
function ShellNotFound() {
  return createElement(AppLayout, { children: createElement(NotFoundPage) });
}

// Route components hand route params and search to the feature pages as props, so features
// stay independent of the route tree and its layout ids.
function LibraryRoute() {
  const { libraryId } = useParams({ from: "/shell/libraries/$libraryId" });
  return createElement(LibraryPage, { libraryId });
}

function ItemRoute() {
  const { itemId } = useParams({ from: "/shell/items/$itemId" });
  const search = useSearch({ from: "/shell/items/$itemId" });
  return createElement(ItemPage, { itemId, search });
}

// The dialog state of the settings page lives in the URL; the page gets it and a setter.
function SettingsRoute() {
  const search = useSearch({ from: "/shell/admin/manage/libraries" });
  const navigate = useNavigate({ from: "/admin/manage/libraries" });
  return createElement(LibrariesSettingsPage, {
    search,
    onSearchChange: (next) => {
      void navigate({ search: next });
    },
  });
}

// The whole validated search goes to the player, so new player search params need no edit here.
function PlayerRoute() {
  const { itemId } = useParams({ from: "/full-screen/items/$itemId/play" });
  const search = useSearch({ from: "/full-screen/items/$itemId/play" });
  return createElement(PlayerPage, { itemId, search });
}

// Code-based route tree: no generated file in milestone 1. Switching to file-based routing
// needs an ADR note (ADR 0005). Pages with sidebar and top bar sit below the shell layout
// route; full-screen pages (the player) below their own layout route without them.
function createRouteTree() {
  const rootRoute = createRootRoute({ notFoundComponent: ShellNotFound });
  const shellRoute = createRoute({
    getParentRoute: () => rootRoute,
    id: shellLayoutId,
    component: AppLayout,
  });
  const fullScreenRoute = createRoute({
    getParentRoute: () => rootRoute,
    id: fullScreenLayoutId,
    component: FullScreenLayout,
  });
  const shellPages = [
    createRoute({ getParentRoute: () => shellRoute, path: "/", component: HomePage }),
    createRoute({
      getParentRoute: () => shellRoute,
      path: "/libraries/$libraryId",
      component: LibraryRoute,
    }),
    createRoute({
      getParentRoute: () => shellRoute,
      path: "/admin/manage/libraries",
      validateSearch: validateSettingsSearch,
      component: SettingsRoute,
    }),
    createRoute({
      getParentRoute: () => shellRoute,
      path: "/items/$itemId",
      validateSearch: validateItemSearch,
      component: ItemRoute,
    }),
  ] as const;
  const playerRoute = createRoute({
    getParentRoute: () => fullScreenRoute,
    path: "/items/$itemId/play",
    validateSearch: validatePlayerSearch,
    component: PlayerRoute,
  });
  return rootRoute.addChildren([
    shellRoute.addChildren(shellPages),
    fullScreenRoute.addChildren([playerRoute]),
  ]);
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
