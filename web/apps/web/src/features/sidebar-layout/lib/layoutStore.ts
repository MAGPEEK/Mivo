import { productId } from "@mivo/ui-tokens";

import { parseStoredLayout, serializeLayout, type LibraryLayout } from "./libraryLayout";

/**
 * Where the library layout is kept. Milestone 5 keeps it per device in `localStorage`;
 * milestone 7 swaps in a server-backed store per profile (SH-16) behind the same interface.
 */
export interface LayoutStore {
  /** The saved layout, or `undefined` for the default. Returns the same object until it changes. */
  readonly read: () => LibraryLayout | undefined;
  /** Saves a layout; `undefined` resets to the default. */
  readonly write: (layout: LibraryLayout | undefined) => void;
  /** Calls `onChange` after every change, also from other tabs; returns the unsubscribe. */
  readonly subscribe: (onChange: () => void) => () => void;
}

/** `localStorage` key of the library layout. Derived from the brand id (ADR 0002). */
export const libraryLayoutStorageKey = `${productId}.sidebar.layout`;

// `storage` events only reach other tabs, so writes in this tab announce themselves with an
// own event. Using the window as the channel lets the sidebar and the settings page each
// create a store and still see each other's changes. The event carries the layout, so a
// layout that could not be saved (blocked storage) still reaches every subscribed store.
const changeEventName = `${libraryLayoutStorageKey}.change`;

interface LayoutChange {
  readonly layout: LibraryLayout | undefined;
  readonly saved: boolean;
}

function layoutChangeOf(event: Event): LayoutChange | undefined {
  return event instanceof CustomEvent ? (event.detail as LayoutChange) : undefined;
}

/**
 * A {@link LayoutStore} on `localStorage`. A missing or blocked storage, or a failed write,
 * leaves the default layout or the last one read; the arrangement then lasts for the session.
 */
export function createLocalLayoutStore(storage: Storage | undefined): LayoutStore {
  let cachedRaw: string | null = null;
  let cachedLayout: LibraryLayout | undefined;
  let sessionLayout: LibraryLayout | undefined;
  let storageFailed = false;

  function adopt(change: LayoutChange): void {
    sessionLayout = change.layout;
    storageFailed = !change.saved;
  }

  function readRaw(): string | null {
    try {
      return storage?.getItem(libraryLayoutStorageKey) ?? null;
    } catch {
      return null;
    }
  }

  return {
    read: () => {
      if (storageFailed) {
        return sessionLayout;
      }
      const raw = readRaw();
      if (raw !== cachedRaw) {
        cachedRaw = raw;
        cachedLayout = parseStoredLayout(raw);
      }
      return cachedLayout;
    },
    write: (layout) => {
      const change: LayoutChange = { layout, saved: saveLayout(storage, layout) };
      adopt(change);
      window.dispatchEvent(new CustomEvent<LayoutChange>(changeEventName, { detail: change }));
    },
    subscribe: (onChange) => listenForChanges(onChange, adopt),
  };
}

// Listens for writes in this tab (adopting their layout first) and in other tabs.
function listenForChanges(onChange: () => void, adopt: (change: LayoutChange) => void) {
  const onStorage = (event: StorageEvent) => {
    if (event.key === null || event.key === libraryLayoutStorageKey) {
      onChange();
    }
  };
  const onLocalChange = (event: Event) => {
    const change = layoutChangeOf(event);
    if (change !== undefined) {
      adopt(change);
    }
    onChange();
  };
  window.addEventListener(changeEventName, onLocalChange);
  window.addEventListener("storage", onStorage);
  return () => {
    window.removeEventListener(changeEventName, onLocalChange);
    window.removeEventListener("storage", onStorage);
  };
}

// Returns false when the layout could not be saved (no storage, quota, privacy mode).
function saveLayout(storage: Storage | undefined, layout: LibraryLayout | undefined): boolean {
  if (storage === undefined) {
    return false;
  }
  try {
    if (layout === undefined) {
      storage.removeItem(libraryLayoutStorageKey);
    } else {
      storage.setItem(libraryLayoutStorageKey, serializeLayout(layout));
    }
    return true;
  } catch {
    return false;
  }
}
