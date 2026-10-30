import type { Library } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { AlertIcon, Badge, Skeleton } from "../../../shared/ui";
import { useLibrary } from "../hooks/useLibrary";
import { errorCodeOf, errorKeyForCode } from "../lib/errorKeys";
import { libraryNamespace } from "../locales";
import { primaryButtonClass } from "./buttonClasses";
import { LibraryItemGrid } from "./LibraryItemGrid";
import { ScanControl } from "./ScanControl";

/** Props of {@link LibraryPage}. */
export interface LibraryPageProps {
  /** Library from the route `/libraries/$libraryId`. */
  readonly libraryId: string;
}

const headingClass = "text-3xl font-semibold";

/** One library: name, item count, scan button and the poster grid of its videos. */
export function LibraryPage({ libraryId }: LibraryPageProps) {
  const { t } = useTranslation(libraryNamespace);
  const library = useLibrary(libraryId);
  if (library.status === "success") {
    // Keyed, so scan state and focus targets never carry over to another library.
    return <LibraryContent key={libraryId} library={library.library} />;
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

interface LibraryContentProps {
  readonly library: Library;
}

function LibraryContent({ library }: LibraryContentProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <>
      <div className="mb-8 flex flex-col gap-3">
        <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
          <h1 className={headingClass}>{library.name}</h1>
          <p className="text-sm text-text-muted">{t("itemCount", { count: library.itemCount })}</p>
          {!library.available && (
            <Badge icon={<AlertIcon className="size-3 shrink-0 text-accent" />}>
              {t("libraryUnavailable")}
            </Badge>
          )}
        </div>
        <ScanControl library={library} />
      </div>
      <LibraryItemGrid libraryId={library.id} />
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
