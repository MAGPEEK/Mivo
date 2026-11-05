import { useTranslation } from "react-i18next";

import { filterValueLabel, type FilterLabels } from "../lib/filterLabels";
import { libraryNamespace } from "../locales";

/** The labels of the filter registry's entries in the UI language, for the menu and the chips. */
export function useFilterLabels(): FilterLabels {
  const { t, i18n } = useTranslation(libraryNamespace);
  const texts = {
    uiLanguage: i18n.language,
    decade: (firstYear: string) => t("filter.decadeValue", { decade: firstYear }),
    unknownLanguage: t("filter.unknownLanguage"),
  };
  return {
    quick: (id) => t(`filter.quick.${id}`),
    field: (id) => t(`filter.field.${id}`),
    value: (definition, value) => filterValueLabel(definition.format, value, texts),
  };
}
