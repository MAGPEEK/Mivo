import type { Library } from "@mivo/api-client";
import { productName } from "@mivo/ui-tokens";
import { useRef, useState } from "react";
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
import type { SettingsSearch } from "../lib/settingsSearch";
import { settingsNamespace } from "../locales";
import { LibraryDialog } from "./LibraryDialog";
import { LibraryRow } from "./LibraryRow";
import { SettingsNav } from "./SettingsNav";
import { SettingsBreadcrumb } from "./SettingsPageHeading";

interface LibraryListProps {
  readonly onEdit: (library: Library) => void;
  readonly onRemove: LibraryRemoval["ask"];
}

function LibraryList({ onEdit, onRemove }: LibraryListProps) {
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
        <LibraryRow key={library.id} library={library} onEdit={onEdit} onRemove={onRemove} />
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

/** Props of {@link LibrariesSettingsPage}. */
export interface LibrariesSettingsPageProps {
  /** The validated search of the route; it says which dialog is open. */
  readonly search: SettingsSearch;
  /** Replaces the search of the route; `{}` closes the dialogs. */
  readonly onSearchChange: (search: SettingsSearch) => void;
}

interface FormDialogProps extends LibrariesSettingsPageProps {
  readonly onSaved: (name: string) => void;
}

// The edit dialog waits for the list: it needs the library to edit, and an unknown id in the
// URL (removed meanwhile) simply shows no dialog.
function FormDialog({ search, onSearchChange, onSaved }: FormDialogProps) {
  const libraries = useLibraries();
  const close = () => {
    onSearchChange({});
  };
  if (search.add === true) {
    return <LibraryDialog onClose={close} onSaved={onSaved} />;
  }
  const editing = libraries.data?.find((library) => library.id === search.edit);
  return editing === undefined ? null : (
    <LibraryDialog key={editing.id} library={editing} onClose={close} onSaved={onSaved} />
  );
}

/** Settings › Libraries: every library with its state and the actions to scan, edit and remove. */
export function LibrariesSettingsPage({ search, onSearchChange }: LibrariesSettingsPageProps) {
  const { t } = useTranslation(settingsNamespace);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [announcement, setAnnouncement] = useState("");
  // The removed row takes the focused button with it; the heading is the stable landmark left.
  const removal = useLibraryRemoval(() => headingRef.current?.focus());
  return (
    <>
      <SettingsBreadcrumb title={t("libraries.heading")} />
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h1 ref={headingRef} tabIndex={-1} className="text-3xl font-semibold">
          {t("libraries.heading")}
        </h1>
        <button
          type="button"
          onClick={() => {
            onSearchChange({ add: true });
          }}
          className={primaryButtonClass}
        >
          {t("libraries.add")}
        </button>
      </div>
      <SettingsNav />
      {/* Live region for the scan the server starts after saving; it outlives the dialog. */}
      <p role="status" className="sr-only">
        {announcement}
      </p>
      <LibraryList
        onEdit={(library) => {
          onSearchChange({ edit: library.id });
        }}
        onRemove={removal.ask}
      />
      <FormDialog
        search={search}
        onSearchChange={onSearchChange}
        onSaved={(name) => {
          setAnnouncement(t("libraries.dialog.scanning", { name }));
          onSearchChange({});
        }}
      />
      <RemoveDialog removal={removal} />
    </>
  );
}
