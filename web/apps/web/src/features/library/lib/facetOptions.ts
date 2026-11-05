import type { Facet, FacetList } from "@mivo/api-client";

import { fieldFilterById, type FieldFilterId } from "./filterRegistry";

/** The counted facets per known field; fields of a newer server are left out. */
export type FacetsByField = ReadonlyMap<FieldFilterId, Facet>;

/** The value counts of the filter menu (LB-20) as far as they are loaded. */
export interface Facets {
  /** `success` once counts are known; while new counts load the previous ones stay. */
  readonly status: "pending" | "error" | "success";
  /** With status `error`: the server cannot count yet (501). */
  readonly notSupported: boolean;
  readonly byField: FacetsByField;
}

/** One value the filter menu offers for a field. */
export interface FacetOption {
  readonly value: string;
  /** Items with this value under the other filters; 0 only for a chosen value. */
  readonly count: number;
  readonly selected: boolean;
}

/** The facets of `list` by field, without fields this client does not know. */
export function facetsByField(list: FacetList): FacetsByField {
  const byField = new Map<FieldFilterId, Facet>();
  for (const facet of list.facets) {
    const definition = fieldFilterById(facet.field);
    if (definition !== undefined) {
      byField.set(definition.id, facet);
    }
  }
  return byField;
}

/**
 * The values to offer for a field: every counted value in server order, chosen ones marked.
 * Values without items are hidden, except chosen ones: they stay at the end with count 0, so
 * they can still be removed.
 */
export function facetOptions(
  facet: Facet | undefined,
  selected: readonly string[] = [],
): readonly FacetOption[] {
  const counted = (facet?.values ?? [])
    .filter(({ count }) => count > 0)
    .map(({ value, count }) => ({ value, count, selected: selected.includes(value) }));
  const missing = selected
    .filter((value) => !counted.some((option) => option.value === value))
    .map((value) => ({ value, count: 0, selected: true }));
  return [...counted, ...missing];
}
