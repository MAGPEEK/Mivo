import type { ItemPerson, PersonRole } from "@mivo/api-client";

/** The names credited with `role`, in metadata order. */
export function creditedNames(
  people: readonly ItemPerson[] | undefined,
  role: PersonRole,
): string[] {
  return (people ?? []).filter((person) => person.role === role).map((person) => person.name);
}

/**
 * Joins names the way the UI language lists them ("A, B and C", "A, B und C"); "" for none.
 */
export function joinNames(names: readonly string[], language: string): string {
  return new Intl.ListFormat(language, { style: "long", type: "conjunction" }).format(names);
}
