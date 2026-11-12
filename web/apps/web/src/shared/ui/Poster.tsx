import { imageSrcSet, imageUrl, type ImageRef, type ImageWidth } from "@mivo/api-client";
import { useState, type ReactNode } from "react";

import { FilmIcon } from "./icons";

/** Box shape of a {@link Poster}: `poster` is 2:3 portrait, `still` a 16:9 frame, `square` 1:1. */
export type PosterAspect = "poster" | "still" | "square";

/** Props of {@link Poster}. */
export interface PosterProps {
  /** The image; without one, or when it fails to load, the placeholder shows. */
  readonly imageRef?: ImageRef | undefined;
  readonly aspect: PosterAspect;
  /**
   * Accessible name of the image and of the placeholder, usually the item title. An empty
   * string makes the artwork decorative, e.g. next to a heading that already names the item.
   */
  readonly alt: string;
  /** `sizes` attribute: how wide the box is displayed, so the browser picks a width. */
  readonly sizes: string;
  /** Width of the plain `src` for browsers without `srcset`; defaults to a typical card. */
  readonly fallbackWidth?: ImageWidth | undefined;
  /** Icon on the placeholder, e.g. the item kind; a film strip by default. */
  readonly icon?: ReactNode;
  /** Extra classes for the box, e.g. its width or rounding. */
  readonly className?: string;
}

// Cards and the detail page are at most a few hundred CSS pixels wide; 960 covers them at a
// device pixel ratio of 3 without offering the backdrop-sized renditions.
const offeredWidths = [160, 320, 480, 640, 960] as const satisfies readonly ImageWidth[];
// The plain `src` for browsers that ignore `srcset`: a typical card at pixel ratio 2.
const defaultFallbackWidth: ImageWidth = 320;

const aspectClasses: Readonly<Record<PosterAspect, string>> = {
  poster: "aspect-2/3",
  still: "aspect-video",
  square: "aspect-square",
};

function isSameImage(left: ImageRef, right: ImageRef | undefined): boolean {
  return left.id === right?.id && left.tag === right.tag;
}

/**
 * Artwork in a fixed aspect box, so nothing shifts while it loads. Shows a neutral
 * placeholder with an icon when there is no image or it cannot be loaded.
 */
export function Poster({
  imageRef,
  aspect,
  alt,
  sizes,
  fallbackWidth = defaultFallbackWidth,
  icon,
  className = "",
}: PosterProps) {
  // The failed image is remembered by id and tag: callers may rebuild equal references on every
  // render, and a new tag of the same id is a new image that deserves another try.
  const [failedRef, setFailedRef] = useState<ImageRef | undefined>(undefined);
  const showImage = imageRef !== undefined && !isSameImage(imageRef, failedRef);
  const placeholderAccess =
    alt === "" ? { "aria-hidden": true } : { role: "img", "aria-label": alt };
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
          {...placeholderAccess}
          data-poster-placeholder=""
          className="grid size-full place-items-center text-text-muted"
        >
          {icon ?? <FilmIcon className="size-1/4" />}
        </div>
      )}
    </div>
  );
}
