import type { LibraryFolderEntry } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";

import { IconFrame } from "../../../shared/ui";

/** Props of {@link FolderList}. */
export interface FolderListProps {
  readonly libraryId: string;
  /** Accessible name of the list. */
  readonly label: string;
  readonly folders: readonly LibraryFolderEntry[];
}

function FolderIcon() {
  return (
    <IconFrame className="size-5 shrink-0 text-accent">
      <path d="M3 6a1 1 0 0 1 1-1h5l2 2h9a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z" />
    </IconFrame>
  );
}

/** The sub-folders of the open folder, one link each; only the relative name is shown. */
export function FolderList({ libraryId, label, folders }: FolderListProps) {
  return (
    // eslint-disable-next-line jsx-a11y/no-redundant-roles -- Safari drops the list role of a list without bullets
    <ul role="list" aria-label={label} className="mb-6">
      {folders.map((folder) => (
        <li
          key={folder.path}
          className="border-b border-divider hover:bg-surface focus-within:bg-surface"
        >
          <Link
            to="/libraries/$libraryId/folders/$"
            params={{ libraryId, _splat: folder.path }}
            className="flex items-center gap-3 p-2.5"
          >
            <FolderIcon />
            <span>{folder.name}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}
