/** Sort direction of the children of a season. */
export type ChildOrder = "asc" | "desc";

/** Validated search parameters of the item route `/items/$itemId`. */
export interface ItemSearch {
  /** `desc` when the episode list is reversed; ascending (the default) is left out of the URL. */
  readonly order?: "desc" | undefined;
}

/**
 * Validates the parsed search parameters of the item route. Unknown values are dropped, so a
 * hand-edited link still opens the page. The key is always returned: the router lays the
 * result over the raw search, so a dropped value must be overwritten with `undefined`.
 */
export function validateItemSearch(search: Readonly<Record<string, unknown>>): ItemSearch {
  return { order: search["order"] === "desc" ? "desc" : undefined };
}

/** The sort direction the search asks for. */
export function childOrder(search: ItemSearch): ChildOrder {
  return search.order === "desc" ? "desc" : "asc";
}

/** The search that selects `order`; the default direction leaves the URL clean. */
export function searchForOrder(order: ChildOrder): ItemSearch {
  return { order: order === "desc" ? "desc" : undefined };
}

/** The direction the order toggle switches to. */
export function oppositeOrder(order: ChildOrder): ChildOrder {
  return order === "asc" ? "desc" : "asc";
}
