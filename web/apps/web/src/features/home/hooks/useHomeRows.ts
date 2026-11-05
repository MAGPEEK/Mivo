import type { HeroItem, Row } from "@mivo/api-client";
import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import type { SidebarLibraries } from "../../../shared/ui";
import { useSidebarLibraries } from "../../library";
import { useArrangedLibraries } from "../../sidebar-layout";
import { homeRowsQueryOptions } from "../api/homeRowsQuery";
import {
  homeHint,
  visibleHeroItems,
  visibleHomeRows,
  type HomeHint,
  type HomeLayout,
} from "../lib/homeRows";

/** What Home shows below its heading. */
export type HomeRowsState =
  | { readonly state: "loading" }
  | { readonly state: "error"; readonly retry: () => void }
  | { readonly state: "noLibraries" }
  | {
      readonly state: "loaded";
      /** Hero titles of pinned libraries in server order; empty hides the hero. */
      readonly hero: readonly HeroItem[];
      /** Rows of pinned libraries in sidebar order. */
      readonly rows: readonly Row[];
      /** Shown instead of the rows when there are none, or below them. */
      readonly hint: HomeHint | undefined;
    };

const unknownLayout: HomeLayout = { order: [], unpinned: new Set() };

// The sidebar layout; unknown while the libraries load or failed, so a failing library list
// neither hides nor reorders anything.
function homeLayout(libraries: SidebarLibraries): HomeLayout {
  if (libraries.state !== "loaded") {
    return unknownLayout;
  }
  return {
    order: libraries.links.map((link) => link.id),
    unpinned: new Set(
      libraries.links.filter((link) => link.pinned === false).map((link) => link.id),
    ),
  };
}

/**
 * The Home rows (HO-02, HO-05, HO-07) and hero titles (HO-01) as the server built them,
 * limited to the libraries pinned in the sidebar and in its order, with the empty, loading
 * and error states. The client never builds row content itself.
 */
export function useHomeRows(): HomeRowsState {
  const client = useApiClient();
  const query = useQuery(homeRowsQueryOptions(client));
  const libraries = useArrangedLibraries(useSidebarLibraries());
  const noLibraries = libraries.state === "loaded" && libraries.links.length === 0;
  if (noLibraries || query.data?.hint === "noLibraries") {
    return { state: "noLibraries" };
  }
  // A failed refresh keeps the rows already shown; only a first load ends in the error state.
  if (query.data === undefined) {
    return query.isError && !query.isFetching
      ? { state: "error", retry: () => void query.refetch() }
      : { state: "loading" };
  }
  const layout = homeLayout(libraries);
  const serverRows = Array.isArray(query.data.rows) ? query.data.rows : [];
  const serverHero = Array.isArray(query.data.hero) ? query.data.hero : [];
  const rows = visibleHomeRows(serverRows, layout);
  return {
    state: "loaded",
    hero: visibleHeroItems(serverHero, layout.unpinned),
    rows,
    hint: homeHint(query.data.hint, { server: serverRows.length, visible: rows.length }),
  };
}
