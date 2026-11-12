import type { LibraryFolderEntry, LibraryFolderListing } from "@mivo/api-client";
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
} from "../../../test/fakeApi";
import { fakeItem, fakeLibrary, libraryRoute } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";
import { maxFolderOnlyPages } from "../hooks/useFolderListing";

const library = fakeLibrary();
const topFolderId = library.folders[0]?.id ?? "";
const foldersApiPath = `/api/v1/libraries/${library.id}/folders`;
const folderUrl = `/libraries/${library.id}/folders/${topFolderId}`;

function folder(name: string): LibraryFolderEntry {
  return { kind: "folder", name, path: `${topFolderId}/${name}` };
}

function listing(entries: LibraryFolderEntry[], nextCursor?: string): LibraryFolderListing {
  return {
    libraryId: library.id,
    path: topFolderId,
    entries,
    ...(nextCursor === undefined ? {} : { nextCursor }),
  };
}

function render(route: FakeRoute) {
  const server = createFakeServer([libraryRoute(library), route]);
  renderApp(folderUrl, { fetch: server.fetch });
  return server;
}

function cursorsOf(server: ReturnType<typeof createFakeServer>): (string | null)[] {
  return server.requests
    .filter((request) => new URL(request.url).pathname === foldersApiPath)
    .map((request) => new URL(request.url).searchParams.get("cursor"));
}

describe("FolderBrowser paging", () => {
  it("loads the next page by cursor when the first holds only folders", async () => {
    const item: LibraryFolderEntry = {
      kind: "item",
      name: "bunny.mkv",
      path: `${topFolderId}/bunny.mkv`,
      item: fakeItem(),
    };
    const server = render({
      method: "GET",
      path: foldersApiPath,
      respond: ({ url }) =>
        jsonResponse(
          url.searchParams.get("cursor") === "page-2"
            ? listing([item])
            : listing([folder("Animation")], "page-2"),
        ),
    });

    expect(await screen.findByText("Big Buck Bunny")).toBeDefined();
    expect(screen.getByRole("link", { name: "Animation" })).toBeDefined();
    expect(cursorsOf(server)).toEqual([null, "page-2"]);
  });

  it("offers a retry when the next page fails and only folders are loaded", async () => {
    let failNext = true;
    render({
      method: "GET",
      path: foldersApiPath,
      respond: ({ url }) => {
        if (url.searchParams.get("cursor") === null) {
          return jsonResponse(listing([folder("Animation")], "page-2"));
        }
        if (failNext) {
          return problemResponse(500, "internal");
        }
        return jsonResponse(
          listing([
            { kind: "item", name: "b.mkv", path: `${topFolderId}/b.mkv`, item: fakeItem() },
          ]),
        );
      },
    });

    expect(await screen.findByText("More items could not be loaded.")).toBeDefined();
    failNext = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(await screen.findByText("Big Buck Bunny")).toBeDefined();
  });

  it("stops asking by itself after a bounded number of folder-only pages", async () => {
    let requests = 0;
    render({
      method: "GET",
      path: foldersApiPath,
      respond: () => {
        requests += 1;
        return jsonResponse(
          listing([folder(`Folder ${String(requests)}`)], `page-${String(requests)}`),
        );
      },
    });

    expect(
      await screen.findByText("More items could not be loaded.", {}, { timeout: 15_000 }),
    ).toBeDefined();
    expect(requests).toBe(maxFolderOnlyPages);
  });

  it("stops at a page without entries even when it names a next cursor", async () => {
    let requests = 0;
    render({
      method: "GET",
      path: foldersApiPath,
      respond: () => {
        requests += 1;
        return jsonResponse(
          requests === 1 ? listing([folder("Animation")], "again") : listing([], "again"),
        );
      },
    });

    expect(await screen.findByText("More items could not be loaded.")).toBeDefined();
    expect(requests).toBe(2);
  });
});
