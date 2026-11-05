import { facetOptions, type Facets } from "./facetOptions";
import type { FilterLabels } from "./filterLabels";
import {
  fieldFilters,
  quickFilters,
  type FieldFilterDefinition,
  type FieldFilterId,
  type FieldFilterSelection,
  type QuickFilterId,
} from "./filterRegistry";

// Content of the filter menu (LB-20) as plain data, built from the filter registry: the root
// view lists the quick filters and the fields, a field view lists that field's values with
// their counts. The component only renders these sections.

/** What the menu shows: the root view or the values of one field. */
export type FilterMenuView = "root" | FieldFilterId;

/** What choosing a menu item does. */
export type FilterMenuAction =
  | { readonly kind: "quick"; readonly id: QuickFilterId }
  | { readonly kind: "openField"; readonly field: FieldFilterId }
  | { readonly kind: "toggleValue"; readonly field: FieldFilterId; readonly value: string }
  | { readonly kind: "back" };

/** One item of the menu. */
export interface FilterMenuItem {
  readonly key: string;
  readonly role: "menuitem" | "menuitemradio" | "menuitemcheckbox";
  /** Visible label; type-ahead searches it. */
  readonly label: string;
  /** Accessible name when it says more than the label, e.g. the count. */
  readonly accessibleLabel?: string | undefined;
  /** Shown at the end of the item, e.g. a count. */
  readonly detail?: string | undefined;
  readonly checked?: boolean | undefined;
  /** Absent for a status line (loading, error), which is shown disabled. */
  readonly action?: FilterMenuAction | undefined;
}

/** A group of items, optionally under a heading. */
export interface FilterMenuSection {
  readonly key: string;
  readonly heading?: string | undefined;
  readonly items: readonly FilterMenuItem[];
}

/** The translated texts of the menu. */
export interface FilterMenuTexts {
  readonly labels: FilterLabels;
  readonly quickHeading: string;
  readonly fieldHeading: string;
  readonly back: string;
  readonly loading: string;
  readonly loadFailed: string;
  readonly notSupported: string;
  readonly noValues: string;
  readonly count: (count: number) => string;
  readonly valueWithCount: (label: string, count: number) => string;
  readonly fieldWithSelection: (label: string, selected: number) => string;
}

/** What the menu is built from. */
export interface FilterMenuInput {
  readonly view: FilterMenuView;
  readonly quick: QuickFilterId;
  readonly fields: FieldFilterSelection;
  readonly facets: Facets;
}

function quickSection({ quick }: FilterMenuInput, texts: FilterMenuTexts): FilterMenuSection {
  return {
    key: "quick",
    heading: texts.quickHeading,
    items: quickFilters.map(({ id }) => ({
      key: `quick-${id}`,
      role: "menuitemradio",
      label: texts.labels.quick(id),
      checked: id === quick,
      action: { kind: "quick", id },
    })),
  };
}

// Until the counts are known every field is offered; then fields without values are hidden.
function hasValues(input: FilterMenuInput, field: FieldFilterId): boolean {
  const { facets, fields } = input;
  return (
    facets.status !== "success" || facetOptions(facets.byField.get(field), fields[field]).length > 0
  );
}

function statusItem(key: string, label: string): FilterMenuItem {
  return { key, role: "menuitem", label };
}

function fieldItem(
  definition: FieldFilterDefinition,
  input: FilterMenuInput,
  texts: FilterMenuTexts,
): FilterMenuItem {
  const label = texts.labels.field(definition.id);
  const selected = input.fields[definition.id]?.length ?? 0;
  return {
    key: `field-${definition.id}`,
    role: "menuitem",
    label,
    accessibleLabel: selected > 0 ? texts.fieldWithSelection(label, selected) : undefined,
    detail: selected > 0 ? texts.count(selected) : undefined,
    action: { kind: "openField", field: definition.id },
  };
}

function fieldSection(input: FilterMenuInput, texts: FilterMenuTexts): FilterMenuSection {
  const items = fieldFilters
    .filter(({ id }) => hasValues(input, id))
    .map((definition) => fieldItem(definition, input, texts));
  return {
    key: "fields",
    heading: texts.fieldHeading,
    items: items.length > 0 ? items : [statusItem("no-fields", texts.noValues)],
  };
}

function facetStatusItem({ facets }: FilterMenuInput, texts: FilterMenuTexts): FilterMenuItem {
  if (facets.status === "pending") {
    return statusItem("loading", texts.loading);
  }
  return statusItem("failed", facets.notSupported ? texts.notSupported : texts.loadFailed);
}

function valueItems(
  definition: FieldFilterDefinition,
  input: FilterMenuInput,
  texts: FilterMenuTexts,
): readonly FilterMenuItem[] {
  if (input.facets.status !== "success") {
    return [facetStatusItem(input, texts)];
  }
  const options = facetOptions(
    input.facets.byField.get(definition.id),
    input.fields[definition.id],
  );
  if (options.length === 0) {
    return [statusItem("no-values", texts.noValues)];
  }
  return options.map(({ value, count, selected }) => {
    const label = texts.labels.value(definition, value);
    return {
      key: `value-${value}`,
      role: "menuitemcheckbox",
      label,
      accessibleLabel: texts.valueWithCount(label, count),
      detail: texts.count(count),
      checked: selected,
      action: { kind: "toggleValue", field: definition.id, value },
    };
  });
}

function fieldViewSections(
  definition: FieldFilterDefinition,
  input: FilterMenuInput,
  texts: FilterMenuTexts,
): readonly FilterMenuSection[] {
  return [
    {
      key: "back",
      items: [{ key: "back", role: "menuitem", label: texts.back, action: { kind: "back" } }],
    },
    {
      key: "values",
      heading: texts.labels.field(definition.id),
      items: valueItems(definition, input, texts),
    },
  ];
}

/** The sections of the menu in `input.view`; an unknown field shows the root view. */
export function filterMenuSections(
  input: FilterMenuInput,
  texts: FilterMenuTexts,
): readonly FilterMenuSection[] {
  const definition = fieldFilters.find(({ id }) => id === input.view);
  return definition === undefined
    ? [quickSection(input, texts), fieldSection(input, texts)]
    : fieldViewSections(definition, input, texts);
}
