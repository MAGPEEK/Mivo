import { imageSrcSet, imageUrl, type ImageRef, type ImageWidth } from "@mivo/api-client";
import { useState, type ReactNode } from "react";

import { FilmIcon } from "./icons";

/** Box shape of a {@link Poster}: `poster` is 2:3 portrait, `still` is a 16:9 frame. */
export type PosterAspect = "poster" | "still";

/** Props of {@link Poster}. */
export interface PosterProps {
  /** The image; without one, or when it fails to load, the placeholder shows. */
  readonly imageRef?: ImageRef | undefined;
  readonly aspect: PosterAspect;
  /** Accessible name of the image and of the placeholder, usually the item title. */
  readonly alt: string;
  /** `sizes` attribute: how wide the box is displayed, so the browser picks a width. */
  readonly sizes: string;
  /** Icon on the placeholder, e.g. the item kind; a film strip by default. */
  readonly icon?: ReactNode;
  /** Extra classes for the box, e.g. its width or rounding. */
  readonly className?: string;
}

// Cards and the detail page are at most a few hundred CSS pixels wide; 960 covers them at a
// device pixel ratio of 3 without offering the backdrop-sized renditions.
const offeredWidths = [160, 320, 480, 640, 960] as const satisfies readonly ImageWidth[];
// The plain `src` for browsers that ignore `srcset`: a typical card at pixel ratio 2.
const fallbackWidth: ImageWidth = 320;

const aspectClasses: Readonly<Record<PosterAspect, string>> = {
  poster: "aspect-2/3",
  still: "aspect-video",
};

/**
 * Artwork in a fixed aspect box, so nothing shifts while it loads. Shows a neutral
 * placeholder with an icon when there is no image or it cannot be loaded.
 */
export function Poster({ imageRef, aspect, alt, sizes, icon, className = "" }: PosterProps) {
  // The failed image is remembered by reference, so a new image is tried again.
  const [failedRef, setFailedRef] = useState<ImageRef | undefined>(undefined);
  const showImage = imageRef !== undefined && imageRef !== failedRef;
  return (
    <div
      className={`${aspectClasses[aspect]} relative overflow-hidden bg-surface-raised ${className}`}
    >
      {showImage ? (
        <img
          src={imageUrl(imageRef, fallbackWidth)}
          srcSet={imageSrcSet(imageRef, offeredWidths)}
          sizes={sizes}
          alt={alt}
          loading="lazy"
          decoding="async"
          onError={() => {
            setFailedRef(imageRef);
          }}
          className="size-full object-cover"
        />
      ) : (
        <div
          role="img"
          aria-label={alt}
          data-poster-placeholder=""
          className="grid size-full place-items-center text-text-muted"
        >
          {icon ?? <FilmIcon className="size-1/4" />}
        </div>
      )}
    </div>
  );
}
