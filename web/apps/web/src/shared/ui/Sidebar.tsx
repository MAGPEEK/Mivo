import { productName } from "@mivo/ui-tokens";
import { Link } from "@tanstack/react-router";
import { useId, type ReactNode, type Ref } from "react";
import { useTranslation } from "react-i18next";

import { HomeIcon, SidebarIcon } from "./icons";

/** Props of {@link Sidebar}. */
export interface SidebarProps {
  /** Collapsed shows icons only; every label stays available to screen readers. */
  readonly collapsed: boolean;
  readonly onToggleCollapsed: () => void;
  /** Receives the toggle button, e.g. to move focus to it. */
  readonly toggleRef?: Ref<HTMLButtonElement>;
  /**
   * Bottom area, e.g. the server version. Unlike the labels it stays visible while collapsed,
   * so it can show a warning there; the caller adapts it to the narrow width.
   */
  readonly footer?: ReactNode;
}

// Collapsing hides labels visually but keeps them in the accessibility tree, so the
// navigation reads the same with a screen reader in both states.
function labelClass(collapsed: boolean, expandedClass: string): string {
  return collapsed ? "sr-only" : expandedClass;
}

/** Main navigation on the left: brand, page links, libraries and a footer slot. */
export function Sidebar({ collapsed, onToggleCollapsed, toggleRef, footer }: SidebarProps) {
  const { t } = useTranslation();
  const navigationId = useId();
  return (
    <nav
      id={navigationId}
      aria-label={t("sidebar.label")}
      className={`sticky top-0 flex h-dvh shrink-0 flex-col border-r border-divider bg-surface transition-[width] motion-reduce:transition-none ${collapsed ? "w-16" : "w-60"}`}
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
      <LibrariesSection collapsed={collapsed} />
      {footer !== undefined && (
        <div className={`mt-auto border-t border-divider py-3 ${collapsed ? "px-2" : "px-4"}`}>
          {footer}
        </div>
      )}
    </nav>
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
          className={`group flex h-10 items-center gap-3 rounded-md px-2.5 text-sm font-medium text-text-muted hover:bg-surface-raised hover:text-text aria-[current=page]:bg-surface-raised aria-[current=page]:text-text ${collapsed ? "justify-center" : ""}`}
        >
          <HomeIcon className="size-5 shrink-0 group-aria-[current=page]:text-accent" />
          <span className={labelClass(collapsed, "truncate")}>{t("sidebar.home")}</span>
        </Link>
      </li>
    </ul>
  );
}

function LibrariesSection({ collapsed }: CollapsibleProps) {
  const { t } = useTranslation();
  return (
    <div className="mt-6 px-3">
      <h2
        className={labelClass(
          collapsed,
          "px-2.5 text-xs font-semibold tracking-wide text-text-muted uppercase",
        )}
      >
        {t("sidebar.libraries.heading")}
      </h2>
      <p className={labelClass(collapsed, "mt-2 px-2.5 text-sm text-text-muted")}>
        {t("sidebar.libraries.empty")}
      </p>
    </div>
  );
}
