import type { Facet } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { filterChips } from "./filterChips";
import { facetOptions, facetsByField, type Facets } from "./facetOptions";
import { filterValueLabel, type FilterLabels, type FilterValueTexts } from "./filterLabels";
import { filterMenuSections, type FilterMenuInput, type FilterMenuTexts } from "./filterMenuModel";
import type { FieldValueFormat } from "./filterRegistry";

const labels: FilterLabels = {
  quick: (id) => `quick:${id}`,
  field: (id) => `field:${id}`,
  value: (definition, value) => `${definition.id}=${value}`,
};

const texts: FilterMenuTexts = {
  labels,
  quickHeading: "Quick",
  fieldHeading: "Fields",
  back: "Back",
  loading: "Loading",
  loadFailed: "Failed",
  notSupported: "Not supported",
  noValues: "No values",
  count: (count) => String(count),
  valueWithCount: (label, count) => `${label} (${String(count)})`,
  fieldWithSelection: (label, count) => `${label} [${String(count)}]`,
};

function facet(field: Facet["field"], values: readonly [string, number][]): Facet {
  return { field, values: values.map(([value, count]) => ({ value, count })), truncated: false };
}

const loaded: Facets = {
  status: "success",
  notSupported: false,
  byField: facetsByField({
    facets: [
      facet("genre", [
        ["Comedy", 3],
        ["Drama", 5],
      ]),
      facet("year", []),
      facet("resolution", [["1080p", 2]]),
    ],
  }),
};

function input(overrides: Partial<FilterMenuInput>): FilterMenuInput {
  return { view: "root", quick: "all", fields: {}, facets: loaded, ...overrides };
}

describe("facetOptions", () => {
  it("offers counted values, hides zero counts and keeps chosen values without items", () => {
    const counted = facet("genre", [
      ["Comedy", 3],
      ["Horror", 0],
    ]);

    expect(facetOptions(counted, ["Western"])).toEqual([
      { value: "Comedy", count: 3, selected: false },
      { value: "Western", count: 0, selected: true },
    ]);
    expect(facetOptions(undefined)).toEqual([]);
  });

  it("ignores facets of fields this client does not know", () => {
    const list = { facets: [facet("genre", [["A", 1]]), facet("director" as "genre", [])] };

    expect([...facetsByField(list).keys()]).toEqual(["genre"]);
  });
});

describe("filterMenuSections", () => {
  it("lists the quick filters and only the fields with values", () => {
    const [quick, fields] = filterMenuSections(
      input({ quick: "hdr", fields: { genre: ["Drama"] } }),
      texts,
    );

    expect(quick?.items.filter((item) => item.checked === true).map((item) => item.key)).toEqual([
      "quick-hdr",
    ]);
    expect(fields?.items.map((item) => [item.label, item.accessibleLabel, item.detail])).toEqual([
      ["field:genre", "field:genre [1]", "1"],
      ["field:resolution", undefined, undefined],
    ]);
  });

  it("offers every field while the counts load", () => {
    const pending: Facets = { status: "pending", notSupported: false, byField: new Map() };
    const [, fields] = filterMenuSections(input({ facets: pending }), texts);

    expect(fields?.items).toHaveLength(10);
  });

  it("says so when no field has values", () => {
    const empty: Facets = { status: "success", notSupported: false, byField: new Map() };
    const [, fields] = filterMenuSections(input({ facets: empty }), texts);

    expect(fields?.items).toEqual([{ key: "no-fields", role: "menuitem", label: "No values" }]);
  });

  it("lists a field's values with counts and checks the chosen ones", () => {
    const [back, values] = filterMenuSections(
      input({ view: "genre", fields: { genre: ["Drama"] } }),
      texts,
    );

    expect(back?.items.map((item) => item.action)).toEqual([{ kind: "back" }]);
    expect(values?.heading).toBe("field:genre");
    expect(
      values?.items.map((item) => [item.role, item.accessibleLabel, item.detail, item.checked]),
    ).toEqual([
      ["menuitemcheckbox", "genre=Comedy (3)", "3", false],
      ["menuitemcheckbox", "genre=Drama (5)", "5", true],
    ]);
    expect(values?.items[1]?.action).toEqual({
      kind: "toggleValue",
      field: "genre",
      value: "Drama",
    });
  });

  it.each<{ name: string; facets: Facets; label: string }>([
    {
      name: "loading",
      facets: { status: "pending", notSupported: false, byField: new Map() },
      label: "Loading",
    },
    {
      name: "failed",
      facets: { status: "error", notSupported: false, byField: new Map() },
      label: "Failed",
    },
    {
      name: "not supported",
      facets: { status: "error", notSupported: true, byField: new Map() },
      label: "Not supported",
    },
    {
      name: "empty",
      facets: { status: "success", notSupported: false, byField: new Map() },
      label: "No values",
    },
  ])("shows a disabled status line while $name", ({ facets, label }) => {
    const [, values] = filterMenuSections(input({ view: "year", facets }), texts);

    expect(values?.items).toHaveLength(1);
    expect(values?.items[0]?.label).toBe(label);
    expect(values?.items[0]?.action).toBeUndefined();
  });
});

describe("filterValueLabel", () => {
  const valueTexts: FilterValueTexts = {
    uiLanguage: "en",
    decade: (year) => `${year}s`,
    unknownLanguage: "Unknown",
  };

  it.each<{ format: FieldValueFormat; value: string; expected: string }>([
    { format: "text", value: "Drama", expected: "Drama" },
    { format: "decade", value: "1990", expected: "1990s" },
    { format: "resolution", value: "4k", expected: "4K" },
    { format: "resolution", value: "sd", expected: "SD" },
    { format: "resolution", value: "1080p", expected: "1080p" },
    { format: "codec", value: "h264", expected: "H.264" },
    { format: "container", value: "mkv", expected: "MKV" },
    { format: "language", value: "ger", expected: "German" },
    { format: "language", value: "und", expected: "Unknown" },
  ])("shows $format $value as $expected", ({ format, value, expected }) => {
    expect(filterValueLabel(format, value, valueTexts)).toBe(expected);
  });
});

describe("filterChips", () => {
  it("lists the quick filter first, then each field value in registry order", () => {
    const chips = filterChips(
      { quick: "atmos", fields: { decade: ["1990"], genre: ["Drama", "Comedy"] } },
      labels,
      (field, value) => `${field}: ${value}`,
    );

    expect(chips.map((chip) => [chip.key, chip.label])).toEqual([
      ["quick", "quick:atmos"],
      ["genre:Drama", "field:genre: genre=Drama"],
      ["genre:Comedy", "field:genre: genre=Comedy"],
      ["decade:1990", "field:decade: decade=1990"],
    ]);
    expect(chips[1]?.target).toEqual({ kind: "field", field: "genre", value: "Drama" });
  });

  it("has no chips without filters", () => {
    expect(filterChips({ quick: "all", fields: {} }, labels, () => "")).toEqual([]);
  });
});
