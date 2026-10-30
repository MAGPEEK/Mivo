import { Outlet } from "@tanstack/react-router";
import { useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { useSidebarLibraries } from "../../features/library";
import { ServerVersion } from "../../features/system";
import { Sidebar, TopBar } from "../../shared/ui";
import { useNarrowScreen } from "./useNarrowScreen";
import { useSearchShortcut } from "./useSearchShortcut";
import { useSidebarOverlay } from "./useSidebarOverlay";
import { useSidebarState } from "./useSidebarState";

// Fixed rather than useId(): React's generated ids contain characters that are awkward in a
// URL fragment, and the layout exists once per page.
const mainContentId = "main-content";

/** Props of {@link AppLayout}. */
export interface AppLayoutProps {
  /** Page content. Defaults to the matched child route; the not-found view passes itself. */
  readonly children?: ReactNode;
}

/** Frame around every page: sidebar navigation, top bar with search and the page content. */
export function AppLayout({ children = <Outlet /> }: AppLayoutProps) {
  const { t } = useTranslation();
  const sidebar = useSidebarState(useNarrowScreen());
  const { collapsed, toggleCollapsed, expandAndFocusToggle, overlayOpen, closeOverlay, toggleRef } =
    sidebar;
  const navigationRef = useRef<HTMLElement>(null);
  useSidebarOverlay({ open: overlayOpen, close: closeOverlay, navigationRef, toggleRef });
  const searchInputRef = useSearchShortcut();
  const libraries = useSidebarLibraries();
  return (
    <div className="flex min-h-dvh">
      <a
        href={`#${mainContentId}`}
        className="sr-only rounded-md bg-accent px-4 py-2 font-medium text-on-accent focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-40"
      >
        {t("layout.skipToContent")}
      </a>
      <Sidebar
        collapsed={collapsed}
        onToggleCollapsed={toggleCollapsed}
        libraries={libraries}
        toggleRef={toggleRef}
        navigationRef={navigationRef}
        footer={(navigationId) =>
          collapsed ? (
            <ServerVersion compact onExpand={expandAndFocusToggle} sidebarId={navigationId} />
          ) : (
            <ServerVersion />
          )
        }
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar searchInputRef={searchInputRef} />
        {/* tabIndex -1 lets the skip link move keyboard focus into the content. */}
        <main
          id={mainContentId}
          tabIndex={-1}
          className="flex-1 px-6 py-8 focus-visible:outline-none"
        >
          {children}
        </main>
      </div>
    </div>
  );
}
