import { useTranslation } from "react-i18next";

import type { SidebarLibraries } from "../../../shared/ui";
import { LibraryKindIcon } from "../components/LibraryKindIcon";
import { libraryNamespace } from "../locales";
import { useLibraries } from "./useLibraries";

/**
 * The libraries section of the sidebar: one link per library with its kind icon and a note
 * when its folder is unavailable, or the loading, error or empty state with translated text.
 */
export function useSidebarLibraries(): SidebarLibraries {
  const { t } = useTranslation(libraryNamespace);
  const libraries = useLibraries();
  if (libraries.isPending) {
    return { state: "loading", message: t("sidebar.loading") };
  }
  if (libraries.isError) {
    return { state: "error", message: t("sidebar.loadFailed") };
  }
  return {
    state: "loaded",
    links: libraries.data.map((library) => ({
      id: library.id,
      name: library.name,
      icon: <LibraryKindIcon kind={library.kind} className="size-5 shrink-0" />,
      note: library.available ? undefined : t("sidebar.unavailable"),
    })),
  };
}
