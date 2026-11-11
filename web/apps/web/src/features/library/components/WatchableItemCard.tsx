import type { ItemSummary } from "@mivo/api-client";

import { useWatchedToggle } from "../hooks/useWatchedToggle";
import { ItemCard } from "./ItemCard";

/** Props of {@link WatchableItemCard}. */
export interface WatchableItemCardProps {
  readonly item: ItemSummary;
  /** Focus the card link when it mounts: the first card of a page "Load more" loaded. */
  readonly focusOnMount: boolean;
}

/**
 * The poster card of the Browse grid with the hover toggle "Mark as watched" (LB-41). Row
 * cards stay plain `ItemCard`s: a row is one tab stop, a button per card would break that.
 */
export function WatchableItemCard({ item, focusOnMount }: WatchableItemCardProps) {
  const watchedToggle = useWatchedToggle(item);
  return <ItemCard item={item} focusOnMount={focusOnMount} watchedToggle={watchedToggle} />;
}
