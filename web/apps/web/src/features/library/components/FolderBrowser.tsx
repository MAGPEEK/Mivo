import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { EmptyState, Skeleton } from "../../../shared/ui";
import { useFolderBrowser } from "../hooks/useFolderBrowser";
import type { FolderListing } from "../hooks/useFolderListing";
import { useFolderHeadingFocus } from "../hooks/useFolderHeadingFocus";
import { useFolderItemTexts } from "../hooks/useFolderTexts";
import { useFocusOnMount } from "../hooks/useFocusOnMount";
import { useViewPreference } from "../hooks/useViewPreference";
import { errorCodeOf } from "../lib/errorKeys";
import { parseFolderPath } from "../lib/folderPath";
import { libraryNamespace } from "../locales";
import { BrowseItemGrid } from "./BrowseItemGrid";
import { primaryButtonClass, secondaryButtonClass } from "./buttonClasses";
import { FolderCrumbs } from "./FolderCrumbs";
import { FolderList } from "./FolderList";
import { ViewControls } from "./ViewControls";

/** Props of {@link FolderBrowser}. */
export interface FolderBrowserProps {
  /** Library from the route `/libraries/$libraryId/folders/*`. */
  readonly libraryId: string;
  /** The splat of the route: the folder path below the library; absent or empty is the top. */
  readonly splat: string | undefined;
}

const headingClass = "text-3xl font-semibold";

/**
 * The folder view of a library (LB-21): breadcrumb, sub-folders and the items of one folder. The
 * folder is the URL, so Back, Forward and a reload restore it. A path that cannot be a folder
 * (`..`, absolute, empty segments) shows the not-found state without a request.
 */
export function FolderBrowser({ libraryId, splat }: FolderBrowserProps) {
  const parsed = parseFolderPath(splat);
  if (!parsed.valid) {
    return <FolderNotFound libraryId={libraryId} />;
  }
  return <FolderContent libraryId={libraryId} path={parsed.path} segments={parsed.segments} />;
}

interface FolderContentProps {
  readonly libraryId: string;
  readonly path: string;
  readonly segments: readonly string[];
}

function FolderContent({ libraryId, path, segments }: FolderContentProps) {
  const { t } = useTranslation(libraryNamespace);
  const { library, listing, crumbs } = useFolderBrowser(libraryId, path, segments);
  const focusHeading = useFolderHeadingFocus(path);
  if (
    listing.status === "notFound" ||
    (library.status === "error" && errorCodeOf(library.error) === "not_found")
  ) {
    return <FolderNotFound libraryId={libraryId} />;
  }
  const libraryName = library.status === "success" ? library.library.name : t("heading");
  return (
    <>
      <h1 className={`mb-4 ${headingClass}`}>{libraryName}</h1>
      <FolderCrumbs libraryId={libraryId} libraryName={libraryName} crumbs={crumbs} />
      <FolderListingView
        libraryId={libraryId}
        listing={library.status === "error" ? { status: "error", retry: library.retry } : listing}
        scrollKey={`${libraryId}|folders|${path}`}
        title={crumbs.at(-1)?.name ?? t("folders.top")}
        focusHeading={focusHeading}
      />
    </>
  );
}

interface FolderListingViewProps {
  readonly libraryId: string;
  readonly listing: FolderListing;
  readonly scrollKey: string;
  /** Name of the open folder, the heading of the listing. */
  readonly title: string;
  /** Whether the heading takes focus when it appears (the user moved to this folder). */
  readonly focusHeading: boolean;
}

function FolderListingView({
  libraryId,
  listing,
  scrollKey,
  title,
  focusHeading,
}: FolderListingViewProps) {
  const { t } = useTranslation(libraryNamespace);
  if (listing.status === "pending") {
    return (
      <div role="status" className="w-60">
        <Skeleton label={t("folders.loading")} className="h-5 w-full" />
      </div>
    );
  }
  if (listing.status === "error") {
    return (
      <div className="flex flex-wrap items-center gap-4">
        <p role="alert" className="text-text-muted">
          {t("folders.loadFailed")}
        </p>
        <button type="button" onClick={listing.retry} className={secondaryButtonClass}>
          {t("retry")}
        </button>
      </div>
    );
  }
  if (listing.status === "notFound") {
    return <FolderNotFound libraryId={libraryId} />;
  }
  return (
    <LoadedFolder
      libraryId={libraryId}
      listing={listing}
      scrollKey={scrollKey}
      title={title}
      focusHeading={focusHeading}
    />
  );
}

type LoadedListing = Extract<FolderListing, { status: "success" }>;

interface LoadedFolderProps {
  readonly libraryId: string;
  readonly listing: LoadedListing;
  readonly scrollKey: string;
  readonly title: string;
  readonly focusHeading: boolean;
}

// Paging stopped while the loaded pages hold only folders, so the grid that would show the
// failure is not there: say it here and offer the retry.
function ItemsLoadFailed({ onRetry }: { readonly onRetry: () => void }) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <div className="flex flex-wrap items-center gap-4">
      <p role="alert" className="text-text-muted">
        {t("folders.items.loadMoreFailed")}
      </p>
      <button type="button" onClick={onRetry} className={secondaryButtonClass}>
        {t("retry")}
      </button>
    </div>
  );
}

function LoadedFolder({ libraryId, listing, scrollKey, title, focusHeading }: LoadedFolderProps) {
  const { t } = useTranslation(libraryNamespace);
  const view = useViewPreference();
  const texts = useFolderItemTexts();
  const headingRef = useFocusOnMount<HTMLHeadingElement>(focusHeading);
  const { folders, items } = listing;
  const hasItems = items.items.length > 0;
  // Complete only when no page is left, so a half-loaded folder never claims to be empty.
  if (folders.length === 0 && !hasItems && !items.hasMore) {
    return <EmptyState title={t("folders.empty.title")} description={t("folders.empty.hint")} />;
  }
  return (
    <>
      <h2 ref={headingRef} tabIndex={-1} className="mb-3 text-xl font-semibold outline-none">
        {title}
      </h2>
      {folders.length > 0 && (
        <FolderList libraryId={libraryId} label={t("folders.list")} folders={folders} />
      )}
      {!hasItems && items.loadMoreFailed && !items.loadingMore && (
        <ItemsLoadFailed onRetry={items.loadMore} />
      )}
      {hasItems && (
        <>
          <div
            role="group"
            aria-label={t("folders.viewOptions")}
            className="mb-4 flex flex-wrap gap-3"
          >
            <ViewControls view={view} />
          </div>
          <BrowseItemGrid
            items={items}
            filter="all"
            view={view}
            texts={texts}
            scrollKey={scrollKey}
          />
        </>
      )}
    </>
  );
}

interface FolderNotFoundProps {
  readonly libraryId: string;
}

function FolderNotFound({ libraryId }: FolderNotFoundProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <>
      <h1 className={`mb-6 ${headingClass}`}>{t("folders.heading")}</h1>
      <EmptyState
        title={t("folders.notFound.title")}
        description={t("folders.notFound.hint")}
        action={
          <Link to="/libraries/$libraryId" params={{ libraryId }} className={primaryButtonClass}>
            {t("folders.notFound.action")}
          </Link>
        }
      />
    </>
  );
}
