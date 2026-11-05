import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import {
  EmptyState,
  LibraryIcon,
  RowsSkeleton as SharedRowsSkeleton,
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

/** Placeholder rows while the Home rows load; screen readers hear one "Loading rows". */
export function RowsSkeleton() {
  const { t } = useTranslation(homeNamespace);
  return <SharedRowsSkeleton label={t("rows.loading")} />;
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
