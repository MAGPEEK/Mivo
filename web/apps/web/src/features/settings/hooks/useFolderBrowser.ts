import type { FolderListing, FolderRoot, FolderRootList } from "@mivo/api-client";
import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import { useState } from "react";

import { useApiClient } from "../../../shared/api";
import { folderListingQueryOptions, folderRootsQueryOptions } from "../api/libraryManagement";

/** Loads the folder roots and the installation facts (kind, service user). */
export function useFolderRoots(): UseQueryResult<FolderRootList> {
  const client = useApiClient();
  return useQuery(folderRootsQueryOptions(client));
}

/** One step of the breadcrumb: a label and the path (relative to the root) it leads to. */
export interface Crumb {
  readonly label: string;
  readonly path: string;
}

/** State of the folder browser dialog. */
export interface FolderBrowser {
  readonly roots: UseQueryResult<FolderRootList>;
  /** The root being browsed; undefined until the roots arrive or when none is available. */
  readonly root: FolderRoot | undefined;
  /** Current folder relative to the root; empty for the root itself. */
  readonly path: string;
  readonly listing: UseQueryResult<FolderListing>;
  /** Root first, then every folder down to the current one. */
  readonly crumbs: readonly Crumb[];
  readonly chooseRoot: (rootId: string) => void;
  /** Shows the folder at `path` (relative to the root). */
  readonly goTo: (path: string) => void;
  readonly up: () => void;
}

function parentOf(path: string): string {
  return path.split("/").slice(0, -1).join("/");
}

function crumbsOf(root: FolderRoot | undefined, path: string): Crumb[] {
  const segments = path === "" ? [] : path.split("/");
  const steps = segments.map((segment, index) => ({
    label: segment,
    path: segments.slice(0, index + 1).join("/"),
  }));
  return [{ label: root?.path ?? "", path: "" }, ...steps];
}

/** Drives the folder browser: root choice, current path, and the listing query for both. */
export function useFolderBrowser(): FolderBrowser {
  const client = useApiClient();
  const roots = useFolderRoots();
  const [chosenRootId, setChosenRootId] = useState<string | null>(null);
  const [path, setPath] = useState("");
  const available = roots.data?.roots.filter((candidate) => candidate.available) ?? [];
  const root = available.find((candidate) => candidate.id === chosenRootId) ?? available[0];
  const listing = useQuery({
    ...folderListingQueryOptions(client, { rootId: root?.id ?? "", path }),
    enabled: root !== undefined,
  });
  return {
    roots,
    root,
    path,
    listing,
    crumbs: crumbsOf(root, path),
    chooseRoot: (rootId) => {
      setChosenRootId(rootId);
      setPath("");
    },
    goTo: setPath,
    up: () => {
      setPath(parentOf(path));
    },
  };
}
