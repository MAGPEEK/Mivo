import type { Library } from "@mivo/api-client";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import type { LibraryLoadStatus } from "../hooks/useLibraryList";
import type { LibraryAccessDraft } from "../lib/libraryAccess";
import { usersNamespace } from "../locales";
import { LibraryLoadNotice } from "./LibraryLoadNotice";

/** Props of {@link LibraryAccessField}. */
export interface LibraryAccessFieldProps {
  readonly access: LibraryAccessDraft;
  readonly libraries: readonly Library[];
  /** Load state of `libraries`; until `ready` a notice replaces the checkboxes. */
  readonly status: LibraryLoadStatus;
  readonly onRetry: () => void;
  /** True for an admin account: it always has every library, so the choice is fixed. */
  readonly locked: boolean;
  readonly error?: string | undefined;
  readonly onAllChange: (all: boolean) => void;
  readonly onLibraryChange: (libraryId: string, selected: boolean) => void;
}

const checkClass = "flex items-center gap-2 text-sm text-text";

function LibraryChecks(props: LibraryAccessFieldProps & { readonly disabled: boolean }) {
  const { t } = useTranslation(usersNamespace);
  if (props.status !== "ready") {
    return <LibraryLoadNotice status={props.status} onRetry={props.onRetry} />;
  }
  if (props.libraries.length === 0) {
    return <p className="text-sm text-text-muted">{t("users.dialog.access.noLibraries")}</p>;
  }
  return (
    <div role="group" aria-label={t("users.dialog.access.selectionLabel")}>
      <ul className="flex flex-col gap-1">
        {props.libraries.map((library) => (
          <li key={library.id}>
            <label className={checkClass}>
              <input
                type="checkbox"
                disabled={props.disabled}
                checked={props.access.libraryIds.includes(library.id)}
                onChange={(event) => {
                  props.onLibraryChange(library.id, event.target.checked);
                }}
              />
              {library.name}
            </label>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** "All libraries" or "Selected libraries" with one checkbox per library (US-22). */
export function LibraryAccessField(props: LibraryAccessFieldProps) {
  const { t } = useTranslation(usersNamespace);
  const name = useId();
  const errorId = useId();
  const all = props.locked || props.access.all;
  return (
    <fieldset className="mb-4 flex flex-col gap-2" aria-describedby={errorId}>
      <legend className="mb-1 text-sm font-medium text-text">
        {t("users.dialog.access.legend")}
      </legend>
      <label className={checkClass}>
        <input
          type="radio"
          name={name}
          disabled={props.locked}
          checked={all}
          onChange={() => {
            props.onAllChange(true);
          }}
        />
        {t("users.dialog.access.all")}
      </label>
      <label className={checkClass}>
        <input
          type="radio"
          name={name}
          disabled={props.locked}
          checked={!all}
          onChange={() => {
            props.onAllChange(false);
          }}
        />
        {t("users.dialog.access.selected")}
      </label>
      <div className="ml-6">
        <LibraryChecks {...props} disabled={all} />
      </div>
      {props.locked && (
        <p className="text-xs text-text-muted">{t("users.dialog.access.adminHint")}</p>
      )}
      <p id={errorId} role="alert" className="text-sm text-accent empty:hidden">
        {props.error}
      </p>
    </fieldset>
  );
}
