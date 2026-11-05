import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { cardWidths } from "../../../shared/lib/rowScroll";
import {
  EmptyState,
  LibraryIcon,
  Skeleton,
  primaryButtonClass,
  secondaryButtonClass,
} from "../../../shared/ui";
import type { HomeHint } from "../lib/homeRows";
import { homeNamespace } from "../locales";

/** Home without libraries: leads to Settings › Libraries (HO-09). */
export function NoLibraries() {
  const { t } = useTranslation();
  return (
    <EmptyState
      icon={<LibraryIcon className="size-10" />}
      title={t("home.noLibraries.title")}
      description={t("home.noLibraries.hint")}
      action={
        <Link
          to="/admin/manage/libraries"
          search={{ add: true }}
          className={`inline-block ${primaryButtonClass}`}
        >
          {t("home.noLibraries.add")}
        </Link>
      }
    />
  );
}

const skeletonRowCount = 2;
const skeletonCardCount = 6;

/** Placeholder rows while the Home rows load; screen readers hear one "Loading rows". */
export function RowsSkeleton() {
  const { t } = useTranslation(homeNamespace);
  return (
    <div role="status" className="relative flex flex-col gap-8">
      <span className="sr-only">{t("rows.loading")}</span>
      {Array.from({ length: skeletonRowCount }, (_, row) => (
        <div key={row} aria-hidden="true" className="flex flex-col gap-3">
          <Skeleton label="" className="h-5 w-48" />
          {/* Relative, so the clip also holds the absolutely positioned screen reader labels. */}
          <div className="relative flex gap-4 overflow-hidden">
            {Array.from({ length: skeletonCardCount }, (_, card) => (
              <div key={card} className="shrink-0" style={{ width: cardWidths.poster }}>
                <Skeleton label="" className="aspect-2/3 w-full rounded-md" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

/** Props of {@link RowsLoadError}. */
export interface RowsLoadErrorProps {
  readonly onRetry: () => void;
}

/** Shown when the Home rows could not be loaded, with a retry. */
export function RowsLoadError({ onRetry }: RowsLoadErrorProps) {
  const { t } = useTranslation(homeNamespace);
  return (
    <div role="alert" className="rounded-lg border border-divider bg-surface px-6 py-8">
      <h2 className="text-lg font-semibold text-text">{t("rows.loadError.title")}</h2>
      <p className="mt-2 text-sm text-text-muted">{t("rows.loadError.message")}</p>
      <button type="button" onClick={onRetry} className={`mt-4 ${secondaryButtonClass}`}>
        {t("rows.loadError.retry")}
      </button>
    </div>
  );
}

/** Props of {@link RowsHint}. */
export interface RowsHintProps {
  readonly hint: HomeHint;
}

/** A short note why Home has no or few rows. */
export function RowsHint({ hint }: RowsHintProps) {
  const { t } = useTranslation(homeNamespace);
  return (
    <p className="rounded-lg border border-dashed border-divider bg-surface p-5 text-sm text-text-muted">
      {t(`hint.${hint}`)}
    </p>
  );
}
