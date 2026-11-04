import type { LibraryOptions } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import type { LibraryForm } from "../hooks/useLibraryForm";
import { settingsNamespace } from "../locales";

/** Props of {@link AdvancedPane}. */
export interface AdvancedPaneProps {
  readonly form: LibraryForm;
  /** Whether the library came from the configuration file. */
  readonly importedFromConfig: boolean;
}

// Option names with their labels, in display order. The labels are literal keys for type checks.
const optionLabels = [
  ["scanExtras", "libraries.dialog.options.scanExtras"],
  ["readNfo", "libraries.dialog.options.readNfo"],
  ["localArtwork", "libraries.dialog.options.localArtwork"],
] as const satisfies readonly (readonly [keyof LibraryOptions, string])[];

/** Tab "Advanced": the switches milestone 3 supports. */
export function AdvancedPane({ form, importedFromConfig }: AdvancedPaneProps) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <>
      <fieldset className="flex flex-col gap-3">
        <legend className="mb-2 text-sm font-medium">{t("libraries.dialog.options.legend")}</legend>
        {optionLabels.map(([option, labelKey]) => (
          <label key={option} className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={form.draft.options[option]}
              onChange={(event) => {
                form.setOption(option, event.target.checked);
              }}
            />
            {t(labelKey)}
          </label>
        ))}
      </fieldset>
      {importedFromConfig && (
        <p className="mt-3 text-xs text-text-muted">{t("libraries.dialog.importedHint")}</p>
      )}
    </>
  );
}
