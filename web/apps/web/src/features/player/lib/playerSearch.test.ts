import { describe, expect, it } from "vitest";

import { validatePlayerSearch, type PlayerSearch } from "./playerSearch";

const noSearch: PlayerSearch = { fileId: undefined, t: undefined };

describe("validatePlayerSearch", () => {
  it.each<{ name: string; search: Record<string, unknown>; expected: PlayerSearch }>([
    { name: "no parameters", search: {}, expected: noSearch },
    {
      name: "file and start position",
      search: { fileId: "file-1", t: 90 },
      expected: { fileId: "file-1", t: 90 },
    },
    { name: "start at zero", search: { t: 0 }, expected: { ...noSearch, t: 0 } },
    { name: "fractional start", search: { t: 12.5 }, expected: { ...noSearch, t: 12.5 } },
    { name: "empty file id", search: { fileId: "" }, expected: noSearch },
    { name: "numeric file id", search: { fileId: 42 }, expected: noSearch },
    { name: "object file id", search: { fileId: { id: "file-1" } }, expected: noSearch },
    { name: "negative start", search: { t: -1 }, expected: noSearch },
    { name: "start as text", search: { t: "90" }, expected: noSearch },
    { name: "start not a number", search: { t: Number.NaN }, expected: noSearch },
    { name: "infinite start", search: { t: Number.POSITIVE_INFINITY }, expected: noSearch },
    {
      name: "invalid start next to a valid file",
      search: { fileId: "file-1", t: -3 },
      expected: { fileId: "file-1", t: undefined },
    },
    {
      name: "unknown parameters",
      search: { quality: "high", t: 5 },
      expected: { ...noSearch, t: 5 },
    },
  ])("keeps only the valid values for $name", ({ search, expected }) => {
    expect(validatePlayerSearch(search)).toStrictEqual(expected);
  });
});
