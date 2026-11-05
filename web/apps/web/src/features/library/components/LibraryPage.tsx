import type { Library } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { AlertIcon, Badge, Skeleton, Tabs } from "../../../shared/ui";
import type { BrowseSearchChange } from "../hooks/useBrowseState";
import { useLibrary } from "../hooks/useLibrary";
import { useLibraryPivot } from "../hooks/useLibraryPivot";
import { libraryPivots, type BrowseSearch } from "../lib/browseQuery";
import { errorCodeOf, errorKeyForCode } from "../lib/errorKeys";
import { libraryNamespace } from "../locales";
import { BrowsePivot, type LibraryItemsRenderer } from "./BrowsePivot";
import { primaryButtonClass } from "./buttonClasses";
import { OverviewPivot } from "./OverviewPivot";
import { ScanControl } from "./ScanControl";

/** Props of {@link LibraryPage}. */
export interface LibraryPageProps {
  /** Library from the route `/libraries/$libraryId`. */
  readonly libraryId: string;
  /** Validated search of the route: pivot, and sort, type and filter of the Browse pivot. */
  readonly search: BrowseSearch;
  readonly onSearchChange: BrowseSearchChange;
  /**
   * Renders the listed items of a type that has its own view, e.g. the album grid of a music
   * library. The app layer passes it, so this feature does not depend on other features.
   */
  readonly renderItems?: LibraryItemsRenderer | undefined;
}

const headingClass = "text-3xl font-semibold";

/** One library: name, scan button and the pivots Overview and Browse (LB-01). */
export function LibraryPage({ libraryId, search, onSearchChange, renderItems }: LibraryPageProps) {
  const { t } = useTranslation(libraryNamespace);
  const library = useLibrary(libraryId);
  if (library.status === "success") {
    // Keyed, so scan state and focus targets never carry over to another library.
    return (
      <LibraryContent
        key={libraryId}
        library={library.library}
        search={search}
        onSearchChange={onSearchChange}
        renderItems={renderItems}
      />
    );
  }
  if (library.status === "error") {
    return <LibraryLoadError error={library.error} onRetry={library.retry} />;
  }
  return (
    <>
      <h1 className={`mb-6 ${headingClass}`}>{t("heading")}</h1>
      <div role="status" className="w-60">
        <Skeleton label={t("loading")} className="h-5 w-full" />
      </div>
    </>
  );
}

interface LibraryContentProps extends Omit<LibraryPageProps, "libraryId"> {
  readonly library: Library;
}

function LibraryContent({ library, search, onSearchChange, renderItems }: LibraryContentProps) {
  const { t } = useTranslation(libraryNamespace);
  const { pivot, selectPivot, onBrowseSearchChange } = useLibraryPivot({
    libraryId: library.id,
    search,
    onSearchChange,
  });
  return (
    <>
      <div className="mb-6 flex flex-col gap-3">
        <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
          <h1 className={headingClass}>{library.name}</h1>
          {!library.available && (
            <Badge icon={<AlertIcon className="size-3 shrink-0 text-accent" />}>
              {t("libraryUnavailable")}
            </Badge>
          )}
        </div>
        <ScanControl library={library} />
      </div>
      <Tabs
        label={t("pivots.label")}
        tabs={libraryPivots.map((id) => ({ id, label: t(`pivots.${id}`) }))}
        value={pivot}
        onChange={selectPivot}
      >
        <div className="pt-6">
          {pivot === "overview" ? (
            <OverviewPivot
              libraryId={library.id}
              onOpenBrowse={() => {
                selectPivot("browse");
              }}
            />
          ) : (
            <BrowsePivot
              libraryId={library.id}
              libraryKind={library.kind}
              search={search}
              onSearchChange={onBrowseSearchChange}
              renderItems={renderItems}
            />
          )}
        </div>
      </Tabs>
    </>
  );
}

interface LibraryLoadErrorProps {
  readonly error: Error;
  readonly onRetry: () => void;
}

function LibraryLoadError({ error, onRetry }: LibraryLoadErrorProps) {
  const { t } = useTranslation(libraryNamespace);
  const errorKey = errorKeyForCode(errorCodeOf(error));
  const notFound = errorKey === "errors.notFound";
  return (
    <>
      <h1 className={`mb-6 ${headingClass}`}>{t("heading")}</h1>
      <p role="alert" className="text-text-muted">
        {notFound ? t(errorKey) : `${t("loadFailed")} ${t(errorKey)}`}
      </p>
      {notFound ? (
        <Link to="/" className={`mt-6 inline-block ${primaryButtonClass}`}>
          {t("backHome")}
        </Link>
      ) : (
        <button type="button" onClick={onRetry} className={`mt-6 ${primaryButtonClass}`}>
          {t("retry")}
        </button>
      )}
    </>
  );
}
