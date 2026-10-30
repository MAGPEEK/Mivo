import type { ItemDetail } from "@mivo/api-client";
import { useQuery, type UseQueryResult } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { itemQueryOptions } from "../api/itemQuery";

/** Loads one item with its files and streams. Failures arrive as `ApiError`. */
export function useItem(itemId: string): UseQueryResult<ItemDetail> {
  const client = useApiClient();
  return useQuery(itemQueryOptions(client, itemId));
}
