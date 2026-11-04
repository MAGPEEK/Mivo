import {
  requestData,
  type ApiClient,
  type FolderListing,
  type FolderRootList,
  type Library,
  type LibraryCreate,
  type LibraryUpdate,
} from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

/** Removes a library and its catalog; media files are not touched. Resolves on 204. */
export async function deleteLibrary(client: ApiClient, libraryId: string): Promise<void> {
  await requestData(
    client.DELETE("/api/v1/libraries/{libraryId}", { params: { path: { libraryId } } }),
  );
}

/** Creates a library; the server scans it right away. */
export function createLibrary(client: ApiClient, body: LibraryCreate): Promise<Library> {
  return requestData(client.POST("/api/v1/libraries", { body }));
}

/** Request of {@link updateLibrary}. */
export interface UpdateLibraryRequest {
  readonly libraryId: string;
  readonly body: LibraryUpdate;
}

/** Changes name, folders, language or options of a library. */
export function updateLibrary(client: ApiClient, request: UpdateLibraryRequest): Promise<Library> {
  return requestData(
    client.PATCH("/api/v1/libraries/{libraryId}", {
      params: { path: { libraryId: request.libraryId } },
      body: request.body,
    }),
  );
}

/** Cache key of the folder roots. */
export const folderRootsQueryKey = ["folder-roots"] as const;

/** Query options for the folder roots and the installation facts that go with them. */
export function folderRootsQueryOptions(client: ApiClient) {
  return queryOptions<FolderRootList>({
    queryKey: folderRootsQueryKey,
    queryFn: ({ signal }) => requestData(client.GET("/api/v1/folder-roots", { signal })),
    // Permissions change while the admin fixes them; always look again when the dialog opens.
    staleTime: 0,
  });
}

/** Where a folder listing is read from. */
export interface FolderLocation {
  readonly rootId: string;
  /** Path relative to the root; empty for the root itself. */
  readonly path: string;
}

/** Query options for the sub-folders at `location`. */
export function folderListingQueryOptions(client: ApiClient, location: FolderLocation) {
  return queryOptions<FolderListing>({
    queryKey: [...folderRootsQueryKey, location.rootId, location.path] as const,
    queryFn: ({ signal }) =>
      requestData(
        client.GET("/api/v1/folder-roots/{rootId}/folders", {
          params: { path: { rootId: location.rootId }, query: { path: location.path } },
          signal,
        }),
      ),
    staleTime: 0,
  });
}
