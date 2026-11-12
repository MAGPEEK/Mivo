import type { ItemSummary } from "@mivo/api-client";

import type { CardSize } from "../lib/viewStyle";

/** What a Browse grid hands to its card renderer. */
export interface BrowseCardProps {
  readonly item: ItemSummary;
  /**
   * Always `false` since the grid is virtual (M5-34): it moves the focus itself, to a card that
   * is already mounted or as soon as it is. Kept so existing card renderers keep compiling.
   */
  readonly focusOnMount: boolean;
  /** The card size of the size slider; `undefined` keeps the card's own fixed sizes. */
  readonly size?: CardSize | undefined;
}
