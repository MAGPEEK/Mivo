import { imageSrcSet, imageUrl, type ImageRef, type ImageWidth } from "@mivo/api-client";
import { useState } from "react";

import {
  backdropFadeClass,
  backdropImageClass,
  backdropScrimClass,
  plainBackdropClass,
} from "../lib/backdropScrim";

/** Props of {@link Backdrop}. */
export interface BackdropProps {
  /** The image to blur behind the header; `undefined` shows the plain background. */
  readonly imageRef: ImageRef | undefined;
}

// The image is blurred heavily, so a rendition at the full display width adds nothing; the
// widths stay inside the allowed set (LB-44).
const offeredWidths = [480, 640, 960, 1280] as const satisfies readonly ImageWidth[];
const fallbackWidth: ImageWidth = 960;

function isSameImage(left: ImageRef, right: ImageRef | undefined): boolean {
  return left.id === right?.id && left.tag === right.tag;
}

/**
 * The decorative layer behind the detail header (DT-01): the image blurred, dimmed and covered
 * by the scrim token so text on it keeps AA contrast, fading into the page at the bottom.
 * Without an image, or when it fails to load, a plain background without blur shows.
 */
export function Backdrop({ imageRef }: BackdropProps) {
  const [failedRef, setFailedRef] = useState<ImageRef | undefined>(undefined);
  const showImage = imageRef !== undefined && !isSameImage(imageRef, failedRef);
  return (
    <div
      aria-hidden="true"
      data-backdrop={showImage ? "image" : "plain"}
      className="absolute inset-0 -z-10 overflow-hidden"
    >
      {showImage ? (
        <>
          <img
            src={imageUrl(imageRef, fallbackWidth)}
            srcSet={imageSrcSet(imageRef, offeredWidths)}
            sizes="100vw"
            alt=""
            loading="lazy"
            decoding="async"
            onError={() => {
              setFailedRef(imageRef);
            }}
            className={backdropImageClass}
          />
          <div data-backdrop-scrim="" className={`absolute inset-0 ${backdropScrimClass}`} />
          <div className={`absolute inset-0 ${backdropFadeClass}`} />
        </>
      ) : (
        <div className={`size-full ${plainBackdropClass}`} />
      )}
    </div>
  );
}
