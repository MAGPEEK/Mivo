import type { ItemDetail } from "@mivo/api-client";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import { toPhotoDetails, type PhotoDetailsView } from "../lib/photoFacts";
import { photosNamespace } from "../locales";

/** The image, shape and translated facts of a loaded photo. */
export function usePhotoDetails(photo: ItemDetail): PhotoDetailsView {
  const { t, i18n } = useTranslation(photosNamespace);
  const language = i18n.language;
  return useMemo(
    () =>
      toPhotoDetails(photo, {
        language,
        label: (key) => t(`facts.${key}`),
        fileTime: (date) => t("facts.fileTime", { date }),
      }),
    [photo, language, t],
  );
}
