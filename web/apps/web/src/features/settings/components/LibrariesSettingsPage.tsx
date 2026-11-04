import { productName } from "@mivo/ui-tokens";
import { useRef } from "react";
import { useTranslation } from "react-i18next";

import {
  ConfirmDialog,
  EmptyState,
  LibraryIcon,
  Skeleton,
  primaryButtonClass,
  secondaryButtonClass,
} from "../../../shared/ui";
import { useLibraries } from "../../library";
import { useLibraryRemoval, type LibraryRemoval } from "../hooks/useLibraryRemoval";
import { settingsNamespace } from "../locales";
import { LibraryRow } from "./LibraryRow";

function LibraryList({ onRemove }: { readonly onRemove: LibraryRemoval["ask"] }) {
  const { t } = useTranslation(settingsNamespace);
  const libraries = useLibraries();
  if (libraries.isPending) {
    return (
      <div role="status" className="w-60">
        <Skeleton label={t("libraries.loading")} className="h-5 w-full" />
      </div>
    );
  }
  if (libraries.isError) {
    return (
      <div role="alert" className="flex flex-col items-start gap-3">
        <p>{t("libraries.loadFailed")}</p>
        <button
          type="button"
          onClick={() => void libraries.refetch()}
          className={secondaryButtonClass}
        >
          {t("libraries.retry")}
        </button>
      </div>
    );
  }
  if (libraries.data.length === 0) {
    return (
      <EmptyState
        icon={<LibraryIcon className="size-10" />}
        title={t("libraries.empty.title")}
        description={t("libraries.empty.hint")}
      />
    );
  }
  return (
    <ul aria-label={t("libraries.listLabel")} className="flex flex-col gap-3">
      {libraries.data.map((library) => (
        <LibraryRow key={library.id} library={library} onRemove={onRemove} />
      ))}
    </ul>
  );
}

function RemoveDialog({ removal }: { readonly removal: LibraryRemoval }) {
  const { t } = useTranslation(settingsNamespace);
  if (removal.target === null) {
    return null;
  }
  return (
    <ConfirmDialog
      title={t("libraries.remove.title")}
      description={t("libraries.remove.description", { name: removal.target.name, productName })}
      confirmLabel={t("libraries.remove.confirm")}
      cancelLabel={t("libraries.remove.cancel")}
      pending={removal.pending}
      error={removal.errorText ?? undefined}
      onConfirm={removal.confirm}
      onCancel={removal.cancel}
    />
  );
}

/** Settings › Libraries: every library with its state and the actions to scan, edit and remove. */
export function LibrariesSettingsPage() {
  const { t } = useTranslation(settingsNamespace);
  const headingRef = useRef<HTMLHeadingElement>(null);
  // The removed row takes the focused button with it; the heading is the stable landmark left.
  const removal = useLibraryRemoval(() => headingRef.current?.focus());
  return (
    <>
      <nav aria-label={t("libraries.breadcrumb.label")} className="mb-2 text-sm text-text-muted">
        <ol className="flex gap-2">
          <li>{t("libraries.breadcrumb.settings")}</li>
          <li aria-hidden="true">›</li>
          <li aria-current="page">{t("libraries.heading")}</li>
        </ol>
      </nav>
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h1 ref={headingRef} tabIndex={-1} className="text-3xl font-semibold">
          {t("libraries.heading")}
        </h1>
        {/* The add dialog arrives with M3-19n; until then the button stays inert. */}
        <button type="button" disabled className={primaryButtonClass}>
          {t("libraries.add")}
        </button>
      </div>
      <LibraryList onRemove={removal.ask} />
      <RemoveDialog removal={removal} />
    </>
  );
}
