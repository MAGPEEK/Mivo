import type { FacetField, ItemQuickFilter } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import {
  countFieldValues,
  fieldFilterById,
  fieldFilters,
  fieldQueryValues,
  maxFieldValues,
  parseFieldValues,
  quickFilterById,
  quickFilters,
  toggleFieldValue,
  type FieldFilterDefinition,
} from "./filterRegistry";

function field(id: string): FieldFilterDefinition {
  const definition = fieldFilterById(id);
  if (definition === undefined) {
    throw new Error(`no field ${id}`);
  }
  return definition;
}

describe("filter registry", () => {
  it("offers every quick filter of the task in order, Unmatched last", () => {
    expect(quickFilters.map(({ id }) => id)).toEqual([
      "all",
      "hdr",
      "dolbyVision",
      "hdr10plus",
      "atmos",
      "uhd",
      "unavailable",
      "duplicates",
      "unmatched",
    ]);
  });

  it("maps every quick filter to its listing parameter", () => {
    const serverValues: readonly ItemQuickFilter[] = [
      "hdr",
      "dolbyVision",
      "hdr10plus",
      "atmos",
      "uhd",
      "unavailable",
      "duplicates",
    ];
    for (const value of serverValues) {
      expect(quickFilterById(value)?.parameter).toEqual({ name: "quick", value });
    }
    expect(quickFilterById("unmatched")?.parameter).toEqual({
      name: "matchState",
      value: "unmatched",
    });
    expect(quickFilterById("all")?.parameter).toBeUndefined();
    expect(quickFilterById("unwatched")).toBeUndefined();
  });

  it("has one field per facet field of the API", () => {
    const facetFields: readonly FacetField[] = [
      "genre",
      "year",
      "decade",
      "contentRating",
      "resolution",
      "videoCodec",
      "audioCodec",
      "container",
      "audioLanguage",
      "subtitleLanguage",
    ];
    expect(fieldFilters.map(({ id }) => id)).toEqual(facetFields);
    expect(fieldFilterById("director")).toBeUndefined();
  });
});

describe("parseFieldValues", () => {
  it.each<{ name: string; id: string; raw: unknown; expected: readonly string[] | undefined }>([
    { name: "one text value", id: "genre", raw: "Drama", expected: ["Drama"] },
    { name: "a list", id: "genre", raw: ["Drama", "Comedy"], expected: ["Drama", "Comedy"] },
    { name: "duplicates", id: "genre", raw: ["Drama", "Drama"], expected: ["Drama"] },
    { name: "blank text", id: "genre", raw: [" ", ""], expected: undefined },
    { name: "too long text", id: "audioLanguage", raw: "x".repeat(36), expected: undefined },
    { name: "objects", id: "genre", raw: [{}, null], expected: undefined },
    { name: "nothing", id: "genre", raw: undefined, expected: undefined },
    { name: "a year as number", id: "year", raw: [2008], expected: ["2008"] },
    { name: "a year as text", id: "year", raw: "1999", expected: ["1999"] },
    { name: "a short year", id: "year", raw: "99", expected: undefined },
    { name: "a fractional year", id: "year", raw: 1999.5, expected: undefined },
    { name: "a decade", id: "decade", raw: [1990, "2000"], expected: ["1990", "2000"] },
    { name: "a year as decade", id: "decade", raw: 1995, expected: undefined },
    { name: "a decade before 1000", id: "decade", raw: "0990", expected: undefined },
  ])("keeps the valid values of $name", ({ id, raw, expected }) => {
    expect(parseFieldValues(field(id), raw)).toEqual(expected);
  });

  it("keeps at most the values the listing accepts", () => {
    const many = Array.from({ length: maxFieldValues + 5 }, (_, index) => `Genre ${String(index)}`);

    expect(parseFieldValues(field("genre"), many)).toHaveLength(maxFieldValues);
  });
});

describe("fieldQueryValues", () => {
  it.each([
    { id: "genre", values: ["Drama"], expected: ["Drama"] },
    { id: "year", values: ["2008"], expected: [2008] },
    { id: "decade", values: ["1990", "2000"], expected: [1990, 2000] },
    { id: "resolution", values: ["4k"], expected: ["4k"] },
  ])("sends $id as $expected", ({ id, values, expected }) => {
    expect(fieldQueryValues(field(id), values)).toEqual(expected);
  });
});

describe("toggleFieldValue and countFieldValues", () => {
  it("adds a value, removes it again and drops the emptied field", () => {
    const added = toggleFieldValue({ year: ["2001"] }, "genre", "Drama");
    expect(added).toEqual({ year: ["2001"], genre: ["Drama"] });
    expect(countFieldValues(added)).toBe(2);

    const removed = toggleFieldValue(added, "genre", "Drama");
    expect(removed).toEqual({ year: ["2001"], genre: undefined });
    expect(countFieldValues(removed)).toBe(1);
  });
});
