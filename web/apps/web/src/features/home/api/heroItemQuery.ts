import { requestData, type ApiClient } from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

import { itemQueryKey } from "../../../shared/api";

/**
 * Query options for the item behind a hero title. The key is the one the detail page uses, so
 * "More info" after "Play" (or the other way round) reads the cached item.
 */
export function heroItemQueryOptions(client: ApiClient, itemId: string) {
  return queryOptions({
    queryKey: itemQueryKey(itemId),
    queryFn: ({ signal }) =>
      requestData(client.GET("/api/v1/items/{itemId}", { params: { path: { itemId } }, signal })),
    retry: false,
  });
}
