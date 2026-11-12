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
  RemoteSetupPage,
  SessionGate,
  SetupPage,
  SignInPage,
  profilesPath,
  remoteSetupPath,
  setupPath,
  signInPath,
  validateAuthSearch,
  useAdminViewer,
} from "../features/auth";
import {
  AdminDevicesPage,
  OwnDevicesPage,
  adminDevicesPath,
  ownDevicesPath,
} from "../features/devices";
import { HomePage } from "../features/home";
import { ItemPage, validateItemSearch } from "../features/item";
import {
  FolderBrowser,
  LibraryPage,
  validateBrowseSearch,
  type BrowseSearchChange,
  type LibraryItemsRenderer,
} from "../features/library";
import { MusicLibraryGrid } from "../features/music";
import { PhotoAlbumGrid, PhotoGrid } from "../features/photos";
import { PlayerPage, validatePlayerSearch } from "../features/player";
import { ProfilePickerPage } from "../features/profiles";
import {
  AboutSettingsPage,
  LibrariesSettingsPage,
  MetadataSettingsPage,
  TranscoderSettingsPage,
  validateSettingsSearch,
} from "../features/settings";
import {
  OwnProfilesPage,
  UsersPage,
  addProfileHref,
  ownAccountPath,
  validateAccountSearch,
} from "../features/users";
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

// The folder view: the splat is the folder path below the library, the URL is the whole state.
function LibraryFoldersRoute() {
  const { libraryId, _splat } = useParams({ from: "/shell/libraries/$libraryId/folders/$" });
  return createElement(FolderBrowser, { libraryId, splat: _splat });
}

function libraryFoldersPage<Parent extends AnyRoute>(parent: Parent) {
  return createRoute({
    getParentRoute: () => parent,
    path: "/libraries/$libraryId/folders/$",
    component: LibraryFoldersRoute,
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

// Admin › Users (M7-32): a viewer without admin powers gets the not-found page, so the URL
// does not even reveal that the page exists.
function AdminUsersRoute() {
  const admin = useAdminViewer();
  return createElement(admin ? UsersPage : NotFoundPage);
}

// The own account page (M7-33): which profile editor is open lives in the URL.
function OwnAccountRoute() {
  const search = useSearch({ from: "/shell/settings/account" });
  const navigate = useNavigate({ from: ownAccountPath });
  return createElement(OwnProfilesPage, {
    search,
    onSearchChange: (next) => {
      void navigate({ search: next });
    },
  });
}

// "Add profile" in the picker selects the primary profile and then opens the profile editor.
function ProfilePickerRoute() {
  return createElement(ProfilePickerPage, { addProfilePath: addProfileHref });
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
    createRoute({
      getParentRoute: () => parent,
      path: profilesPath,
      validateSearch: validateAuthSearch,
      component: ProfilePickerRoute,
    }),
  ] as const;
}

// Admin pages without search params; split off to keep the route tree function short.
function adminPages<Parent extends AnyRoute>(parent: Parent) {
  return [
    createRoute({
      getParentRoute: () => parent,
      path: "/admin/settings/metadata",
      component: MetadataSettingsPage,
    }),
    createRoute({
      getParentRoute: () => parent,
      path: "/admin/settings/transcoder",
      component: TranscoderSettingsPage,
    }),
    createRoute({
      getParentRoute: () => parent,
      path: "/admin/settings/about",
      component: AboutSettingsPage,
    }),
    createRoute({
      getParentRoute: () => parent,
      path: "/admin/users",
      component: AdminUsersRoute,
    }),
  ] as const;
}

// The own account page, the own device list and Admin › Devices (M7-33, M7-34).
function accountPages<Parent extends AnyRoute>(parent: Parent) {
  return [
    createRoute({
      getParentRoute: () => parent,
      path: ownAccountPath,
      validateSearch: validateAccountSearch,
      component: OwnAccountRoute,
    }),
    createRoute({ getParentRoute: () => parent, path: ownDevicesPath, component: OwnDevicesPage }),
    createRoute({
      getParentRoute: () => parent,
      path: adminDevicesPath,
      component: AdminDevicesPage,
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
    libraryFoldersPage(shellRoute),
    createRoute({
      getParentRoute: () => shellRoute,
      path: "/admin/manage/libraries",
      validateSearch: validateSettingsSearch,
      component: SettingsRoute,
    }),
    ...adminPages(shellRoute),
    createRoute({
      getParentRoute: () => shellRoute,
      path: "/items/$itemId",
      validateSearch: validateItemSearch,
      component: ItemRoute,
    }),
    ...accountPages(shellRoute),
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
