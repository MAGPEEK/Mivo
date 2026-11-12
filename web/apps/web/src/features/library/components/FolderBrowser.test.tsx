import type { LibraryFolderEntry, LibraryFolderListing } from "@mivo/api-client";
import { fireEvent, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
} from "../../../test/fakeApi";
import { fakeItem, fakeLibrary, libraryRoute } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";

const library = fakeLibrary();
const topFolderId = library.folders[0]?.id ?? "";
const foldersApiPath = `/api/v1/libraries/${library.id}/folders`;
// The router writes the top without a trailing slash.
const foldersUrl = (path = "") =>
  `/libraries/${library.id}/folders${path === "" ? "" : `/${path}`}`;

function folderEntry(path: string): LibraryFolderEntry {
  return { kind: "folder", name: path.split("/").at(-1) ?? path, path };
}

// Answers the folder listing of `path`; an unknown path is a 404 like on the server.
function foldersRoute(
  listings: Readonly<Record<string, readonly LibraryFolderEntry[]>>,
): FakeRoute {
  return {
    method: "GET",
    path: foldersApiPath,
    respond: ({ url }) => {
      const path = url.searchParams.get("path") ?? "";
      const entries = listings[path];
      if (entries === undefined) {
        return problemResponse(404, "not_found");
      }
      const body: LibraryFolderListing = { libraryId: library.id, path, entries: [...entries] };
      return jsonResponse(body);
    },
  };
}

const animation = `${topFolderId}/Animation`;
const listings = {
  "": [folderEntry(topFolderId)],
  [topFolderId]: [
    folderEntry(animation),
    {
      kind: "item",
      name: "bunny.mkv",
      path: `${topFolderId}/bunny.mkv`,
      item: fakeItem({ title: "Big Buck Bunny" }),
    },
    {
      kind: "item",
      name: "gone.mkv",
      path: `${topFolderId}/gone.mkv`,
      item: fakeItem({
        id: "gone",
        title: "Gone Movie",
        media: { available: false, container: "mkv" },
      }),
    },
  ],
  [animation]: [],
} satisfies Record<string, readonly LibraryFolderEntry[]>;

function renderFolders(path: string, languages: readonly string[] = ["en"]) {
  const server = createFakeServer([libraryRoute(library), foldersRoute(listings)]);
  const app = renderApp(foldersUrl(path), { fetch: server.fetch, languages });
  return { ...app, server };
}

function folderRequestPaths(server: ReturnType<typeof createFakeServer>): (string | null)[] {
  return server.requests
    .filter((request) => new URL(request.url).pathname === foldersApiPath)
    .map((request) => new URL(request.url).searchParams.get("path"));
}

describe("FolderBrowser", () => {
  it("lists the library's folders at the top by their relative names", async () => {
    const { server } = renderFolders("");

    const list = await screen.findByRole("list", { name: "Folders" });
    expect(
      within(list).getByRole("link", { name: "6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d" }),
    ).toBeDefined();
    expect(folderRequestPaths(server)).toEqual([null]);
    expect(document.body.textContent).not.toContain("/media/movies");
  });

  it("shows sub-folders and items of a folder with a breadcrumb to every ancestor", async () => {
    renderFolders(topFolderId);

    expect(await screen.findByText("Big Buck Bunny")).toBeDefined();
    const crumbs = screen.getByRole("navigation", { name: "Folder path" });
    expect(within(crumbs).getByRole("link", { name: "Movies" }).getAttribute("href")).toBe(
      foldersUrl(),
    );
    // The first segment is an id; the breadcrumb shows the last name of the folder's path.
    expect(within(crumbs).getByRole("link", { name: "movies" }).getAttribute("aria-current")).toBe(
      "page",
    );
    expect(screen.getByRole("link", { name: "Animation" }).getAttribute("href")).toBe(
      foldersUrl(animation),
    );
    expect(screen.getByRole("link", { name: /Big Buck Bunny/ }).getAttribute("href")).toBe(
      "/items/8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d",
    );
  });

  it("marks an unavailable item", async () => {
    renderFolders(topFolderId);

    await screen.findByText("Gone Movie");
    expect(screen.getAllByText("Unavailable").length).toBeGreaterThan(0);
  });

  it("opens a folder from the list and goes up from the breadcrumb", async () => {
    const { router, server } = renderFolders(topFolderId);

    fireEvent.click(await screen.findByRole("link", { name: "Animation" }));

    expect(await screen.findByRole("heading", { name: "This folder is empty" })).toBeDefined();
    expect(router.state.location.pathname).toBe(foldersUrl(animation));
    expect(folderRequestPaths(server)).toEqual([topFolderId, animation]);
    const crumbs = screen.getByRole("navigation", { name: "Folder path" });
    expect(
      within(crumbs)
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["Movies", "movies", "Animation"]);

    fireEvent.click(within(crumbs).getByRole("link", { name: "movies" }));
    expect(await screen.findByText("Big Buck Bunny")).toBeDefined();
  });

  it("restores the folder on Back", async () => {
    const { router } = renderFolders(topFolderId);
    fireEvent.click(await screen.findByRole("link", { name: "Animation" }));
    await screen.findByRole("heading", { name: "This folder is empty" });

    router.history.back();

    expect(await screen.findByText("Big Buck Bunny")).toBeDefined();
  });

  it("shows the not-found state for a folder the server does not have", async () => {
    renderFolders(`${topFolderId}/Nope`);

    expect(await screen.findByRole("heading", { name: "Folder not found" })).toBeDefined();
    expect(screen.getByRole("link", { name: "Back to the library" }).getAttribute("href")).toBe(
      `/libraries/${library.id}`,
    );
  });

  it.each([
    ["a parent segment", `${topFolderId}/%2E%2E/x`],
    ["a backslash", `${topFolderId}/a%5Cb`],
    ["a Windows drive", "C%3A/Windows"],
  ])("rejects %s without asking the server", async (_, path) => {
    const { server } = renderFolders(path);

    expect(await screen.findByRole("heading", { name: "Folder not found" })).toBeDefined();
    expect(folderRequestPaths(server)).toEqual([]);
  });

  it("offers a retry when the folder fails to load", async () => {
    const server = createFakeServer([
      libraryRoute(library),
      { method: "GET", path: foldersApiPath, respond: () => problemResponse(500, "internal") },
    ]);
    renderApp(foldersUrl(), { fetch: server.fetch });

    expect(await screen.findByText("The folder could not be loaded.")).toBeDefined();
    expect(screen.getByRole("button", { name: "Try again" })).toBeDefined();
  });

  it("is translated to German", async () => {
    renderFolders(`${topFolderId}/Nope`, ["de"]);

    expect(await screen.findByRole("heading", { name: "Ordner nicht gefunden" })).toBeDefined();
  });

  it("is reachable from the library page", async () => {
    const server = createFakeServer([libraryRoute(library), foldersRoute(listings)]);
    renderApp(`/libraries/${library.id}`, { fetch: server.fetch });

    const link = await screen.findByRole("link", { name: "Browse folders" });

    expect(link.getAttribute("href")).toBe(foldersUrl());
  });

  it("has no accessibility violations", async () => {
    renderFolders(topFolderId);
    await screen.findByText("Big Buck Bunny");

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});
