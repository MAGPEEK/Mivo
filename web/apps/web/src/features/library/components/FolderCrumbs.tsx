import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import type { FolderCrumb } from "../lib/folderPath";
import { libraryNamespace } from "../locales";

/** Props of {@link FolderCrumbs}. */
export interface FolderCrumbsProps {
  readonly libraryId: string;
  readonly libraryName: string;
  /** The folders from the first one below the library down to the open one. */
  readonly crumbs: readonly FolderCrumb[];
}

const crumbLinkClass = "rounded-sm hover:underline";

/** Breadcrumb from the library root to the open folder: a link for every ancestor (LB-21). */
export function FolderCrumbs({ libraryId, libraryName, crumbs }: FolderCrumbsProps) {
  const { t } = useTranslation(libraryNamespace);
  const last = crumbs.length - 1;
  return (
    <nav aria-label={t("folders.breadcrumb")} className="mb-4">
      <ol className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
        <li>
          <Link
            to="/libraries/$libraryId/folders/$"
            params={{ libraryId, _splat: "" }}
            aria-current={crumbs.length === 0 ? "page" : undefined}
            className={crumbLinkClass}
          >
            {libraryName}
          </Link>
        </li>
        {crumbs.map((crumb, index) => (
          <li key={crumb.path} className="flex items-center gap-2">
            <span aria-hidden="true" className="text-text-muted">
              /
            </span>
            <Link
              to="/libraries/$libraryId/folders/$"
              params={{ libraryId, _splat: crumb.path }}
              aria-current={index === last ? "page" : undefined}
              className={crumbLinkClass}
            >
              {crumb.name}
            </Link>
          </li>
        ))}
      </ol>
    </nav>
  );
}
