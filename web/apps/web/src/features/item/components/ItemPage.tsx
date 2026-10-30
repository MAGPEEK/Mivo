import { useHeadingFocusAfterRetry } from "../hooks/useHeadingFocusAfterRetry";
import { useItemDetails } from "../hooks/useItemDetails";
import { ItemDetails } from "./ItemDetails";
import { ItemLoadError, ItemLoading, ItemNotFound } from "./ItemStates";

/** Props of {@link ItemPage}. */
export interface ItemPageProps {
  /** Item from the route `/items/$itemId`. */
  readonly itemId: string;
}

/** Item detail page: what a video is technically, its tracks and the Play button. */
export function ItemPage({ itemId }: ItemPageProps) {
  const state = useItemDetails(itemId);
  const { headingRef, focusAfter } = useHeadingFocusAfterRetry(state);
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
      return <ItemDetails headingRef={headingRef} itemId={itemId} item={state.item} />;
  }
}
