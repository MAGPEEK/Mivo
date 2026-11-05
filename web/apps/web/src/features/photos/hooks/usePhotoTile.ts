import type { ItemSummary } from "@mivo/api-client";
import type { TFunction } from "i18next";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import { formatDuration } from "../../library";
import { toPhotoTile, type PhotoTileModel } from "../lib/photoTile";
import { photosNamespace } from "../locales";

/** A {@link PhotoTileModel} ready to render: the label names a video, the badge its length. */
export interface PhotoTileView extends PhotoTileModel {
  /** Clock notation of a video's length, e.g. `0:03`; undefined for a still photo. */
  readonly durationText: string | undefined;
}

/** The tile of a listed photo, with its date in the UI language. */
export function usePhotoTile(photo: ItemSummary): PhotoTileView {
  const { t, i18n } = useTranslation(photosNamespace);
  const language = i18n.language;
  return useMemo(() => {
    const tile = toPhotoTile(photo, language);
    const isVideo = tile.durationSeconds !== undefined;
    return {
      ...tile,
      label: tileLabel(tile, t),
      durationText: isVideo ? formatDuration(tile.durationSeconds) : undefined,
    };
  }, [photo, language, t]);
}

// The tile's note and badge are hidden from assistive technology, so the label says them.
function tileLabel(tile: PhotoTileModel, t: TFunction<typeof photosNamespace>): string {
  if (tile.unsupported) {
    return t("tile.unsupportedLabel", { label: tile.label });
  }
  return tile.durationSeconds === undefined ? tile.label : t("tile.video", { label: tile.label });
}
