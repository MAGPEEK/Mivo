import type { ExtraType, ItemSummary } from "@mivo/api-client";

/** Extras of one kind, in the order the server lists them (by title). */
export interface ExtraGroup {
  readonly type: ExtraType;
  readonly extras: readonly ItemSummary[];
}

// Order of the rows on the page: what viewers look for first comes first (DT-08).
const groupOrder: readonly ExtraType[] = [
  "trailer",
  "featurette",
  "behind_the_scenes",
  "deleted_scene",
  "interview",
  "scene",
  "short",
  "other",
];

// A type added in a later API version, or none at all, is shown with the others.
function typeOf(extra: ItemSummary): ExtraType {
  const type = extra.extraType;
  return type !== undefined && groupOrder.includes(type) ? type : "other";
}

/** Groups extras by kind in a fixed order; kinds without an extra are left out. */
export function groupExtras(extras: readonly ItemSummary[]): ExtraGroup[] {
  return groupOrder
    .map((type) => ({ type, extras: extras.filter((extra) => typeOf(extra) === type) }))
    .filter((group) => group.extras.length > 0);
}

/** The first trailer, which the trailer button plays; `undefined` when there is none. */
export function firstTrailer(extras: readonly ItemSummary[]): ItemSummary | undefined {
  return extras.find((extra) => extra.extraType === "trailer");
}
