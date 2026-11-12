import type { ItemPerson } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { initialsOf, toCastRow } from "./castRow";

const roleLabel = (role: string): string => `role:${role}`;

describe("initialsOf", () => {
  it.each([
    ["Ines Carvalho", "IC"],
    ["  mara   lindqvist  ", "ML"],
    ["Kofi", "K"],
    ["Anna Maria von Trapp", "AM"],
    ["Élodie Roux", "ÉR"],
    ["   ", ""],
  ])("%s gives %s", (name, expected) => {
    expect(initialsOf(name)).toBe(expected);
  });
});

describe("toCastRow", () => {
  it("shows the character of an actor and the role of the crew", () => {
    const people: ItemPerson[] = [
      { name: "Ines Carvalho", role: "actor", character: "Ines" },
      { name: "Mara Lindqvist", role: "director" },
      { name: "Tomas Aure", role: "actor", character: "  " },
    ];

    expect(toCastRow(people, roleLabel).map((member) => [member.name, member.caption])).toEqual([
      ["Ines Carvalho", "Ines"],
      ["Mara Lindqvist", "role:director"],
      ["Tomas Aure", "role:actor"],
    ]);
  });

  it("drops blank names and audiobook roles and keeps equal names apart", () => {
    const people: ItemPerson[] = [
      { name: " ", role: "actor" },
      { name: "Ada Inkwell", role: "author" },
      { name: "Nora Vale", role: "narrator" },
      { name: "Sam Hill", role: "actor" },
      { name: "Sam Hill", role: "producer" },
    ];

    const cast = toCastRow(people, roleLabel);

    expect(cast.map((member) => member.name)).toEqual(["Sam Hill", "Sam Hill"]);
    expect(new Set(cast.map((member) => member.key)).size).toBe(2);
  });

  it("is empty without people", () => {
    expect(toCastRow(undefined, roleLabel)).toEqual([]);
  });
});
