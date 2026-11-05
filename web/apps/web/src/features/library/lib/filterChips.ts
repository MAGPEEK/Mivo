import type { FilterLabels } from "./filterLabels";
import {
  fieldFilters,
  type FieldFilterId,
  type FieldFilterSelection,
  type QuickFilterId,
} from "./filterRegistry";

/** What removing a chip takes away: the quick filter or one value of a field. */
export type FilterChipTarget =
  | { readonly kind: "quick" }
  | { readonly kind: "field"; readonly field: FieldFilterId; readonly value: string };

/** One active filter under the toolbar. */
export interface FilterChipModel {
  readonly key: string;
  /** Visible text, e.g. "HDR" or "Genre: Drama". */
  readonly label: string;
  readonly target: FilterChipTarget;
}

/** The active filters to show as chips: the quick filter first, then field values in registry order. */
export function filterChips(
  state: { readonly quick: QuickFilterId; readonly fields: FieldFilterSelection },
  labels: FilterLabels,
  fieldValueLabel: (field: string, value: string) => string,
): readonly FilterChipModel[] {
  const quick: FilterChipModel[] =
    state.quick === "all"
      ? []
      : [{ key: "quick", label: labels.quick(state.quick), target: { kind: "quick" } }];
  const fields = fieldFilters.flatMap((definition) =>
    (state.fields[definition.id] ?? []).map((value) => ({
      key: `${definition.id}:${value}`,
      label: fieldValueLabel(labels.field(definition.id), labels.value(definition, value)),
      target: { kind: "field" as const, field: definition.id, value },
    })),
  );
  return [...quick, ...fields];
}
