import type { LibraryKind } from "@mivo/api-client";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { languageName } from "../../item";
import type { LibraryForm } from "../hooks/useLibraryForm";
import { metadataLanguageCodes } from "../lib/libraryDraft";
import { settingsNamespace } from "../locales";
import { KindTiles } from "./KindTiles";

/** Props of {@link GeneralPane}. */
export interface GeneralPaneProps {
  readonly form: LibraryForm;
  readonly kinds: readonly LibraryKind[];
  readonly editing: boolean;
  /** Field-level messages from a failed save or local check. */
  readonly errors: {
    readonly name?: string | undefined;
    readonly language?: string | undefined;
    readonly kind?: string | undefined;
  };
}

/** Length limit of a library name in the API. */
const maxNameLength = 128;

const inputClass =
  "w-full rounded-md border border-divider bg-surface px-3 py-2 text-sm text-text aria-[invalid=true]:border-accent";

interface FieldProps {
  readonly form: LibraryForm;
  readonly error: string | undefined;
}

function NameField({ form, error }: FieldProps) {
  const { t } = useTranslation(settingsNamespace);
  const nameId = useId();
  const errorId = useId();
  return (
    <div className="mb-4 flex flex-col gap-1">
      <label htmlFor={nameId} className="text-sm font-medium">
        {t("libraries.dialog.name")}
      </label>
      <input
        id={nameId}
        type="text"
        data-autofocus
        maxLength={maxNameLength}
        value={form.draft.name}
        aria-invalid={error !== undefined}
        aria-describedby={errorId}
        onChange={(event) => {
          form.setName(event.target.value);
        }}
        className={inputClass}
      />
      <p id={errorId} role="alert" className="text-sm text-accent empty:hidden">
        {error}
      </p>
    </div>
  );
}

function LanguageField({ form, error }: FieldProps) {
  const { t, i18n } = useTranslation(settingsNamespace);
  const selectId = useId();
  const hintId = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={selectId} className="text-sm font-medium">
        {t("libraries.dialog.language")}
      </label>
      <select
        id={selectId}
        value={form.draft.language}
        aria-describedby={hintId}
        aria-invalid={error !== undefined}
        onChange={(event) => {
          form.setLanguage(event.target.value);
        }}
        className={inputClass}
      >
        {metadataLanguageCodes.map((code) => (
          <option key={code} value={code}>
            {code === ""
              ? t("libraries.dialog.languageDefault")
              : languageName(code, i18n.language)}
          </option>
        ))}
      </select>
      <p id={hintId} className="text-xs text-text-muted">
        {t("libraries.dialog.languageHint")}
      </p>
      <p role="alert" className="text-sm text-accent empty:hidden">
        {error}
      </p>
    </div>
  );
}

/** Tab "General": kind, name and metadata language. */
export function GeneralPane({ form, kinds, editing, errors }: GeneralPaneProps) {
  return (
    <>
      <KindTiles
        kinds={kinds}
        value={form.draft.kind}
        onChange={form.setKind}
        locked={editing}
        error={errors.kind}
      />
      <NameField form={form} error={errors.name} />
      <LanguageField form={form} error={errors.language} />
    </>
  );
}
