import { useTranslation } from "react-i18next";

import { RowsSkeleton, secondaryButtonClass } from "../../../shared/ui";
import { useLibraryRows } from "../hooks/useLibraryRows";
import { libraryNamespace } from "../locales";
import { ServerRow } from "./ServerRow";

/** Props of {@link OverviewPivot}. */
export interface OverviewPivotProps {
  readonly libraryId: string;
  /** Switches to the Browse pivot, offered when rows are missing. */
  readonly onOpenBrowse: () => void;
}

const noteClass =
  "flex flex-wrap items-center gap-x-4 gap-y-3 rounded-lg border border-dashed border-divider bg-surface p-5 text-sm text-text-muted";

/**
 * The Overview pivot (LB-02...04, LB-09): the rows the server generated for the library. With
 * too little data it shows the server's hint and a way to Browse instead of an empty page.
 */
export function OverviewPivot({ libraryId, onOpenBrowse }: OverviewPivotProps) {
  const { t } = useTranslation(libraryNamespace);
  const rows = useLibraryRows(libraryId);
  switch (rows.state) {
    case "loading":
      return <RowsSkeleton label={t("overview.loading")} />;
    case "error":
      return rows.notSupported ? (
        <OverviewNote text={t("overview.notSupported")} onOpenBrowse={onOpenBrowse} />
      ) : (
        <div role="alert" className="rounded-lg border border-divider bg-surface px-6 py-8">
          <p className="text-text">{t("overview.loadFailed")}</p>
          <button type="button" onClick={rows.retry} className={`mt-4 ${secondaryButtonClass}`}>
            {t("retry")}
          </button>
        </div>
      );
    default:
      return (
        <div className="flex flex-col gap-8">
          {rows.rows.map((row) => (
            <ServerRow key={row.id} row={row} />
          ))}
          {rows.hint !== undefined && (
            <OverviewNote text={t(`overview.hint.${rows.hint}`)} onOpenBrowse={onOpenBrowse} />
          )}
        </div>
      );
  }
}

interface OverviewNoteProps {
  readonly text: string;
  readonly onOpenBrowse: () => void;
}

function OverviewNote({ text, onOpenBrowse }: OverviewNoteProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <div className={noteClass}>
      <p className="grow">{text}</p>
      <button type="button" onClick={onOpenBrowse} className={secondaryButtonClass}>
        {t("overview.openBrowse")}
      </button>
    </div>
  );
}
