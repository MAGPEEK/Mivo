import { codecLabel, containerLabel } from "@mivo/player-core";

import { languageName } from "../../../shared/lib/languageName";
import type {
  FieldFilterDefinition,
  FieldFilterId,
  FieldValueFormat,
  QuickFilterId,
} from "./filterRegistry";

/** Labels of filters and their values in the UI language. */
export interface FilterLabels {
  readonly quick: (id: QuickFilterId) => string;
  readonly field: (id: FieldFilterId) => string;
  readonly value: (definition: FieldFilterDefinition, value: string) => string;
}

/** The translated parts a filter value label may need. */
export interface FilterValueTexts {
  readonly uiLanguage: string;
  /** Label of a decade by its first year, e.g. "1990s". */
  readonly decade: (firstYear: string) => string;
  /** Shown for an undetermined language ("und"). */
  readonly unknownLanguage: string;
}

// Resolution classes are written as in the API ("1080p"), except the letter classes.
const upperCaseResolutions: ReadonlySet<string> = new Set(["sd", "4k", "8k"]);

function resolutionLabel(value: string): string {
  return upperCaseResolutions.has(value) ? value.toUpperCase() : value;
}

/** The label of a filter value as the menu and the chips show it. */
export function filterValueLabel(
  format: FieldValueFormat,
  value: string,
  texts: FilterValueTexts,
): string {
  switch (format) {
    case "decade":
      return texts.decade(value);
    case "resolution":
      return resolutionLabel(value);
    case "codec":
      return codecLabel(value);
    case "container":
      return containerLabel(value);
    case "language":
      return languageName(value, texts.uiLanguage) ?? texts.unknownLanguage;
    case "text":
      return value;
  }
}
