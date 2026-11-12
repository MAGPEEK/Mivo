import { useTranslation } from "react-i18next";

import type { BrowseItemTexts } from "../components/BrowseItemGrid";
import { libraryNamespace } from "../locales";

/**
 * The texts of the item grid in the folder view. The grid is shown only when the folder has
 * items, so its empty texts are never read; the folder's own empty state speaks instead.
 */
export function useFolderItemTexts(): BrowseItemTexts {
  const { t } = useTranslation(libraryNamespace);
  const emptyTitle = t("folders.empty.title");
  const emptyHint = t("folders.empty.hint");
  return {
    label: t("folders.items.label"),
    loading: t("folders.items.loading"),
    loadFailed: t("folders.items.loadFailed"),
    loadingMore: t("folders.items.loadingMore"),
    loadMoreFailed: t("folders.items.loadMoreFailed"),
    emptyTitle,
    emptyHint,
    emptyUnmatchedTitle: emptyTitle,
    emptyUnmatchedHint: emptyHint,
  };
}
