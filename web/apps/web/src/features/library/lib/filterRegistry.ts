import type { FacetField, ItemQuickFilter } from "@mivo/api-client";

// The filters of the Browse pivot (LB-20) as data. The filter menu, the chips, the URL and the
// listing query all iterate these tables, so a later field (people, collection, M6/M7) or quick
// filter (watch state, M7) is one more row here plus its strings, not a new code branch.

/** A quick filter the server's `quick` parameter knows, without the default "all". */
export type ServerQuickFilter = Exclude<ItemQuickFilter, "all">;

/** A quick filter of the menu: the server's quick filters, "all" and "unmatched". */
export type QuickFilterId = "all" | ServerQuickFilter | "unmatched";

/** The URL and listing parameters that carry a quick filter. */
export type QuickFilterParameter = "quick" | "matchState";

/** One quick filter: single choice, the first one ("all") filters nothing. */
export interface QuickFilterDefinition {
  readonly id: QuickFilterId;
  /** The parameter and value that apply the filter; absent for "all". */
  readonly parameter?: { readonly name: QuickFilterParameter; readonly value: string } | undefined;
}

function serverQuick(id: ServerQuickFilter): QuickFilterDefinition {
  return { id, parameter: { name: "quick", value: id } };
}

/** Every quick filter in menu order. */
export const quickFilters: readonly QuickFilterDefinition[] = [
  { id: "all" },
  serverQuick("hdr"),
  serverQuick("dolbyVision"),
  serverQuick("hdr10plus"),
  serverQuick("atmos"),
  serverQuick("uhd"),
  serverQuick("unavailable"),
  serverQuick("duplicates"),
  // "Not matched" is the listing's matchState filter (ADR 0031), not a `quick` value.
  { id: "unmatched", parameter: { name: "matchState", value: "unmatched" } },
];

/** A field filter: named like its listing parameter and facet field. */
export type FieldFilterId = FacetField;

/** How URL values of a field are checked: free text, a year or a decade's first year. */
export type FieldValueKind = "text" | "year" | "decade";

/** How a value is shown in the menu and on a chip. */
export type FieldValueFormat =
  "text" | "decade" | "resolution" | "codec" | "container" | "language";

/** One field filter: several values combine with OR, fields with AND. */
export interface FieldFilterDefinition {
  readonly id: FieldFilterId;
  readonly kind: FieldValueKind;
  /** Longest text value the listing accepts. */
  readonly maxLength: number;
  readonly format: FieldValueFormat;
}

// Lengths from the listing's parameter schemas (api/openapi.yaml).
const genreMaxLength = 200;
const contentRatingMaxLength = 100;
const codecMaxLength = 64;
const languageMaxLength = 35;
const resolutionMaxLength = 16;
const yearLength = 4;

/** Every field filter in menu order. */
export const fieldFilters: readonly FieldFilterDefinition[] = [
  { id: "genre", kind: "text", maxLength: genreMaxLength, format: "text" },
  { id: "year", kind: "year", maxLength: yearLength, format: "text" },
  { id: "decade", kind: "decade", maxLength: yearLength, format: "decade" },
  { id: "contentRating", kind: "text", maxLength: contentRatingMaxLength, format: "text" },
  { id: "resolution", kind: "text", maxLength: resolutionMaxLength, format: "resolution" },
  { id: "videoCodec", kind: "text", maxLength: codecMaxLength, format: "codec" },
  { id: "audioCodec", kind: "text", maxLength: codecMaxLength, format: "codec" },
  { id: "container", kind: "text", maxLength: codecMaxLength, format: "container" },
  { id: "audioLanguage", kind: "text", maxLength: languageMaxLength, format: "language" },
  { id: "subtitleLanguage", kind: "text", maxLength: languageMaxLength, format: "language" },
];

/** The chosen values per field; a field without values is absent or `undefined`. */
export type FieldFilterSelection = Readonly<
  Partial<Record<FieldFilterId, readonly string[] | undefined>>
>;

/** Most values per field the listing accepts. */
export const maxFieldValues = 50;

const minYear = 1000;
const maxYear = 9999;
const yearsPerDecade = 10;

/** The field filter with this id, or `undefined` for a field this client does not know. */
export function fieldFilterById(id: string): FieldFilterDefinition | undefined {
  return fieldFilters.find((definition) => definition.id === id);
}

/** The quick filter with this id, or `undefined`. */
export function quickFilterById(id: string): QuickFilterDefinition | undefined {
  return quickFilters.find((definition) => definition.id === id);
}

function isValidValue(definition: FieldFilterDefinition, value: string): boolean {
  if (definition.kind === "text") {
    return value.trim() !== "" && value.length <= definition.maxLength;
  }
  if (!/^\d{4}$/.test(value)) {
    return false;
  }
  const year = Number(value);
  const inRange = year >= minYear && year <= maxYear;
  return definition.kind === "year" ? inRange : inRange && year % yearsPerDecade === 0;
}

function rawValues(raw: unknown): readonly unknown[] {
  // The router parses `?genre=["Drama"]` to an array; a hand-edited link may carry one value.
  return Array.isArray(raw) ? raw : [raw];
}

function asText(value: unknown): string | undefined {
  if (typeof value === "string") {
    return value;
  }
  return typeof value === "number" && Number.isInteger(value) ? String(value) : undefined;
}

/**
 * The valid values of a field from a URL or a row's query: text and integers are accepted,
 * duplicates and invalid values dropped, at most {@link maxFieldValues} kept. `undefined` when
 * nothing valid is left, so the field stays out of the URL.
 */
export function parseFieldValues(
  definition: FieldFilterDefinition,
  raw: unknown,
): readonly string[] | undefined {
  const values: string[] = [];
  for (const candidate of rawValues(raw)) {
    const value = asText(candidate);
    if (value !== undefined && isValidValue(definition, value) && !values.includes(value)) {
      values.push(value);
    }
  }
  const kept = values.slice(0, maxFieldValues);
  return kept.length === 0 ? undefined : kept;
}

/** The listing parameter value of a field: years and decades as numbers, the rest as text. */
export function fieldQueryValues(
  definition: FieldFilterDefinition,
  values: readonly string[],
): readonly string[] | readonly number[] {
  return definition.kind === "text" ? values : values.map(Number);
}

/** `selection` with `value` of `field` added or, when present, removed. */
export function toggleFieldValue(
  selection: FieldFilterSelection,
  field: FieldFilterId,
  value: string,
): FieldFilterSelection {
  const current = selection[field] ?? [];
  const next = current.includes(value)
    ? current.filter((candidate) => candidate !== value)
    : [...current, value].slice(0, maxFieldValues);
  return { ...selection, [field]: next.length === 0 ? undefined : next };
}

/** How many field values are chosen across all fields. */
export function countFieldValues(selection: FieldFilterSelection): number {
  return fieldFilters.reduce((sum, { id }) => sum + (selection[id]?.length ?? 0), 0);
}
