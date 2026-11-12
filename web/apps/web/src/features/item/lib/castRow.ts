import type { ItemPerson, PersonRole } from "@mivo/api-client";

/** One card of the cast and crew row. */
export interface CastMember {
  /** Stable key: the position keeps two credits of the same name apart. */
  readonly key: string;
  readonly name: string;
  /** One or two capital letters for the portrait placeholder. */
  readonly initials: string;
  /** The character an actor plays, else the translated role (e.g. "Director"). */
  readonly caption: string;
}

// Authors and narrators belong to audiobooks, which show them in the title block.
const hiddenRoles: ReadonlySet<PersonRole> = new Set<PersonRole>(["author", "narrator"]);
const maxInitials = 2;

/** The up to two initials of a name; a single word gives one letter. */
export function initialsOf(name: string): string {
  return Array.from(name.trim().split(/\s+/))
    .filter((word) => word !== "")
    .slice(0, maxInitials)
    .map((word) => Array.from(word)[0]?.toLocaleUpperCase() ?? "")
    .join("");
}

/**
 * The people of an item as cards, in the order the metadata lists them (cast by billing, then
 * crew). Blank names and the audiobook roles are dropped; `roleLabel` translates a role.
 */
export function toCastRow(
  people: readonly ItemPerson[] | undefined,
  roleLabel: (role: PersonRole) => string,
): CastMember[] {
  return (people ?? [])
    .filter((person) => person.name.trim() !== "" && !hiddenRoles.has(person.role))
    .map((person, position) => {
      const character = person.character?.trim() ?? "";
      return {
        key: `${String(position)}:${person.name}`,
        name: person.name.trim(),
        initials: initialsOf(person.name),
        caption: character === "" ? roleLabel(person.role) : character,
      };
    });
}
