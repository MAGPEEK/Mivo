import { Link, useMatchRoute } from "@tanstack/react-router";
import { useRef, useState, type ReactNode, type Ref } from "react";
import { useTranslation } from "react-i18next";

import { HomeIcon, LibraryIcon, SearchIcon, SettingsIcon } from "./icons";
import { Sheet } from "./Sheet";
import { AllLibraries, LayoutLiveRegion } from "./SidebarLibraryLists";
import { useAllLibrariesSwitch } from "./useAllLibrariesSwitch";
import type { SidebarLibraries } from "./Sidebar";

/** Props of {@link BottomNav}. */
export interface BottomNavProps {
  /** The libraries the sheet lists, the same list the sidebar shows (SH-17). */
  readonly libraries: SidebarLibraries;
  /** Opens the search UI; today it focuses the field in the top bar (M5-50 replaces it). */
  readonly onSearch: () => void;
}

// Height of the bar without the safe-area inset. Layouts read it as `--bottom-nav-height`
// (see AppLayout), so the mini player and music bar can sit on top of it (M5-67, M5-70).
const barHeightClass = "h-16";

// min-h-11/min-w-11 = 44 px, the minimum touch target (WCAG 2.5.5, Apple HIG).
const tabClass =
  "flex h-full min-h-11 min-w-11 w-full flex-col items-center justify-center gap-0.5 rounded-md text-xs font-medium text-text-muted hover:text-text aria-[current=page]:text-accent";

const sheetActionClass =
  "mt-3 flex min-h-11 w-full items-center rounded-md px-2.5 text-sm font-medium text-accent hover:text-accent-hover";

const sheetLinkClass =
  "flex min-h-11 items-center gap-3 rounded-md px-2.5 text-sm font-medium text-text hover:bg-surface aria-[current=page]:text-accent";

/**
 * Navigation bar at the bottom of phone screens, which replaces the sidebar there: Home,
 * Libraries (a sheet with the library list), Search and Settings. It pads itself by the
 * bottom safe-area inset, so the home indicator of a phone never covers a tab.
 */
export function BottomNav({ libraries, onSearch }: BottomNavProps) {
  const { t } = useTranslation();
  const [sheetOpen, setSheetOpen] = useState(false);
  const closeSheet = () => {
    setSheetOpen(false);
  };
  return (
    // The sheet is a sibling of the bar: inside the fixed bar it would share its stacking
    // context and could end up below other fixed layers.
    <>
      <nav
        aria-label={t("sidebar.label")}
        className="fixed inset-x-0 bottom-0 z-30 border-t border-divider bg-surface pr-[env(safe-area-inset-right)] pb-[env(safe-area-inset-bottom)] pl-[env(safe-area-inset-left)]"
      >
        <BottomNavTabs
          librariesOpen={sheetOpen}
          onOpenLibraries={() => {
            setSheetOpen(true);
          }}
          onSearch={onSearch}
        />
      </nav>
      {sheetOpen && <LibrariesSheet libraries={libraries} onClose={closeSheet} />}
    </>
  );
}

interface BottomNavTabsProps {
  /** Whether the libraries sheet is open, for `aria-expanded`. */
  readonly librariesOpen: boolean;
  readonly onOpenLibraries: () => void;
  readonly onSearch: () => void;
}

function BottomNavTabs({ librariesOpen, onOpenLibraries, onSearch }: BottomNavTabsProps) {
  const { t } = useTranslation();
  const matchRoute = useMatchRoute();
  // The tab is a button, so the router cannot mark it; it is current on any library page.
  const onLibraryPage = matchRoute({ to: "/libraries/$libraryId" }) !== false;
  return (
    <ul className={`grid grid-cols-4 ${barHeightClass}`}>
      <li>
        <Link to="/" activeOptions={{ exact: true }} className={tabClass}>
          <HomeIcon className="size-5" />
          {t("sidebar.home")}
        </Link>
      </li>
      <li>
        <button
          type="button"
          aria-haspopup="dialog"
          aria-expanded={librariesOpen}
          aria-current={onLibraryPage ? "page" : undefined}
          onClick={onOpenLibraries}
          className={tabClass}
        >
          <LibraryIcon className="size-5" />
          {t("sidebar.libraries.heading")}
        </button>
      </li>
      <li>
        <button type="button" onClick={onSearch} className={tabClass}>
          <SearchIcon className="size-5" />
          {t("search.label")}
        </button>
      </li>
      <li>
        <Link to="/admin/manage/libraries" className={tabClass}>
          <SettingsIcon className="size-5" />
          {t("sidebar.settings")}
        </Link>
      </li>
    </ul>
  );
}

