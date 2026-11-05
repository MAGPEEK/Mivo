import type { Ref } from "react";

import { PhotoAlbumContents, PhotoView } from "../../photos";
import { useItem } from "../hooks/useItem";
import type { ItemView } from "../lib/itemView";
import { AncestorLinks } from "./ItemNavigation";
import { PlayAction } from "./PlayAction";

/** Props of the photo pages {@link PhotoAlbumPage} and {@link PhotoPage}. */
export interface PhotoPageProps {
  /** Receives the title heading, so focus can move to it after a retry or navigation. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly itemId: string;
  readonly item: ItemView;
}

const headingClass = "text-3xl font-semibold break-words focus-visible:outline-none";
const factSeparator = " · ";

/**
 * A photo album: the albums above it, its name and photo count, then its sub-albums and its
 * photos as square tiles.
 */
export function PhotoAlbumPage({ headingRef, itemId, item }: PhotoPageProps) {
  return (
    <article className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <AncestorLinks ancestors={item.ancestors} />
        <h1 ref={headingRef} tabIndex={-1} className={headingClass}>
          {item.title}
        </h1>
        {item.facts.length > 0 && (
          <p className="text-sm text-text-muted">{item.facts.join(factSeparator)}</p>
        )}
      </div>
      <PhotoAlbumContents albumId={itemId} />
    </article>
  );
}

/**
 * One photo: the albums above it, its name, the photo large in its own aspect ratio and its
 * capture facts. A video of the library offers Play instead of the still.
 */
export function PhotoPage({ headingRef, itemId, item }: PhotoPageProps) {
  // The page only renders once the item has loaded, so this reads the cached entry; the photo
  // view needs the EXIF block and image the item view does not keep.
  const photo = useItem(itemId).data;
  return (
    <article className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <AncestorLinks ancestors={item.ancestors} />
        <h1 ref={headingRef} tabIndex={-1} className={headingClass}>
          {item.title}
        </h1>
      </div>
      {item.play.kind === "playable" && <PlayAction play={item.play} />}
      {photo !== undefined && <PhotoView photo={photo} />}
    </article>
  );
}
