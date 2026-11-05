import {
  imageSrcSet,
  imageUrl,
  type ImageRef,
  type ImageWidth,
  type ItemDetail,
} from "@mivo/api-client";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { usePhotoDetails } from "../hooks/usePhotoDetails";
import type { PhotoFact } from "../lib/photoFacts";
import { photosNamespace } from "../locales";
import { UnsupportedPhoto } from "./UnsupportedPhoto";

/** Props of {@link PhotoView}. */
export interface PhotoViewProps {
  /** The loaded photo item. */
  readonly photo: ItemDetail;
}

// The photo fills the content column up to the viewport height; 1920 covers a full-width
// column on a large screen, the smaller widths serve phones and narrow windows.
const offeredWidths = [640, 960, 1280, 1920] as const satisfies readonly ImageWidth[];
const fallbackWidth: ImageWidth = 1280;
const photoSizes = "(min-width: 1024px) 70vw, 100vw";

/**
 * A photo shown large in its own aspect ratio, with its capture date, camera and the other
 * EXIF facts below. The lightbox with zoom, swiping and the slideshow comes in milestone 5.
 */
export function PhotoView({ photo }: PhotoViewProps) {
  const details = usePhotoDetails(photo);
  return (
    <div className="flex flex-col gap-6">
      {/* A video has no still to show; its page offers Play. */}
      {!details.isVideo && (
        <PhotoImage image={details.image} alt={photo.title} aspectRatio={details.aspectRatio} />
      )}
      <PhotoFacts facts={details.facts} />
    </div>
  );
}

interface PhotoImageProps {
  readonly image: ImageRef | undefined;
  readonly alt: string;
  readonly aspectRatio: string | undefined;
}

function PhotoImage({ image, alt, aspectRatio }: PhotoImageProps) {
  if (image === undefined) {
    return <UnsupportedPhoto variant="view" />;
  }
  // Keyed by the image, so a new photo gets a fresh load state.
  return (
    <LargePhoto
      key={`${image.id}-${image.tag}`}
      image={image}
      alt={alt}
      aspectRatio={aspectRatio}
    />
  );
}

interface LargePhotoProps {
  readonly image: ImageRef;
  readonly alt: string;
  readonly aspectRatio: string | undefined;
}

function LargePhoto({ image, alt, aspectRatio }: LargePhotoProps) {
  const { t } = useTranslation(photosNamespace);
  const [failed, setFailed] = useState(false);
  if (failed) {
    return (
      <p role="alert" className="rounded-lg bg-surface-raised p-6 text-text-muted">
        {t("view.loadFailed")}
      </p>
    );
  }
  return (
    <figure className="flex justify-center rounded-lg bg-surface p-2">
      <img
        src={imageUrl(image, fallbackWidth)}
        srcSet={imageSrcSet(image, offeredWidths)}
        sizes={photoSizes}
        alt={alt}
        decoding="async"
        // The upright size from the EXIF block reserves the space before the image arrives.
        style={aspectRatio === undefined ? undefined : { aspectRatio }}
        onError={() => {
          setFailed(true);
        }}
        className="max-h-[75vh] max-w-full rounded-md object-contain"
      />
    </figure>
  );
}

function PhotoFacts({ facts }: { readonly facts: readonly PhotoFact[] }) {
  const { t } = useTranslation(photosNamespace);
  if (facts.length === 0) {
    return null;
  }
  return (
    <section aria-label={t("view.details")}>
      <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
        {facts.map((fact) => (
          <div key={fact.key} className="contents">
            <dt className="text-text-muted">{fact.label}</dt>
            <dd className="break-words text-text">{fact.value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
