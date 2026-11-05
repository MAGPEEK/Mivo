import { imageSrcSet, imageUrl, type ImageRef, type ImageWidth } from "@mivo/api-client";

/** Props of {@link HeroBackdrop}. */
export interface HeroBackdropProps {
  readonly image: ImageRef;
}

// The hero spans the content column: up to a wide desktop window at a pixel ratio of 2.
const offeredWidths = [640, 960, 1280, 1920] as const satisfies readonly ImageWidth[];
const fallbackWidth: ImageWidth = 1280;

/**
 * The hero backdrop, covering the region. Decorative: the heading names the title. Loaded
 * eagerly because it is the largest image of Home.
 */
export function HeroBackdrop({ image }: HeroBackdropProps) {
  return (
    <img
      src={imageUrl(image, fallbackWidth)}
      srcSet={imageSrcSet(image, offeredWidths)}
      sizes="100vw"
      alt=""
      decoding="async"
      className="absolute inset-0 size-full object-cover"
    />
  );
}
