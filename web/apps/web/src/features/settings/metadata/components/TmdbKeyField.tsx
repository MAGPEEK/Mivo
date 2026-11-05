import { useId } from "react";
import { useTranslation } from "react-i18next";

import { secondaryButtonClass } from "../../../../shared/ui";
import { settingsNamespace } from "../../locales";
import { useTmdbKeyField, type TmdbKeyField as KeyFieldState } from "../hooks/useTmdbKeyField";
import { metadataProblemKeys } from "../lib/problemKeys";
import type { TmdbKeyStatus } from "../lib/providerGroups";

/** Props of {@link TmdbKeyField}. */
export interface TmdbKeyFieldProps {
  readonly keyStatus: TmdbKeyStatus;
}

const statusKeys = {
  bundled: "metadata.key.statusBundled",
  own: "metadata.key.statusOwn",
  none: "metadata.key.statusNone",
} as const satisfies Record<TmdbKeyStatus, string>;

/** Length limit of the key in the API. */
const maxKeyLength = 128;

const inputClass =
  "w-full max-w-sm rounded-md border border-divider bg-surface px-3 py-2 text-sm text-text";

interface KeyActionsProps extends TmdbKeyFieldProps {
  readonly field: KeyFieldState;
}

function KeyActions({ field, keyStatus }: KeyActionsProps) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <>
      <button type="submit" disabled={!field.canSave} className={secondaryButtonClass}>
        {t("metadata.key.save")}
      </button>
      {keyStatus === "own" && (
        <button
          type="button"
          disabled={field.pending}
          onClick={field.remove}
          className={secondaryButtonClass}
        >
          {t("metadata.key.remove")}
        </button>
      )}
    </>
  );
}

/**
 * Write-only input for an own TMDB key. Only the key status is shown, never the key: the
 * server does not return it and the field empties after saving.
 */
export function TmdbKeyField({ keyStatus }: TmdbKeyFieldProps) {
  const { t } = useTranslation(settingsNamespace);
  const field = useTmdbKeyField();
  const inputId = useId();
  const hintId = useId();
  return (
    <form
      className="flex flex-col gap-2 pl-6"
      onSubmit={(event) => {
        event.preventDefault();
        field.save();
      }}
    >
      <label htmlFor={inputId} className="text-sm font-medium">
        {t("metadata.key.label")}
      </label>
      <p id={hintId} className="text-xs text-text-muted">
        {t("metadata.key.hint")} {t("metadata.key.status", { status: t(statusKeys[keyStatus]) })}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <input
          id={inputId}
          type="password"
          autoComplete="off"
          maxLength={maxKeyLength}
          spellCheck={false}
          value={field.draft}
          aria-describedby={hintId}
          onChange={(event) => {
            field.setDraft(event.target.value);
          }}
          className={inputClass}
        />
        <KeyActions field={field} keyStatus={keyStatus} />
      </div>
      {field.problem !== null && (
        <p role="alert" className="text-sm text-accent">
          {t(metadataProblemKeys[field.problem])}
        </p>
      )}
    </form>
  );
}
