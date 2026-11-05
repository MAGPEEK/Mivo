import type { TFunction } from "i18next";
import { useState, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";

import { browserLocalStorage } from "../../../shared/lib/persistentFlag";
import type {
  LayoutAnnouncement,
  SidebarLayoutControls,
  SidebarLibraries,
  SidebarLibraryLink,
} from "../../../shared/ui";
import { createLocalLayoutStore, type LayoutStore } from "../lib/layoutStore";
import {
  arrangeLibraries,
  moveLibraryBy,
  moveLibraryTo,
  toggleLibraryPinned,
  toLayout,
  type ArrangedLibrary,
} from "../lib/libraryLayout";

/**
 * The layout store of this component, from `store` or else on `localStorage`. Kept for the
 * component's lifetime, so the subscription stays stable.
 */
export function useLayoutStore(store?: LayoutStore): LayoutStore {
  const [layoutStore] = useState(() => store ?? createLocalLayoutStore(browserLocalStorage()));
  return layoutStore;
}

/**
 * Applies the user's library layout to the sidebar's libraries: all links in the user's order
 * with their pin state, plus the controls to reorder, pin and reset (SH-12 ... SH-14). Other
 * states pass through unchanged. `store` defaults to this device's `localStorage`.
 */
export function useArrangedLibraries(
  libraries: SidebarLibraries,
  store?: LayoutStore,
): SidebarLibraries {
  const { t } = useTranslation();
  const layoutStore = useLayoutStore(store);
  const layout = useSyncExternalStore(layoutStore.subscribe, layoutStore.read);
  const [announcement, setAnnouncement] = useState<LayoutAnnouncement>({ text: "", id: 0 });
  if (libraries.state !== "loaded") {
    return libraries;
  }
  const linksById = new Map(libraries.links.map((link) => [link.id, link]));
  const arranged = arrangeLibraries([...linksById.keys()], layout);
  const controls = createLayoutControls({
    arranged,
    nameOf: (id) => linksById.get(id)?.name ?? "",
    store: layoutStore,
    // A new id per message lets the live region repeat an identical text (e.g. two resets).
    announce: (text) => {
      setAnnouncement((previous) => ({ text, id: previous.id + 1 }));
    },
    t,
  });
  return {
    state: "loaded",
    links: arranged.flatMap(({ id, pinned }) => withPin(linksById.get(id), pinned)),
    layout: { ...controls, announcement },
  };
}

function withPin(link: SidebarLibraryLink | undefined, pinned: boolean): SidebarLibraryLink[] {
  return link === undefined ? [] : [{ ...link, pinned }];
}

interface LayoutControlsOptions {
  readonly arranged: readonly ArrangedLibrary[];
  readonly nameOf: (id: string) => string;
  readonly store: LayoutStore;
  readonly announce: (message: string) => void;
  readonly t: TFunction;
}

// Every change saves the whole arrangement, so libraries the server dropped leave the saved
// layout too, and tells screen reader users what happened (live region in the sidebar).
function createLayoutControls(
  options: LayoutControlsOptions,
): Omit<SidebarLayoutControls, "announcement"> {
  const { arranged, nameOf, store, announce, t } = options;
  const announceMove = (id: string, list: readonly ArrangedLibrary[]) => {
    const position = String(list.findIndex((library) => library.id === id) + 1);
    announce(
      t("sidebar.libraries.layout.moved", {
        name: nameOf(id),
        position,
        count: String(list.length),
      }),
    );
  };
  return {
    move: (id, offset) => {
      const next = moveLibraryBy(arranged, id, offset);
      store.write(toLayout(next));
      announceMove(id, next);
    },
    // Dragging happens in the pinned list, so the position counts pinned libraries only.
    moveTo: (id, targetId) => {
      const next = moveLibraryTo(arranged, id, targetId);
      store.write(toLayout(next));
      announceMove(
        id,
        next.filter((library) => library.pinned),
      );
    },
    togglePinned: (id) => {
      const next = toggleLibraryPinned(arranged, id);
      store.write(toLayout(next));
      const pinned = next.some((library) => library.id === id && library.pinned);
      const key = pinned
        ? "sidebar.libraries.layout.pinnedNotice"
        : "sidebar.libraries.layout.unpinnedNotice";
      announce(t(key, { name: nameOf(id) }));
    },
    reset: () => {
      store.write(undefined);
      announce(t("sidebar.libraries.layout.resetNotice"));
    },
  };
}

/** Restores the default library layout: server order, everything pinned (SH-14, for M5-80). */
export function useResetLibraryLayout(store?: LayoutStore): () => void {
  const layoutStore = useLayoutStore(store);
  return () => {
    layoutStore.write(undefined);
  };
}
