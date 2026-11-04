import type { Library, LibraryKind } from "@mivo/api-client";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { secondaryButtonClass } from "../../../shared/ui";
import {
  LibraryKindIcon,
  useLibraryScan,
  useScanStatusText,
  type LibraryScan,
} from "../../library";
import { settingsNamespace } from "../locales";

/** Props of {@link LibraryRow}. */
export interface LibraryRowProps {
  readonly library: Library;
  readonly onEdit: (library: Library) => void;
  readonly onRemove: (library: Library) => void;
}

// Later API versions may add kinds; they show their raw name instead of failing.
function kindLabel(kind: LibraryKind, t: TFunction<typeof settingsNamespace>): string {
  switch (kind) {
    case "movies":
      return t("libraries.kind.movies");
    case "shows":
      return t("libraries.kind.shows");
    default:
      return kind;
  }
}

function formatTime(timestamp: string, language: string): string {
  return new Intl.DateTimeFormat(language, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(timestamp),
  );
}

function FolderList({ library }: { readonly library: Library }) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <ul aria-label={t("libraries.foldersLabel")} className="mt-2 flex flex-col gap-1 text-sm">
      {library.folders.map((folder) => (
        <li key={folder.id} className="flex flex-wrap items-center gap-2">
          <code className="break-all text-text-muted">{folder.path}</code>
          {!folder.available && (
            <span className="rounded-md bg-surface-raised px-2 py-0.5 text-xs text-accent">
              {t("libraries.folderUnavailable")}
            </span>
          )}
        </li>
      ))}
    </ul>
  );
}

interface ScanSummaryProps {
  readonly library: Library;
  readonly scan: LibraryScan;
}

function ScanSummary({ library, scan }: ScanSummaryProps) {
  const { t, i18n } = useTranslation(settingsNamespace);
  const statusText = useScanStatusText(library.scan, scan.running, scan.startError);
  const scanTime = library.scan.finishedAt ?? library.scan.startedAt;
  return (
    <div className="mt-1 text-sm text-text-muted">
      <p role="status">{statusText === "" ? t("libraries.neverScanned") : statusText}</p>
      {scanTime !== undefined && (
        <p>
          {t("libraries.lastScan")}:{" "}
          <time dateTime={scanTime}>{formatTime(scanTime, i18n.language)}</time>
        </p>
      )}
    </div>
  );
}

interface ActionsProps extends LibraryRowProps {
  readonly scan: LibraryScan;
}

function RowActions({ library, onEdit, onRemove, scan }: ActionsProps) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <div className="flex shrink-0 flex-wrap gap-2">
      {/* aria-disabled instead of disabled: a disabled button drops keyboard focus. */}
      <button
        type="button"
        onClick={scan.start}
        aria-disabled={scan.running}
        aria-busy={scan.running}
        aria-label={
          scan.running
            ? t("libraries.actions.scanningLabel", { name: library.name })
            : t("libraries.actions.scanLabel", { name: library.name })
        }
        className={secondaryButtonClass}
      >
        {scan.running ? t("libraries.actions.scanning") : t("libraries.actions.scan")}
      </button>
      <button
        type="button"
        onClick={() => {
          onEdit(library);
        }}
        aria-label={t("libraries.actions.editLabel", { name: library.name })}
        className={secondaryButtonClass}
      >
        {t("libraries.actions.edit")}
      </button>
      <button
        type="button"
        onClick={() => {
          onRemove(library);
        }}
        aria-label={t("libraries.actions.removeLabel", { name: library.name })}
        className={secondaryButtonClass}
      >
        {t("libraries.actions.remove")}
      </button>
    </div>
  );
}

/** One library of the settings list: kind, name, folders, state and the Scan/Edit/Remove actions. */
export function LibraryRow({ library, onEdit, onRemove }: LibraryRowProps) {
  const { t } = useTranslation(settingsNamespace);
  const scan = useLibraryScan(library);
  return (
    <li className="flex flex-col gap-3 rounded-lg border border-divider bg-surface p-4 md:flex-row md:items-start md:justify-between">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <LibraryKindIcon kind={library.kind} className="size-5 shrink-0 text-accent" />
          <h2 className="truncate text-lg font-semibold">{library.name}</h2>
          <span className="text-sm text-text-muted">{kindLabel(library.kind, t)}</span>
        </div>
        <FolderList library={library} />
        <p className="mt-2 text-sm text-text-muted">
          {t("libraries.itemCount", { count: library.itemCount })}
        </p>
        <ScanSummary library={library} scan={scan} />
        {library.importedFromConfig && (
          <p className="mt-1 text-xs text-text-muted">{t("libraries.imported")}</p>
        )}
      </div>
      <RowActions library={library} onEdit={onEdit} onRemove={onRemove} scan={scan} />
    </li>
  );
}
