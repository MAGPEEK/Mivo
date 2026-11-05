import type { ReactNode, Ref } from "react";

import { ChapterList } from "../../audiobooks";
import { useItem } from "../hooks/useItem";
import type { ItemView } from "../lib/itemView";
import { ItemHeader } from "./ItemHeader";

/** Props of {@link AudiobookPage}. */
export interface AudiobookPageProps {
  /** Receives the title heading, so focus can move to it after a retry or navigation. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly itemId: string;
  readonly item: ItemView;
  /** Shown next to Play, e.g. the overflow menu with the match actions. */
  readonly actions?: ReactNode;
}

/**
 * An audiobook: 2:3 cover, authors, narrators, length and chapter count, Play from the start
 * and the chapters, each of which starts playback at its own offset.
 */
export function AudiobookPage({ headingRef, itemId, item, actions }: AudiobookPageProps) {
  // The page only renders once the item has loaded, so this reads the cached entry; the
  // chapter list needs the raw files and chapters the view does not keep.
  const book = useItem(itemId).data;
  return (
    <article>
      <ItemHeader headingRef={headingRef} item={item} aspect="poster" actions={actions} />
      {book !== undefined && <ChapterList book={book} />}
    </article>
  );
}
