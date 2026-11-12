import type { ItemView } from "../lib/itemView";
import { CastRow } from "./CastRow";
import { ExtrasRows } from "./ExtrasRows";

/** The rows below the header of a movie or show: cast and crew, then extras by kind. */
export function ItemExtras({ item }: { readonly item: ItemView }) {
  if (item.people.length === 0 && item.extras.length === 0) {
    return null;
  }
  return (
    <div className="mt-8 flex flex-col gap-8">
      <CastRow people={item.people} />
      <ExtrasRows extras={item.extras} />
    </div>
  );
}
