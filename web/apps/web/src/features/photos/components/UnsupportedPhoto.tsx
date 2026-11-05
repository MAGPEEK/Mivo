import { useTranslation } from "react-i18next";

import { AlertIcon } from "../../../shared/ui";
import { photosNamespace } from "../locales";

/** Props of {@link UnsupportedPhoto}. */
export interface UnsupportedPhotoProps {
  /** `tile` is a square grid tile with a short note, `view` the large box of the photo page. */
  readonly variant?: "tile" | "view";
}

/**
 * Stands in for a photo the server cannot render (thumbnailState "unsupported", ADR 0029): the
 * photo stays listed, but no image is requested for it.
 */
export function UnsupportedPhoto({ variant = "tile" }: UnsupportedPhotoProps) {
  const { t } = useTranslation(photosNamespace);
  const isTile = variant === "tile";
  return (
    <div
      data-photo-unsupported=""
      className={`flex flex-col items-center justify-center gap-2 bg-surface-raised p-3 text-center text-text-muted ${isTile ? "aspect-square text-xs" : "min-h-64 w-full rounded-lg text-sm"}`}
    >
      <AlertIcon className={isTile ? "size-6" : "size-10"} />
      <span>{isTile ? t("tile.unsupported") : t("view.unsupported")}</span>
    </div>
  );
}
