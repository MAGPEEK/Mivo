import { productName } from "@mivo/ui-tokens";
import { Link } from "@tanstack/react-router";
import { useId, type ReactNode, type Ref } from "react";
import { useTranslation } from "react-i18next";

import { AlertIcon, HomeIcon, SidebarIcon } from "./icons";

/** One library link of the sidebar. */
export interface SidebarLibraryLink {
  readonly id: string;
  readonly name: string;
  /** Decorative icon of the library kind. */
  readonly icon: ReactNode;
  /** Short state such as "Unavailable", shown and announced after the name. */
  readonly note?: string | undefined;
}

/** Content of the libraries section; the texts come translated from the library feature. */
export type SidebarLibraries =
  | { readonly state: "loading"; readonly message: string }
  | { readonly state: "error"; readonly message: string }
  | {
      readonly state: "loaded";
      readonly links: readonly SidebarLibraryLink[];
      /** Explains how to add libraries; shown when `links` is empty. */
      readonly emptyHint: string;
    };

/** Props of {@link Sidebar}. */
export interface SidebarProps {
  /** Collapsed shows icons only; every label stays available to screen readers. */
  readonly collapsed: boolean;
  readonly onToggleCollapsed: () => void;
  /** The libraries section. */
  readonly libraries: SidebarLibraries;
  /** Receives the toggle button, e.g. to move focus to it. */
  readonly toggleRef?: Ref<HTMLButtonElement>;
  /**
   * Receives the navigation element, e.g. to close the sidebar where it covers the page when a
   * press or keyboard focus lands outside it.
   */
  readonly navigationRef?: Ref<HTMLElement>;
  /**
   * Renders the bottom area, e.g. the server version. Unlike the labels it stays visible while
   * collapsed, so it can show a warning there; the caller adapts it to the narrow width. Gets
   * the id of the navigation, so a control in it can name the sidebar in `aria-controls`.
   */
  readonly footer?: (navigationId: string) => ReactNode;
}

// Collapsing hides labels visually but keeps them in the accessibility tree, so the
// navigation reads the same with a screen reader in both states.
function labelClass(collapsed: boolean, expandedClass: string): string {
  return collapsed ? "sr-only" : expandedClass;
}

const linkClass =
  "group flex h-10 items-center gap-3 rounded-md px-2.5 text-sm font-medium text-text-muted hover:bg-surface-raised hover:text-text aria-[current=page]:bg-surface-raised aria-[current=page]:text-text";

/** Main navigation on the left: brand, page links, libraries and a footer slot. */
export function Sidebar({
  collapsed,
  onToggleCollapsed,
  libraries,
  toggleRef,
  navigationRef,
  footer,
}: SidebarProps) {
  const { t } = useTranslation();
  const navigationId = useId();
  return (
    // The wrapper reserves the space in the page flow. Below the md breakpoint it reserves only
    // the rail, and the expanded sidebar lies over the content instead of squeezing it: on a
    // 320 px phone the full sidebar would leave the page too narrow to read.
    <div className={`shrink-0 ${collapsed ? "w-16" : "w-16 md:w-60"}`}>
      <nav
        ref={navigationRef}
        id={navigationId}
        aria-label={t("sidebar.label")}
        className={`sticky top-0 z-30 flex h-dvh flex-col border-r border-divider bg-surface transition-[width] motion-reduce:transition-none ${collapsed ? "w-16" : "w-60 max-md:shadow-overlay"}`}
      >
        <div className="flex h-14 shrink-0 items-center gap-2 px-3">
          <button
            type="button"
            ref={toggleRef}
            aria-expanded={!collapsed}
            aria-controls={navigationId}
            aria-label={t("sidebar.toggle")}
            onClick={onToggleCollapsed}
            className="grid size-10 shrink-0 place-items-center rounded-md text-text-muted hover:bg-surface-raised hover:text-text"
          >
            <SidebarIcon />
          </button>
          <span className={labelClass(collapsed, "truncate text-lg font-semibold text-text")}>
            {productName}
          </span>
        </div>
        <SidebarLinks collapsed={collapsed} />
        <LibrariesSection collapsed={collapsed} libraries={libraries} />
        {footer !== undefined && (
          <div className={`mt-auto border-t border-divider py-3 ${collapsed ? "px-2" : "px-4"}`}>
            {footer(navigationId)}
          </div>
        )}
      </nav>
    </div>
  );
}

interface CollapsibleProps {
  readonly collapsed: boolean;
}

function SidebarLinks({ collapsed }: CollapsibleProps) {
  const { t } = useTranslation();
  return (
    <ul className="flex flex-col gap-1 px-3 pt-2">
      <li>
        <Link
          to="/"
          activeOptions={{ exact: true }}
          className={`${linkClass} ${collapsed ? "justify-center" : ""}`}
        >
          <HomeIcon className="size-5 shrink-0 group-aria-[current=page]:text-accent" />
          <span className={labelClass(collapsed, "truncate")}>{t("sidebar.home")}</span>
        </Link>
      </li>
    </ul>
  );
}

interface LibrariesSectionProps extends CollapsibleProps {
  readonly libraries: SidebarLibraries;
}

// The section scrolls on its own, so a long list never pushes the footer off screen.
function LibrariesSection({ collapsed, libraries }: LibrariesSectionProps) {
  const { t } = useTranslation();
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="mt-6 min-h-0 overflow-y-auto px-3 pb-3">
      <h2
        id={headingId}
        className={labelClass(
          collapsed,
          "px-2.5 text-xs font-semibold tracking-wide text-text-muted uppercase",
        )}
      >
        {t("sidebar.libraries.heading")}
      </h2>
      <LibrariesContent collapsed={collapsed} libraries={libraries} />
    </section>
  );
}

function LibrariesContent({ collapsed, libraries }: LibrariesSectionProps) {
  const { t } = useTranslation();
  const messageClass = labelClass(collapsed, "mt-2 px-2.5 text-sm text-text-muted");
  if (libraries.state !== "loaded") {
    return <p className={messageClass}>{libraries.message}</p>;
  }
  if (libraries.links.length === 0) {
    return (
      <>
        <p className={messageClass}>{t("sidebar.libraries.empty")}</p>
        <p className={labelClass(collapsed, "mt-1 px-2.5 text-xs text-text-muted")}>
          {libraries.emptyHint}
        </p>
      </>
    );
  }
  return (
    <ul className="mt-2 flex flex-col gap-1">
      {libraries.links.map((library) => (
        <li key={library.id}>
          <LibraryLink collapsed={collapsed} library={library} />
        </li>
      ))}
    </ul>
  );
}

interface LibraryLinkProps extends CollapsibleProps {
  readonly library: SidebarLibraryLink;
}

function LibraryLink({ collapsed, library }: LibraryLinkProps) {
  return (
    <Link
      to="/libraries/$libraryId"
      params={{ libraryId: library.id }}
      className={`${linkClass} relative ${collapsed ? "justify-center" : ""}`}
    >
      {/* The icon draws in currentColor, so the wrapper's colour marks the current library. */}
      <span className="contents group-aria-[current=page]:text-accent">{library.icon}</span>
      <span className={labelClass(collapsed, "min-w-0 flex-1 truncate")}>{library.name}</span>
      {library.note !== undefined && (
        <>
          {/* Visible in both states: the only hint at an unavailable library while collapsed. */}
          <AlertIcon
            className={
              collapsed
                ? "absolute top-1 right-1 size-3.5 text-accent"
                : "size-4 shrink-0 text-accent"
            }
          />
          <span className="sr-only">{`, ${library.note}`}</span>
        </>
      )}
    </Link>
  );
}
