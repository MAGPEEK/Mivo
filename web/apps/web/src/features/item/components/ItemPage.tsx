import { useHeadingFocusAfterNavigation } from "../hooks/useHeadingFocusAfterNavigation";
import { useHeadingFocusAfterRetry } from "../hooks/useHeadingFocusAfterRetry";
import { useItemDetails } from "../hooks/useItemDetails";
import { childOrder, type ItemSearch } from "../lib/itemSearch";
import type { ItemView } from "../lib/itemView";
import { EpisodePage } from "./EpisodePage";
import { ShowPage, SeasonPage } from "./ContainerPages";
import { ItemDetails } from "./ItemDetails";
import { ItemLoadError, ItemLoading, ItemNotFound } from "./ItemStates";
import type { Ref } from "react";

/** Props of {@link ItemPage}. */
export interface ItemPageProps {
  /** Item from the route `/items/$itemId`. */
  readonly itemId: string;
  /** Validated search of the route; defaults to none. */
  readonly search?: ItemSearch;
}

/**
 * Item page: shows what the kind of the item calls for. A movie or extra is what a video is
 * technically, its tracks and the Play button; a show lists its seasons, a season its
 * episodes, an episode links back to both and to its neighbours.
 */
export function ItemPage({ itemId, search = {} }: ItemPageProps) {
  const state = useItemDetails(itemId);
  const { headingRef, focusAfter } = useHeadingFocusAfterRetry(state);
  useHeadingFocusAfterNavigation(itemId, state.status === "ready", headingRef);
  switch (state.status) {
    case "loading":
      return <ItemLoading />;
    case "notFound":
      return <ItemNotFound />;
    case "error":
      return (
        <ItemLoadError
          headingRef={headingRef}
          failure={state.failure}
          onRetry={focusAfter(state.retry)}
        />
      );
    case "ready":
      return (
        <ItemByKind headingRef={headingRef} itemId={itemId} item={state.item} search={search} />
      );
  }
}

interface ItemByKindProps {
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly itemId: string;
  readonly item: ItemView;
  readonly search: ItemSearch;
}

function ItemByKind({ headingRef, itemId, item, search }: ItemByKindProps) {
  switch (item.kind) {
    case "show":
      return <ShowPage headingRef={headingRef} itemId={itemId} item={item} />;
    case "season":
      return (
        <SeasonPage
          headingRef={headingRef}
          itemId={itemId}
          item={item}
          order={childOrder(search)}
        />
      );
    case "episode":
      return <EpisodePage headingRef={headingRef} itemId={itemId} item={item} />;
    case "movie":
    case "extra":
      return <ItemDetails headingRef={headingRef} item={item} />;
  }
}
