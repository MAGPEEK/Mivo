import { Outlet } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { ServerVersion } from "../../features/system";
import { Sidebar, TopBar } from "../../shared/ui";
import { useSearchShortcut } from "./useSearchShortcut";
import { useSidebarState } from "./useSidebarState";

// Fixed rather than useId(): React's generated ids contain characters that are awkward in a
// URL fragment, and the layout exists once per page.
const mainContentId = "main-content";

/** Frame around every page: sidebar navigation, top bar with search and the page content. */
export function AppLayout() {
  const { t } = useTranslation();
  const { collapsed, toggleCollapsed, expandAndFocusToggle, toggleRef } = useSidebarState();
  const searchInputRef = useSearchShortcut();
  return (
    <div className="flex min-h-dvh">
      <a
        href={`#${mainContentId}`}
        className="sr-only rounded-md bg-accent px-4 py-2 font-medium text-on-accent focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-20"
      >
        {t("layout.skipToContent")}
      </a>
      <Sidebar
        collapsed={collapsed}
        onToggleCollapsed={toggleCollapsed}
        toggleRef={toggleRef}
        footer={
          collapsed ? <ServerVersion compact onExpand={expandAndFocusToggle} /> : <ServerVersion />
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
          <Outlet />
        </main>
      </div>
    </div>
  );
}
