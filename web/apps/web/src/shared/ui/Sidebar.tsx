import { productName } from "@mivo/ui-tokens";
import { Link } from "@tanstack/react-router";
import { useId, useRef, type ReactNode, type Ref } from "react";
import { useTranslation } from "react-i18next";

import { HomeIcon, SettingsIcon, SidebarIcon } from "./icons";
import { labelClass, sidebarLinkClass, type SidebarLibraries } from "./sidebarParts";
import {
  AllLibraries,
  LayoutLiveRegion,
  LibraryLink,
  PinnedLibraries,
} from "./SidebarLibraryLists";
import { useAllLibrariesSwitch } from "./useAllLibrariesSwitch";

export type {
  LayoutAnnouncement,
  SidebarLayoutControls,
  SidebarLibraries,
  SidebarLibraryLink,
} from "./sidebarParts";

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
        <LibrariesSection
          collapsed={collapsed}
          libraries={libraries}
          onExpand={onToggleCollapsed}
        />
        <SettingsLink collapsed={collapsed} />
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
          className={`${sidebarLinkClass} ${collapsed ? "justify-center" : ""}`}
        >
          <HomeIcon className="size-5 shrink-0 group-aria-[current=page]:text-accent" />
          <span className={labelClass(collapsed, "truncate")}>{t("sidebar.home")}</span>
        </Link>
      </li>
    </ul>
  );
}

// Last entry of the navigation (SH-18). Until the admin area exists (milestone 5/7) it opens
// the library management, the only settings page.
function SettingsLink({ collapsed }: CollapsibleProps) {
  const { t } = useTranslation();
  return (
    <ul className="px-3 pb-2">
      <li>
        <Link
          to="/admin/manage/libraries"
          className={`${sidebarLinkClass} ${collapsed ? "justify-center" : ""}`}
        >
          <SettingsIcon className="size-5 shrink-0 group-aria-[current=page]:text-accent" />
          <span className={labelClass(collapsed, "truncate")}>{t("sidebar.settings")}</span>
        </Link>
      </li>
    </ul>
  );
}

interface LibrariesSectionProps extends CollapsibleProps {
  readonly libraries: SidebarLibraries;
  /** Expands the sidebar, for "All libraries" pressed while collapsed. */
  readonly onExpand: () => void;
}

// The section scrolls on its own, so a long list never pushes the footer off screen.
function LibrariesSection({ collapsed, libraries, onExpand }: LibrariesSectionProps) {
  const { t } = useTranslation();
  const headingId = useId();
  const doneRef = useRef<HTMLButtonElement>(null);
  const showAllRef = useRef<HTMLButtonElement>(null);
  const allSwitch = useAllLibrariesSwitch({ collapsed, onExpand, doneRef, showAllRef });
  const layout = libraries.state === "loaded" ? libraries.layout : undefined;
  return (
    <section aria-labelledby={headingId} className="mt-6 min-h-0 overflow-y-auto px-3 pb-3">
      <div className="flex items-center justify-between gap-2 px-2.5">
        <h2
          id={headingId}
          className={labelClass(
            collapsed,
            "text-xs font-semibold tracking-wide text-text-muted uppercase",
          )}
        >
          {allSwitch.showAll && layout !== undefined
            ? t("sidebar.libraries.allLibraries")
            : t("sidebar.libraries.heading")}
        </h2>
        {allSwitch.showAll && layout !== undefined && (
          <button
            type="button"
            ref={doneRef}
            onClick={allSwitch.hide}
            className="rounded-md px-2 py-0.5 text-xs font-medium text-text-muted hover:bg-surface-raised hover:text-text"
          >
            {t("sidebar.libraries.done")}
          </button>
        )}
      </div>
      {allSwitch.showAll && libraries.state === "loaded" && libraries.layout !== undefined ? (
        <AllLibraries links={libraries.links} layout={libraries.layout} />
      ) : (
        <LibrariesContent
          collapsed={collapsed}
          libraries={libraries}
          onShowAll={allSwitch.show}
          showAllRef={showAllRef}
        />
      )}
      {layout !== undefined && <LayoutLiveRegion announcement={layout.announcement} />}
    </section>
  );
}

interface LibrariesContentProps extends CollapsibleProps {
  readonly libraries: SidebarLibraries;
  readonly onShowAll: () => void;
  readonly showAllRef: Ref<HTMLButtonElement>;
}

function LibrariesContent({ collapsed, libraries, onShowAll, showAllRef }: LibrariesContentProps) {
  const { t } = useTranslation();
  const messageClass = labelClass(collapsed, "mt-2 px-2.5 text-sm text-text-muted");
  if (libraries.state !== "loaded") {
    return <p className={messageClass}>{libraries.message}</p>;
  }
  if (libraries.links.length === 0) {
    return (
      <>
        <p className={messageClass}>{t("sidebar.libraries.empty")}</p>
        <Link
          to="/admin/manage/libraries"
          search={{ add: true }}
          className={labelClass(
            collapsed,
            "mt-1 block rounded-md px-2.5 py-1 text-sm font-medium text-accent hover:text-accent-hover",
          )}
        >
          {t("sidebar.libraries.add")}
        </Link>
      </>
    );
  }
  if (libraries.layout !== undefined) {
    return (
      <PinnedLibraries
        collapsed={collapsed}
        links={libraries.links.filter((library) => library.pinned !== false)}
        layout={libraries.layout}
        onShowAll={onShowAll}
        showAllRef={showAllRef}
      />
    );
  }
  return (
    <ul className="mt-2 flex flex-col gap-1">
      {libraries.links.map((library) => (
        <li key={library.id} className="flex">
          <LibraryLink collapsed={collapsed} library={library} />
        </li>
      ))}
    </ul>
  );
}
