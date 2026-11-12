import { ItemMatchMenu } from "../../identify";
import { useWatchedToggle } from "../../library";
import { MetadataAttribution } from "../../../shared/ui";
import { useHeadingFocusAfterNavigation } from "../hooks/useHeadingFocusAfterNavigation";
import { useHeadingFocusAfterRetry } from "../hooks/useHeadingFocusAfterRetry";
import { useItemChanged } from "../hooks/useItemChanged";
import { useItemDetails } from "../hooks/useItemDetails";
import { useWatchedMenuEntries } from "../hooks/useWatchedMenuEntries";
import { childOrder, type ItemSearch } from "../lib/itemSearch";
import type { ItemView } from "../lib/itemView";
import { EpisodePage } from "./EpisodePage";
import { ShowPage, SeasonPage } from "./ContainerPages";
import { ItemDetails } from "./ItemDetails";
import { ItemLoadError, ItemLoading, ItemNotFound } from "./ItemStates";
import { AlbumPage, ArtistPage } from "./MusicPages";
import { AudiobookPage } from "./AudiobookPage";
import { PhotoAlbumPage, PhotoPage } from "./PhotoPages";
import { WatchedAction } from "./WatchedAction";
import type { ReactNode, Ref } from "react";

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
 * episodes, an episode links back to both and to its neighbours, an album lists its tracks,
 * an audiobook its chapters, a photo album its sub-albums and photos, a photo itself large.
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

/** What a detail page shows next to Play, by what the kind of item supports. */
interface ItemActions {
  /** "Mark as watched"; renders nothing for kinds without it (extras, music, photos). */
  readonly watchedAction: ReactNode;
  /** The overflow menu; renders nothing for kinds that follow their parent. */
  readonly matchMenu: ReactNode;
  /** Both, for movies and shows. */
  readonly videoActions: ReactNode;
}

function useItemActions(itemId: string, item: ItemView): ItemActions {
  const onItemChanged = useItemChanged(itemId);
  // One toggle for the button and the menu entry, so both show the same state.
  const watchedToggle = useWatchedToggle({
    id: itemId,
    kind: item.kind,
    viewerState: item.viewerState,
  });
  const watchedEntries = useWatchedMenuEntries(watchedToggle);
  const watchedAction = <WatchedAction toggle={watchedToggle} />;
  const matchMenu = (
    <ItemMatchMenu
      itemId={itemId}
      kind={item.kind}
      title={item.title}
      year={item.year}
      onItemChanged={onItemChanged}
      extraEntries={watchedEntries}
    />
  );
  const videoActions = (
    <>
      {watchedAction}
      {matchMenu}
    </>
  );
  return { watchedAction, matchMenu, videoActions };
}

interface ItemByKindProps {
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly itemId: string;
  readonly item: ItemView;
  readonly search: ItemSearch;
}

function ItemByKind({ headingRef, itemId, item, search }: ItemByKindProps) {
  const { watchedAction, matchMenu, videoActions } = useItemActions(itemId, item);
  switch (item.kind) {
    case "show":
      return (
        <ShowPage headingRef={headingRef} itemId={itemId} item={item} actions={videoActions} />
      );
    case "season":
      return (
        <SeasonPage
          headingRef={headingRef}
          itemId={itemId}
          item={item}
          order={childOrder(search)}
          actions={watchedAction}
        />
      );
    case "episode":
      return <EpisodePage headingRef={headingRef} item={item} actions={watchedAction} />;
    case "album":
      return <AlbumPage headingRef={headingRef} itemId={itemId} item={item} actions={matchMenu} />;
    case "audiobook":
      return (
        <AudiobookPage headingRef={headingRef} itemId={itemId} item={item} actions={matchMenu} />
      );
    case "photo_album":
      return <PhotoAlbumPage headingRef={headingRef} itemId={itemId} item={item} />;
    case "photo":
      return <PhotoPage headingRef={headingRef} itemId={itemId} item={item} />;
    case "artist":
      return <ArtistPage headingRef={headingRef} itemId={itemId} item={item} actions={matchMenu} />;
    // The generic detail view shows the rest, tracks and kinds added in later API versions.
    case "movie":
    case "extra":
    case "track":
    default:
      return <ItemDetails headingRef={headingRef} item={item} actions={videoActions} />;
  }
}
