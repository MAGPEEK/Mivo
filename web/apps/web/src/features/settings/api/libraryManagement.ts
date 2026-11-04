import { requestData, type ApiClient } from "@mivo/api-client";

/** Removes a library and its catalog; media files are not touched. Resolves on 204. */
export async function deleteLibrary(client: ApiClient, libraryId: string): Promise<void> {
  await requestData(
    client.DELETE("/api/v1/libraries/{libraryId}", { params: { path: { libraryId } } }),
  );
}
