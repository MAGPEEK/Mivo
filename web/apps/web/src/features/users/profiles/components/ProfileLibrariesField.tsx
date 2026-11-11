import { useId } from "react";
import { useTranslation } from "react-i18next";

import { LibraryLoadNotice } from "../../components/LibraryLoadNotice";
import type { LibraryLoadStatus } from "../../hooks/useLibraryList";
import type { LibraryAccessDraft } from "../../lib/libraryAccess";
import { usersNamespace } from "../../locales";
import { isChosen, type LibraryChoice } from "../lib/profileAccess";

/** Props of {@link ProfileLibrariesField}. */
export interface ProfileLibrariesFieldProps {
  readonly access: LibraryAccessDraft;
  /** Every library, marked by whether the account may see it. */
  readonly choices: readonly LibraryChoice[];
  /** Load state of `choices`; until `ready` a notice replaces the checkboxes. */
  readonly status: LibraryLoadStatus;
  readonly onRetry: () => void;
  readonly error?: string | undefined;
  readonly onAllChange: (all: boolean) => void;
  readonly onLibraryChange: (libraryId: string, selected: boolean) => void;
}

const checkClass = "flex items-center gap-2 text-sm text-text";

function LibraryChecks(props: ProfileLibrariesFieldProps) {
  const { t } = useTranslation(usersNamespace);
  if (props.status !== "ready") {
    return <LibraryLoadNotice status={props.status} onRetry={props.onRetry} />;
  }
  if (props.choices.length === 0) {
    return <p className="text-sm text-text-muted">{t("users.dialog.access.noLibraries")}</p>;
  }
  return (
    <div role="group" aria-label={t("profiles.dialog.access.selectionLabel")}>
      <ul className="flex flex-col gap-1">
        {props.choices.map((choice) => (
          <li key={choice.library.id}>
            <label className={`${checkClass} ${choice.allowed ? "" : "text-text-muted"}`}>
              <input
                type="checkbox"
                disabled={props.access.all || !choice.allowed}
                checked={isChosen(props.access, choice)}
                onChange={(event) => {
                  props.onLibraryChange(choice.library.id, event.target.checked);
                }}
              />
              {choice.library.name}
              {!choice.allowed && <span> ({t("profiles.dialog.access.notAllowed")})</span>}
            </label>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * The libraries of a managed profile: all of its account's, or a selection of them. Libraries
 * the account itself cannot see are listed disabled, so it is clear why they are missing.
 */
export function ProfileLibrariesField(props: ProfileLibrariesFieldProps) {
  const { t } = useTranslation(usersNamespace);
  const name = useId();
  const errorId = useId();
  const { all } = props.access;
  const someNotAllowed = props.choices.some((choice) => !choice.allowed);
  return (
    <fieldset className="mb-4 flex flex-col gap-2" aria-describedby={errorId}>
      <legend className="mb-1 text-sm font-medium text-text">
        {t("users.dialog.access.legend")}
      </legend>
      <label className={checkClass}>
        <input
          type="radio"
          name={name}
          checked={all}
          onChange={() => {
            props.onAllChange(true);
          }}
        />
        {t("profiles.dialog.access.all")}
      </label>
      <label className={checkClass}>
        <input
          type="radio"
          name={name}
          checked={!all}
          onChange={() => {
            props.onAllChange(false);
          }}
        />
        {t("users.dialog.access.selected")}
      </label>
      <div className="ml-6">
        <LibraryChecks {...props} />
      </div>
      {someNotAllowed && (
        <p className="text-xs text-text-muted">{t("profiles.dialog.access.notAllowedHint")}</p>
      )}
      <p id={errorId} role="alert" className="text-sm text-accent empty:hidden">
        {props.error}
      </p>
    </fieldset>
  );
}
