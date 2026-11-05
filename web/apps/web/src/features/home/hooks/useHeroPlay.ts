import type { HeroItem } from "@mivo/api-client";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useCallback, useState } from "react";

import { useApiClient } from "../../../shared/api";
import { heroItemQueryOptions } from "../api/heroItemQuery";

/** How the hero's Play control starts a title. */
export type HeroPlay =
  /** A movie links straight to the player, which picks its primary file. */
  | { readonly kind: "link"; readonly itemId: string }
  /**
   * A show owns no file: pressing Play loads the show and opens the player on the episode it
   * names (`playItemId`), or the detail page when it has none or the load fails.
   */
  | { readonly kind: "show"; readonly busy: boolean; readonly start: () => void };

/** The Play action of a hero title (HO-01). */
export function useHeroPlay(item: HeroItem): HeroPlay {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const itemId = item.itemId;

  const start = useCallback(() => {
    if (busy) {
      return;
    }
    setBusy(true);
    void queryClient
      // Like the deprecated ensureQueryData: a cached item (e.g. from its detail page) is used
      // as it is, only a missing one is fetched.
      .query({ ...heroItemQueryOptions(client, itemId), staleTime: "static" })
      .then((show) => show.playItemId)
      .catch(() => undefined)
      .then((playItemId) =>
        playItemId === undefined
          ? navigate({ to: "/items/$itemId", params: { itemId } })
          : navigate({ to: "/items/$itemId/play", params: { itemId: playItemId }, search: {} }),
      )
      .finally(() => {
        setBusy(false);
      });
  }, [busy, client, itemId, navigate, queryClient]);

  return item.kind === "movie" ? { kind: "link", itemId } : { kind: "show", busy, start };
}
