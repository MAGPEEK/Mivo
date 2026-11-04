import { useTranslation } from "react-i18next";

import { useLibraries } from "../../library";
import type { SaveProblem } from "../lib/libraryDraft";
import { settingsNamespace } from "../locales";

// Literal keys, so a misspelled one fails type checking; codes not listed use the generic text.
const messageKeys = {
  name_required: "libraries.dialog.problems.name_required",
  folders_required: "libraries.dialog.problems.folders_required",
  name_invalid: "libraries.dialog.problems.name_invalid",
  conflict: "libraries.dialog.problems.conflict",
  language_invalid: "libraries.dialog.problems.language_invalid",
  kind_unsupported: "libraries.dialog.problems.kind_unsupported",
  too_many_folders: "libraries.dialog.problems.too_many_folders",
  folder_not_found: "libraries.dialog.problems.folder_not_found",
  folder_unreadable: "libraries.dialog.problems.folder_unreadable",
  folder_overlap: "libraries.dialog.problems.folder_overlap",
  folder_reserved: "libraries.dialog.problems.folder_reserved",
  folder_outside_roots: "libraries.dialog.problems.folder_outside_roots",
  forbidden: "libraries.errors.forbidden",
} as const;

type KnownCode = keyof typeof messageKeys;

function isKnownCode(code: string): code is KnownCode {
  return Object.keys(messageKeys).includes(code);
}

/**
 * Returns a function that turns a {@link SaveProblem} into the text shown next to its field.
 * `folder_overlap` names the other library when the server sent its id and the list knows it;
 * unknown codes fall back to the server's English reason.
 */
export function useProblemText(): (problem: SaveProblem) => string {
  const { t } = useTranslation(settingsNamespace);
  const libraries = useLibraries();
  return (problem) => {
    if (problem.code === "folder_overlap") {
      const other = libraries.data?.find((library) => library.id === problem.libraryId);
      if (other !== undefined) {
        return t("libraries.dialog.problems.folder_overlap_named", { name: other.name });
      }
    }
    return isKnownCode(problem.code)
      ? t(messageKeys[problem.code])
      : t("libraries.dialog.saveFailed", { reason: problem.reason });
  };
}
