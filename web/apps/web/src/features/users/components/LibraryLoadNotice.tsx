import { useTranslation } from "react-i18next";

import { secondaryButtonClass } from "../../../shared/ui";
import type { LibraryLoadStatus } from "../hooks/useLibraryList";
import { usersNamespace } from "../locales";

/** Props of {@link LibraryLoadNotice}. */
export interface LibraryLoadNoticeProps {
  readonly status: Exclude<LibraryLoadStatus, "ready">;
  readonly onRetry: () => void;
}

/**
 * Stands in for the library checkboxes until the libraries are there: a loading line, or the
 * failure with "Try again". The dialog cannot be saved meanwhile.
 */
export function LibraryLoadNotice({ status, onRetry }: LibraryLoadNoticeProps) {
  const { t } = useTranslation(usersNamespace);
  if (status === "loading") {
    return (
      <p role="status" className="text-sm text-text-muted">
        {t("users.dialog.access.loading")}
      </p>
    );
  }
  return (
    <div role="alert" className="flex flex-col items-start gap-2 text-sm text-accent">
      <p>{t("users.dialog.access.loadFailed")}</p>
      <button type="button" onClick={onRetry} className={secondaryButtonClass}>
        {t("users.dialog.access.retry")}
      </button>
    </div>
  );
}
