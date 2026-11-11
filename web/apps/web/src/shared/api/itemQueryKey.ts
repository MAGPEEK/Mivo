/** Root of every item cache key; invalidating it reloads all loaded items and their children. */
export const itemsQueryKey = ["items"] as const;

/**
 * Cache key of `GET /api/v1/items/{itemId}`. Every feature that loads an item uses it, so a
 * card that loads an item for one line shares the entry with the item's page, and
 * invalidating an item reaches the queries below its key (children, tracks).
 */
export function itemQueryKey(itemId: string) {
  return [...itemsQueryKey, itemId] as const;
}
