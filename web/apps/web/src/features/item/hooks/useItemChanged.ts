import type { ItemDetail } from "@mivo/api-client";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";

import { libraryItemsQueryKey } from "../../library";
import { itemQueryKey } from "../api/itemQuery";

/**
 * Callback for actions that changed the item on the server: shows the state the server
 * answered with at once and reloads the item, whose metadata may still change in the
 * background (an applied match is enriched after the answer). The cached item pages of its
 * library are reloaded too, so title, poster and the "Unmatched" badge stay current there.
 */
export function useItemChanged(itemId: string): (item?: ItemDetail) => void {
  const queryClient = useQueryClient();
  return useCallback(
    (item?: ItemDetail) => {
      const queryKey = itemQueryKey(itemId);
      const libraryId =
        item?.libraryId ?? queryClient.getQueryData<ItemDetail>(queryKey)?.libraryId;
      if (item !== undefined) {
        queryClient.setQueryData(queryKey, item);
      }
      void queryClient.invalidateQueries({ queryKey });
      if (libraryId !== undefined) {
        void queryClient.invalidateQueries({ queryKey: libraryItemsQueryKey(libraryId) });
      }
    },
    [queryClient, itemId],
  );
}
