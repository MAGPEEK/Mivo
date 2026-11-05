import {
  requestData,
  toApiError,
  toApiErrorFromException,
  type ApiClient,
  type IdentifyCandidate,
  type IdentifySearchRequest,
  type ItemDetail,
  type MetadataProvider,
} from "@mivo/api-client";

/** Prefix of every identify search cache key. */
export const identifySearchQueryKey = ["identify", "search"] as const;

/** Cache key of one identify search of an item. */
export function identifySearchKey(itemId: string, request: IdentifySearchRequest) {
  return [...identifySearchQueryKey, itemId, request] as const;
}

/** The provider entry an item is matched with. */
export interface MatchTarget {
  readonly provider: MetadataProvider;
  readonly externalId: string;
}

/** Searches the providers for candidates to match the item with, best first. */
export async function searchCandidates(
  client: ApiClient,
  itemId: string,
  request: IdentifySearchRequest,
  signal?: AbortSignal,
): Promise<readonly IdentifyCandidate[]> {
  const list = await requestData(
    client.POST("/api/v1/items/{itemId}/identify/search", {
      params: { path: { itemId } },
      body: request,
      ...(signal === undefined ? {} : { signal }),
    }),
  );
  // The contract requires the list; another body must not break the dialog.
  return Array.isArray(list.candidates) ? list.candidates : [];
}

/** Matches the item with a provider entry; resolves with the item before its enrichment ran. */
export function identifyItem(
  client: ApiClient,
  itemId: string,
  target: MatchTarget,
): Promise<ItemDetail> {
  return requestData(
    client.POST("/api/v1/items/{itemId}/identify", {
      params: { path: { itemId } },
      body: { provider: target.provider, externalId: target.externalId },
    }),
  );
}

/** Removes the online match of the item; local data stays. */
export function removeItemMatch(client: ApiClient, itemId: string): Promise<ItemDetail> {
  return requestData(
    client.DELETE("/api/v1/items/{itemId}/match", { params: { path: { itemId } } }),
  );
}

/**
 * Queues a metadata refresh of the item. The server answers 202 without a body, which
 * `requestData` would treat as a broken response, so the status alone decides.
 */
export async function refreshItemMetadata(client: ApiClient, itemId: string): Promise<void> {
  let result: Awaited<ReturnType<typeof postRefresh>>;
  try {
    result = await postRefresh(client, itemId);
  } catch (cause) {
    throw toApiErrorFromException(cause);
  }
  if (!result.response.ok) {
    throw toApiError(result);
  }
}

function postRefresh(client: ApiClient, itemId: string) {
  return client.POST("/api/v1/items/{itemId}/refresh", {
    params: { path: { itemId } },
    // The 202 body is empty; reading it as JSON would fail.
    parseAs: "stream",
  });
}
