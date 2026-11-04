import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { useApiClient } from "../../../shared/api";
import { itemQueryOptions } from "../api/itemQuery";
import type { ItemView } from "../lib/itemView";
import { itemNamespace } from "../locales";

/** A link to the previous or next season or episode. */
export interface SiblingLink {
  readonly itemId: string;
  /** Accessible name, e.g. "Previous episode: Pilot"; without the title while it loads. */
  readonly label: string;
}

/** The arrows next to a season or episode; an end of the list has no link. */
export interface SiblingLinks {
  readonly previous: SiblingLink | undefined;
  readonly next: SiblingLink | undefined;
}

type SiblingKind = "season" | "episode";

// The detail has no sibling titles, only IDs; the sibling's own detail is cached, so opening
// it afterwards costs nothing.
function useSiblingTitle(siblingId: string | undefined): string | undefined {
  const client = useApiClient();
  const query = useQuery({
    ...itemQueryOptions(client, siblingId ?? ""),
    enabled: siblingId !== undefined,
  });
  return query.data?.title;
}

/** The previous and next season or episode of `item`, named for assistive technology. */
export function useSiblingLinks(item: ItemView, kind: SiblingKind): SiblingLinks {
  const { t } = useTranslation(itemNamespace);
  const previousTitle = useSiblingTitle(item.previousId);
  const nextTitle = useSiblingTitle(item.nextId);
  const label = (direction: "previous" | "next", title: string | undefined) => {
    const key = `sibling.${direction}.${kind}` as const;
    return title === undefined ? t(`${key}.untitled`) : t(`${key}.titled`, { title });
  };
  return {
    previous:
      item.previousId === undefined
        ? undefined
        : { itemId: item.previousId, label: label("previous", previousTitle) },
    next:
      item.nextId === undefined
        ? undefined
        : { itemId: item.nextId, label: label("next", nextTitle) },
  };
}
