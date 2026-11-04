import type { Library, LibraryKind } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import {
  ConfirmDialog,
  ModalDialog,
  primaryButtonClass,
  secondaryButtonClass,
} from "../../../shared/ui";
import { useSystemInfo } from "../../system";
import { useLibraryDialog, type LibraryDialogState } from "../hooks/useLibraryDialog";
import { useProblemText } from "../hooks/useProblemText";
import { chosenFolder, type SaveProblem } from "../lib/libraryDraft";
import { settingsNamespace } from "../locales";
import { AdvancedPane } from "./AdvancedPane";
import { DialogTabs, type DialogTab } from "./DialogTabs";
import { FolderBrowserDialog } from "./FolderBrowserDialog";
import { FoldersPane } from "./FoldersPane";
import { GeneralPane } from "./GeneralPane";

/** Props of {@link LibraryDialog}. */
export interface LibraryDialogProps {
  /** The library to edit; undefined adds a new one. */
  readonly library?: Library | undefined;
  readonly onClose: () => void;
  /** Called with the library name once saved; the caller announces the scan. */
  readonly onSaved: (name: string) => void;
}

interface BodyProps extends LibraryDialogProps {
  readonly kinds: readonly LibraryKind[];
}

type ProblemText = (problem: SaveProblem) => string;

function fieldErrors(problem: SaveProblem | null, text: ProblemText) {
  const message = problem === null ? undefined : text(problem);
  return {
    name: problem?.scope === "name" ? message : undefined,
    language: problem?.scope === "language" ? message : undefined,
    kind: problem?.scope === "kind" ? message : undefined,
  };
}

function DialogFooter({ state }: { readonly state: LibraryDialogState }) {
  const { t } = useTranslation(settingsNamespace);
  const { save } = state;
  return (
    <>
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          role="switch"
          checked={state.expert}
          onChange={(event) => {
            state.setExpert(event.target.checked);
          }}
        />
        {t("libraries.dialog.expert")}
      </label>
      <span className="flex gap-3">
        <button type="button" onClick={state.requestClose} className={secondaryButtonClass}>
          {t("libraries.dialog.cancel")}
        </button>
        <button
          type="button"
          aria-disabled={save.pending}
          onClick={
            save.pending
              ? undefined
              : () => {
                  save.save(state.form.draft);
                }
          }
          className={primaryButtonClass}
        >
          {save.pending ? t("libraries.dialog.saving") : t("libraries.dialog.save")}
        </button>
      </span>
    </>
  );
}

interface PanesProps {
  readonly state: LibraryDialogState;
  readonly library: Library | undefined;
  readonly kinds: readonly LibraryKind[];
  readonly problemText: ProblemText;
}

function DialogPanes({ state, library, kinds, problemText }: PanesProps) {
  const { form, save, tab } = state;
  if (tab === "folders") {
    return (
      <FoldersPane
        form={form}
        problem={save.problem}
        problemText={problemText}
        onAddFolder={() => {
          state.setBrowsing(true);
        }}
      />
    );
  }
  if (tab === "advanced") {
    return <AdvancedPane form={form} importedFromConfig={library?.importedFromConfig ?? false} />;
  }
  return (
    <GeneralPane
      form={form}
      kinds={kinds}
      editing={library !== undefined}
      errors={fieldErrors(save.problem, problemText)}
    />
  );
}

function NestedDialogs({
  state,
  onClose,
}: {
  readonly state: LibraryDialogState;
  onClose: () => void;
}) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <>
      {state.browsing && (
        <FolderBrowserDialog
          onClose={() => {
            state.setBrowsing(false);
          }}
          onSelect={(root, path) => {
            state.form.addFolder((key) => chosenFolder(root, path, key));
            state.setBrowsing(false);
          }}
        />
      )}
      {state.discarding && (
        <ConfirmDialog
          title={t("libraries.dialog.discardTitle")}
          description={t("libraries.dialog.discardDescription")}
          confirmLabel={t("libraries.dialog.discard")}
          cancelLabel={t("libraries.dialog.keepEditing")}
          onConfirm={onClose}
          onCancel={() => {
            state.setDiscarding(false);
          }}
        />
      )}
    </>
  );
}

function LibraryDialogBody({ library, kinds, onClose, onSaved }: BodyProps) {
  const { t } = useTranslation(settingsNamespace);
  const state = useLibraryDialog({ library, kinds, onClose, onSaved });
  const problemText = useProblemText();
  const { save, tab } = state;
  const tabs: DialogTab<LibraryDialogState["tab"]>[] = [
    { id: "general", label: t("libraries.dialog.tabs.general") },
    { id: "folders", label: t("libraries.dialog.tabs.folders") },
    ...(state.expert
      ? [{ id: "advanced" as const, label: t("libraries.dialog.tabs.advanced") }]
      : []),
  ];
  return (
    <>
      <ModalDialog
        title={t(
          library === undefined ? "libraries.dialog.addTitle" : "libraries.dialog.editTitle",
        )}
        closeLabel={t("libraries.dialog.close")}
        onClose={state.requestClose}
        footer={<DialogFooter state={state} />}
      >
        <DialogTabs
          label={t("libraries.dialog.tabsLabel")}
          tabs={tabs}
          selected={tab}
          onSelect={state.setTab}
        >
          <DialogPanes state={state} library={library} kinds={kinds} problemText={problemText} />
        </DialogTabs>
        <p role="alert" className="mt-4 text-sm text-accent empty:hidden">
          {save.problem?.scope === "general" ? problemText(save.problem) : undefined}
        </p>
      </ModalDialog>
      <NestedDialogs state={state} onClose={onClose} />
    </>
  );
}

/**
 * Add or edit a library: kind, name and language, the folder list and the expert options in one
 * dialog. Closing with unsaved changes asks first. Render it only while it is open.
 */
export function LibraryDialog(props: LibraryDialogProps) {
  const { t } = useTranslation(settingsNamespace);
  const info = useSystemInfo();
  if (props.library !== undefined) {
    return <LibraryDialogBody {...props} kinds={[props.library.kind]} />;
  }
  if (info.data === undefined) {
    return (
      <ModalDialog
        title={t("libraries.dialog.addTitle")}
        closeLabel={t("libraries.dialog.close")}
        onClose={props.onClose}
      >
        <p role={info.isError ? "alert" : "status"}>
          {info.isError ? t("libraries.dialog.infoFailed") : t("libraries.dialog.loading")}
        </p>
      </ModalDialog>
    );
  }
  return <LibraryDialogBody {...props} kinds={info.data.libraryKinds} />;
}
