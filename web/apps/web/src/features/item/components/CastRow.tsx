import type { ItemPerson } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { MediaRow } from "../../../shared/ui";
import { useCast } from "../hooks/useCast";
import { itemNamespace } from "../locales";

/** Props of {@link CastRow}. */
export interface CastRowProps {
  readonly people: readonly ItemPerson[];
}

/**
 * Cast and crew of an item as a row of round portraits (DT-08). The API carries no portraits
 * yet, so every card shows the initials; there is no person page to link to before M6. The row
 * is hidden when nobody is credited.
 */
export function CastRow({ people }: CastRowProps) {
  const { t } = useTranslation(itemNamespace);
  const cast = useCast(people);
  if (cast.length === 0) {
    return null;
  }
  return (
    <MediaRow
      title={t("cast.heading")}
      items={cast}
      format="poster"
      getKey={(member) => member.key}
      renderCard={(member) => (
        <div className="flex flex-col items-center gap-2 text-center">
          <div
            aria-hidden="true"
            className="grid size-28 place-items-center rounded-full border border-divider bg-surface-raised text-2xl font-semibold text-text-muted"
          >
            {member.initials}
          </div>
          <p className="w-full text-sm font-medium break-words text-text">{member.name}</p>
          <p className="w-full text-xs break-words text-text-muted">{member.caption}</p>
        </div>
      )}
    />
  );
}
