import { useTranslation } from "react-i18next";

import { secondaryButtonClass } from "../../../shared/ui";
import { useFolderRoots } from "../hooks/useFolderBrowser";
import type { LibraryForm } from "../hooks/useLibraryForm";
import { maxFolders, type SaveProblem } from "../lib/libraryDraft";
import { settingsNamespace } from "../locales";
import { PermissionHelp } from "./PermissionHelp";

/** Props of {@link FoldersPane}. */
export interface FoldersPaneProps {
  readonly form: LibraryForm;
  /** The failed save or local check when it concerns the folders; null otherwise. */
  readonly problem: SaveProblem | null;
  readonly problemText: (problem: SaveProblem) => string;
  readonly onAddFolder: () => void;
}

function FolderProblem(props: {
  readonly problem: SaveProblem;
  readonly problemText: (problem: SaveProblem) => string;
}) {
  const roots = useFolderRoots();
  return (
    <>
      <p role="alert" className="text-sm text-accent">
        {props.problemText(props.problem)}
      </p>
      {props.problem.code === "folder_unreadable" && roots.data !== undefined && (
        <PermissionHelp installKind={roots.data.installKind} serviceUser={roots.data.serviceUser} />
      )}
    </>
  );
}

/** Tab "Folders": the chosen folders with remove buttons and the Add folder button. */
export function FoldersPane({ form, problem, problemText, onAddFolder }: FoldersPaneProps) {
  const { t } = useTranslation(settingsNamespace);
  const { folders } = form.draft;
  const full = folders.length >= maxFolders;
  const listProblem = problem?.scope === "folders" ? problem : null;
  return (
    <>
      <p className="mb-2 text-sm font-medium">
        {t("libraries.dialog.foldersHeading", { max: maxFolders })}
      </p>
      {folders.length === 0 ? (
        <p className="text-sm text-text-muted">{t("libraries.dialog.noFolders")}</p>
      ) : (
        <ul aria-label={t("libraries.foldersLabel")} className="flex flex-col gap-2">
          {folders.map((folder, index) => (
            <li key={folder.key} className="rounded-md border border-divider bg-surface p-2">
              <div className="flex items-center justify-between gap-2">
                <code className="break-all text-sm">{folder.path}</code>
                <button
                  type="button"
                  onClick={() => {
                    form.removeFolder(folder.key);
                  }}
                  aria-label={t("libraries.dialog.removeFolder", { path: folder.path })}
                  className={`${secondaryButtonClass} px-2 py-1`}
                >
                  <span aria-hidden="true">✕</span>
                </button>
              </div>
              {problem?.scope === "folder" && problem.folderIndex === index && (
                <FolderProblem problem={problem} problemText={problemText} />
              )}
            </li>
          ))}
        </ul>
      )}
      {listProblem !== null && <FolderProblem problem={listProblem} problemText={problemText} />}
      <button
        type="button"
        aria-disabled={full}
        onClick={full ? undefined : onAddFolder}
        className={`${secondaryButtonClass} mt-3`}
      >
        {t("libraries.dialog.addFolder")}
      </button>
      <p className="mt-3 text-xs text-text-muted">{t("libraries.dialog.folderRules")}</p>
    </>
  );
}
