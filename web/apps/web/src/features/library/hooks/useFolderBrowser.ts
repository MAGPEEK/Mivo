import type { Library } from "@mivo/api-client";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import { folderBasename, folderCrumbs, type FolderCrumb } from "../lib/folderPath";
import { libraryNamespace } from "../locales";
import { useFolderListing, type FolderListing } from "./useFolderListing";
import { useLibrary, type LibraryState } from "./useLibrary";

/** What the folder view needs for one folder: the library, the folder's children, the crumbs. */
export interface FolderBrowser {
  readonly library: LibraryState;
  readonly listing: FolderListing;
  /** One crumb per folder below the library root; empty at the top. */
  readonly crumbs: readonly FolderCrumb[];
}

// The first path segment is the id of one of the library's folders; its name is the last name of
// the server path. The absolute path itself is never shown.
// Until the library has loaded (or if it does not know the id) a neutral word stands in: the
// raw id is a UUID that means nothing to the user.
function topFolderName(
  library: Library | undefined,
  folderId: string | undefined,
  placeholder: string,
): string {
  const folder = library?.folders.find((candidate) => candidate.id === folderId);
  return folder === undefined ? placeholder : folderBasename(folder.path);
}

/**
 * Loads the library and the children of its folder `path`, and builds the breadcrumb from the
 * validated `segments` of that path.
 */
export function useFolderBrowser(
  libraryId: string,
  path: string,
  segments: readonly string[],
): FolderBrowser {
  const library = useLibrary(libraryId);
  const listing = useFolderListing(libraryId, path);
  const loaded = library.status === "success" ? library.library : undefined;
  const { t } = useTranslation(libraryNamespace);
  const placeholder = t("folders.unnamed");
  const crumbs = useMemo(
    () => folderCrumbs(segments, topFolderName(loaded, segments[0], placeholder)),
    [loaded, segments, placeholder],
  );
  return { library, listing, crumbs };
}
