import type { Library, LibraryKind } from "@mivo/api-client";
import { useState } from "react";

import { draftOf, emptyDraft, type SaveProblem } from "../lib/libraryDraft";
import { useLibraryForm, type LibraryForm } from "./useLibraryForm";
import { useLibrarySave, type LibrarySave } from "./useLibrarySave";

/** The tabs of the library dialog. */
export type LibraryDialogTab = "general" | "folders" | "advanced";

const tabByScope: Readonly<Record<SaveProblem["scope"], LibraryDialogTab | undefined>> = {
  name: "general",
  language: "general",
  kind: "general",
  folders: "folders",
  folder: "folders",
  general: undefined,
};

/** Everything the library dialog needs besides its markup. */
export interface LibraryDialogState {
  readonly form: LibraryForm;
  readonly save: LibrarySave;
  readonly tab: LibraryDialogTab;
  readonly setTab: (tab: LibraryDialogTab) => void;
  readonly expert: boolean;
  readonly setExpert: (expert: boolean) => void;
  readonly browsing: boolean;
  readonly setBrowsing: (browsing: boolean) => void;
  readonly discarding: boolean;
  readonly setDiscarding: (discarding: boolean) => void;
  /** Closes now, or asks about unsaved changes first; ignored while a save runs. */
  readonly requestClose: () => void;
}

/** Options of {@link useLibraryDialog}. */
export interface LibraryDialogOptions {
  readonly library: Library | undefined;
  readonly kinds: readonly LibraryKind[];
  readonly onClose: () => void;
  readonly onSaved: (name: string) => void;
}

/** State of the add/edit dialog: draft, save, active tab, nested dialogs and closing. */
export function useLibraryDialog(options: LibraryDialogOptions): LibraryDialogState {
  const { library, kinds, onClose, onSaved } = options;
  const [tab, setTab] = useState<LibraryDialogTab>("general");
  const [expertRaw, setExpertRaw] = useState(false);
  const [browsing, setBrowsing] = useState(false);
  const [discarding, setDiscarding] = useState(false);
  const save = useLibrarySave(library, {
    onSaved: (saved) => {
      onSaved(saved.name);
    },
    // Show the tab that holds the offending field; unrelated problems stay where the user is.
    onProblem: (problem) => {
      setTab(tabByScope[problem.scope] ?? tab);
    },
  });
  const form = useLibraryForm(
    library === undefined ? emptyDraft(kinds[0] ?? "movies") : draftOf(library),
    save.clearProblem,
  );
  const requestClose = () => {
    // A request in flight cannot be taken back; closing then would hide its outcome.
    if (save.pending) {
      return;
    }
    if (form.dirty) {
      setDiscarding(true);
    } else {
      onClose();
    }
  };
  return {
    form,
    save,
    tab,
    setTab,
    expert: expertRaw,
    setExpert: (expert) => {
      setExpertRaw(expert);
      // The Advanced tab disappears with the switch; leave it first.
      if (!expert && tab === "advanced") {
        setTab("general");
      }
    },
    browsing,
    setBrowsing,
    discarding,
    setDiscarding,
    requestClose,
  };
}
