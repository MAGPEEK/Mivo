import { Outlet } from "@tanstack/react-router";
import { useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { useSidebarLibraries } from "../../features/library";
import { ProfileMenu } from "../../features/profiles";
import { useArrangedLibraries } from "../../features/sidebar-layout";
import { ServerVersion } from "../../features/system";
import { BottomNav, Sidebar, TopBar } from "../../shared/ui";
import { useNarrowScreen } from "./useNarrowScreen";
import { usePhoneScreen } from "./usePhoneScreen";
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
  const libraries = useArrangedLibraries(useSidebarLibraries());
  const phone = usePhoneScreen();
  const shell = shellClasses(phone);
  const focusSearch = () => {
    searchInputRef.current?.focus();
  };
  return (
    <div className={shell.root} data-bottom-nav={phone ? "" : undefined}>
      <a
        href={`#${mainContentId}`}
        className="sr-only rounded-md bg-accent px-4 py-2 font-medium text-on-accent focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-40"
      >
        {t("layout.skipToContent")}
      </a>
      {!phone && (
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
      )}
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar searchInputRef={searchInputRef} actions={<ProfileMenu />} />
        {/* tabIndex -1 lets the skip link move keyboard focus into the content. */}
        <main id={mainContentId} tabIndex={-1} className={shell.main}>
          {children}
        </main>
      </div>
      {phone && <BottomNav libraries={libraries} onSearch={focusSearch} />}
    </div>
  );
}

interface ShellClasses {
  readonly root: string;
  readonly main: string;
}

// On phones the bottom navigation (SH-17) replaces the sidebar. Its height, including the
// safe-area inset, is published as --bottom-nav-height so the content, and later the mini
// player and music bar (M5-67, M5-70), stay clear of it. The page opts into viewport-fit=cover
// (index.html), so every shell keeps clear of the left and right insets: a phone in landscape is
// wide enough for the sidebar shell, and its notch would otherwise cover the sidebar.
const inlineSafeArea = "pr-[env(safe-area-inset-right)] pl-[env(safe-area-inset-left)]";

function shellClasses(phone: boolean): ShellClasses {
  if (!phone) {
    return {
      root: `flex min-h-dvh ${inlineSafeArea}`,
      main: "flex-1 px-6 py-8 focus-visible:outline-none",
    };
  }
  return {
    root: `flex min-h-dvh ${inlineSafeArea} [--bottom-nav-height:calc(var(--spacing)*16+env(safe-area-inset-bottom))]`,
    main: "flex-1 px-4 pt-6 pb-[calc(var(--bottom-nav-height)+var(--spacing)*6)] focus-visible:outline-none",
  };
}