interface LibrariesSheetProps {
  readonly libraries: SidebarLibraries;
  readonly onClose: () => void;
}

// The sheet holds the same two views as the sidebar: the pinned libraries and, when the
// layout can be changed, "All libraries" with move and pin buttons sized for touch (SH-12).
function LibrariesSheet({ libraries, onClose }: LibrariesSheetProps) {
  const { t } = useTranslation();
  const doneRef = useRef<HTMLButtonElement>(null);
  const showAllRef = useRef<HTMLButtonElement>(null);
  const allSwitch = useAllLibrariesSwitch({
    collapsed: false,
    // Never collapsed: the sheet only exists while open.
    onExpand: () => undefined,
    doneRef,
    showAllRef,
  });
  const layout = libraries.state === "loaded" ? libraries.layout : undefined;
  const showAll = allSwitch.showAll && layout !== undefined && libraries.state === "loaded";
  return (
    <Sheet
      title={showAll ? t("sidebar.libraries.allLibraries") : t("sidebar.libraries.heading")}
      closeLabel={t("bottomNav.closeLibraries")}
      onClose={onClose}
    >
      {showAll ? (
        <>
          <button type="button" ref={doneRef} onClick={allSwitch.hide} className={sheetActionClass}>
            {t("sidebar.libraries.done")}
          </button>
          <AllLibraries
            links={libraries.links}
            layout={layout}
            variant="touch"
            onNavigate={onClose}
          />
        </>
      ) : (
        <>
          <LibraryList libraries={libraries} onNavigate={onClose} />
          <AllLibrariesEntry
            arrangeable={
              layout !== undefined && libraries.state === "loaded" && libraries.links.length > 0
            }
            onShowAll={allSwitch.show}
            showAllRef={showAllRef}
            onNavigate={onClose}
          />
        </>
      )}
      {layout !== undefined && <LayoutLiveRegion announcement={layout.announcement} />}
    </Sheet>
  );
}

interface AllLibrariesEntryProps {
  /** True when the full list can be shown in the sheet; else the entry opens library management. */
  readonly arrangeable: boolean;
  readonly onShowAll: () => void;
  readonly showAllRef: Ref<HTMLButtonElement>;
  readonly onNavigate: () => void;
}

function AllLibrariesEntry({
  arrangeable,
  onShowAll,
  showAllRef,
  onNavigate,
}: AllLibrariesEntryProps) {
  const { t } = useTranslation();
  if (arrangeable) {
    return (
      <button type="button" ref={showAllRef} onClick={onShowAll} className={sheetActionClass}>
        {t("bottomNav.allLibraries")}
      </button>
    );
  }
  // Without libraries (or layout controls) there is nothing to arrange; management can add one.
  return (
    <Link to="/admin/manage/libraries" onClick={onNavigate} className={sheetActionClass}>
      {t("bottomNav.allLibraries")}
    </Link>
  );
}

interface LibraryListProps {
  readonly libraries: SidebarLibraries;
  /** Closes the sheet: the opened page must not start covered. */
  readonly onNavigate: () => void;
}

function LibraryList({ libraries, onNavigate }: LibraryListProps): ReactNode {
  const { t } = useTranslation();
  const messageClass = "px-2.5 text-sm text-text-muted";
  if (libraries.state !== "loaded") {
    return <p className={messageClass}>{libraries.message}</p>;
  }
  if (libraries.links.length === 0) {
    return <p className={messageClass}>{t("sidebar.libraries.empty")}</p>;
  }
  return (
    <ul className="flex flex-col gap-1">
      {libraries.links
        .filter((library) => library.pinned !== false)
        .map((library) => (
          <li key={library.id}>
            <Link
              to="/libraries/$libraryId"
              params={{ libraryId: library.id }}
              onClick={onNavigate}
              className={sheetLinkClass}
            >
              <span className="contents">{library.icon}</span>
              <span className="min-w-0 flex-1 truncate">{library.name}</span>
              {library.note !== undefined && (
                <span className="shrink-0 text-xs text-text-muted">{library.note}</span>
              )}
            </Link>
          </li>
        ))}
    </ul>
  );
}
