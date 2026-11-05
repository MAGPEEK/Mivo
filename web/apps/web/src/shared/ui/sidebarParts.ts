import type { ReactNode } from "react";

/** One library link of the sidebar. */
export interface SidebarLibraryLink {
  readonly id: string;
  readonly name: string;
  /** Decorative icon of the library kind. */
  readonly icon: ReactNode;
  /** Short state such as "Unavailable", shown and announced after the name. */
  readonly note?: string | undefined;
  /** Unpinned libraries appear only in "All libraries" (SH-12). Defaults to pinned. */
  readonly pinned?: boolean | undefined;
}

/** The latest layout change in words; `id` changes with every message, even an identical one. */
export interface LayoutAnnouncement {
  readonly text: string;
  readonly id: number;
}

/** Actions that rearrange the libraries of the sidebar (SH-12 ... SH-14). */
export interface SidebarLayoutControls {
  /** Moves a library one place up (-1) or down (1) in the full list. */
  readonly move: (id: string, offset: -1 | 1) => void;
  /** Moves a dragged library to the place of the one it was dropped on. */
  readonly moveTo: (id: string, targetId: string) => void;
  readonly togglePinned: (id: string) => void;
  /** Restores the server order with every library pinned. */
  readonly reset: () => void;
  /** Latest change in words, for the live region. */
  readonly announcement: LayoutAnnouncement;
}

/** Content of the libraries section; the texts come translated from the library feature. */
export type SidebarLibraries =
  | { readonly state: "loading"; readonly message: string }
  | { readonly state: "error"; readonly message: string }
  | {
      readonly state: "loaded";
      /** Every library in display order; pinned ones show in the sidebar. */
      readonly links: readonly SidebarLibraryLink[];
      /** Without controls the sidebar lists every link and offers no rearranging. */
      readonly layout?: SidebarLayoutControls | undefined;
    };

/**
 * Collapsing hides labels visually but keeps them in the accessibility tree, so the
 * navigation reads the same with a screen reader in both states.
 */
export function labelClass(collapsed: boolean, expandedClass: string): string {
  return collapsed ? "sr-only" : expandedClass;
}

/** Classes of a navigation entry; `aria-current` marks the current page. */
export const sidebarLinkClass =
  "h-10 group flex items-center gap-3 rounded-md px-2.5 text-sm font-medium text-text-muted hover:bg-surface-raised hover:text-text aria-[current=page]:bg-surface-raised aria-[current=page]:text-text";
