import {
  createRootRoute,
  type AnyRoute,
  createRoute,
  createRouter,
  Outlet,
  useNavigate,
  useParams,
  useSearch,
  type RouterHistory,
} from "@tanstack/react-router";
import { createElement, useCallback } from "react";

import { AudiobookGrid } from "../features/audiobooks";
import {
  ProfilesPlaceholderPage,
  RemoteSetupPage,
  SessionGate,
  SetupPage,
  SignInPage,
  profilesPath,
  remoteSetupPath,
  setupPath,
  signInPath,
  validateAuthSearch,
} from "../features/auth";
import { HomePage } from "../features/home";
import { ItemPage, validateItemSearch } from "../features/item";
import {
  LibraryPage,
  validateBrowseSearch,
  type BrowseSearchChange,
  type LibraryItemsRenderer,
} from "../features/library";
import { MusicLibraryGrid } from "../features/music";
import { PhotoAlbumGrid, PhotoGrid } from "../features/photos";
import { PlayerPage, validatePlayerSearch } from "../features/player";
import {
  AboutSettingsPage,
  LibrariesSettingsPage,
  MetadataSettingsPage,
  validateSettingsSearch,
} from "../features/settings";
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
// The Browse state (sort, type, filter) lives in the URL; a change starts at the top again.
function LibraryRoute() {
  const { libraryId } = useParams({ from: "/shell/libraries/$libraryId" });
  const search = useSearch({ from: "/shell/libraries/$libraryId" });
  const navigate = useNavigate({ from: "/libraries/$libraryId" });
  // Stable, so effects that depend on it (the random seed) do not run again on every render.
  const onSearchChange = useCallback<BrowseSearchChange>(
    (next, navigation) => {
      void navigate({ search: next, replace: navigation?.replace === true, resetScroll: true });
    },
    [navigate],
  );
  return createElement(LibraryPage, {
    libraryId,
    search,
    onSearchChange,
    renderItems: itemsViewByType,
  });
}

// Listed types with their own view: albums get square covers, books their author line, photo
// albums and photos square tiles; the
// others keep the poster grid.
const itemsViewByType: LibraryItemsRenderer = ({ type, items, filter }) => {
  switch (type) {
    case "album":
      return createElement(MusicLibraryGrid, { albums: items, filter });
    case "audiobook":
      return createElement(AudiobookGrid, { books: items, filter });
    case "photo_album":
      return createElement(PhotoAlbumGrid, { items, filter });
    case "photo":
      return createElement(PhotoGrid, { items, filter });
    default:
      return undefined;
  }
};

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

// Every page sits below the session gate: it decides whether the page may show (ADR 0032).
function GatedRoot() {
  return createElement(SessionGate, { children: createElement(Outlet) });
}

function SignInRoute() {
  const search = useSearch({ from: "/full-screen/sign-in" });
  return createElement(SignInPage, { search });
}

// Sign-in, setup and the profile picker: full screen, without sidebar and top bar.
function authPages<Parent extends AnyRoute>(parent: Parent) {
  return [
    createRoute({
      getParentRoute: () => parent,
      path: signInPath,
      validateSearch: validateAuthSearch,
      component: SignInRoute,
    }),
    createRoute({ getParentRoute: () => parent, path: setupPath, component: SetupPage }),
    createRoute({
      getParentRoute: () => parent,
      path: remoteSetupPath,
      component: RemoteSetupPage,
    }),
    // M7-31 replaces the placeholder with the profile picker.
    createRoute({
      getParentRoute: () => parent,
      path: profilesPath,
      validateSearch: validateAuthSearch,
      component: ProfilesPlaceholderPage,
    }),
  ] as const;
}

// Settings pages without search params; split off to keep the route tree function short.
function settingsPages<Parent extends AnyRoute>(parent: Parent) {
  return [
    createRoute({
      getParentRoute: () => parent,
      path: "/admin/settings/metadata",
      component: MetadataSettingsPage,
    }),
    createRoute({
      getParentRoute: () => parent,
      path: "/admin/settings/about",
      component: AboutSettingsPage,
    }),
  ] as const;
}

// Code-based route tree: no generated file in milestone 1. Switching to file-based routing
// needs an ADR note (ADR 0005). Pages with sidebar and top bar sit below the shell layout
// route; full-screen pages (the player) below their own layout route without them.
function createRouteTree() {
  const rootRoute = createRootRoute({ component: GatedRoot, notFoundComponent: ShellNotFound });
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
      validateSearch: validateBrowseSearch,
      component: LibraryRoute,
    }),
    createRoute({
      getParentRoute: () => shellRoute,
      path: "/admin/manage/libraries",
      validateSearch: validateSettingsSearch,
      component: SettingsRoute,
    }),
    ...settingsPages(shellRoute),
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
    fullScreenRoute.addChildren([playerRoute, ...authPages(fullScreenRoute)]),
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
