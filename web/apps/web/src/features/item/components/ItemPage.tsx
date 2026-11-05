import { ItemMatchMenu } from "../../identify";
import { MetadataAttribution } from "../../../shared/ui";
import { useHeadingFocusAfterNavigation } from "../hooks/useHeadingFocusAfterNavigation";
import { useHeadingFocusAfterRetry } from "../hooks/useHeadingFocusAfterRetry";
import { useItemChanged } from "../hooks/useItemChanged";
import { useItemDetails } from "../hooks/useItemDetails";
import { childOrder, type ItemSearch } from "../lib/itemSearch";
import type { ItemView } from "../lib/itemView";
import { EpisodePage } from "./EpisodePage";
import { ShowPage, SeasonPage } from "./ContainerPages";
import { ItemDetails } from "./ItemDetails";
import { ItemLoadError, ItemLoading, ItemNotFound } from "./ItemStates";
import { AlbumPage } from "./MusicPages";
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
 * episodes, an episode links back to both and to its neighbours, an album lists its tracks.
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
        <>
          <ItemByKind headingRef={headingRef} itemId={itemId} item={state.item} search={search} />
          {/* Credits the providers the shown data came from (ADR 0031 section 4). */}
          <MetadataAttribution
            sources={state.item.metadataSources}
            className="mt-12 border-t border-divider pt-4"
          />
        </>
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
  const onItemChanged = useItemChanged(itemId);
  // The menu decides by kind itself: it renders nothing for kinds that follow their parent
  // (seasons, episodes, extras, tracks).
  const matchMenu = (
    <ItemMatchMenu
      itemId={itemId}
      kind={item.kind}
      title={item.title}
      year={item.year}
      onItemChanged={onItemChanged}
    />
  );
  switch (item.kind) {
    case "show":
      return <ShowPage headingRef={headingRef} itemId={itemId} item={item} actions={matchMenu} />;
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
    case "album":
      return <AlbumPage headingRef={headingRef} itemId={itemId} item={item} actions={matchMenu} />;
    // Artists get their own page with the full music view (M5); until then the generic
    // detail view shows them, tracks and kinds added in later API versions (default).
    case "movie":
    case "extra":
    case "artist":
    case "track":
    default:
      return <ItemDetails headingRef={headingRef} item={item} actions={matchMenu} />;
  }
}
