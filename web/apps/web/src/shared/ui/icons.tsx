import type { ReactNode } from "react";

// Line icons on a 24-unit grid, drawn in the current text colour. They are decorative: the
// control that shows one always carries a text label, so every icon is hidden from assistive
// technology.
const viewBox = "0 0 24 24";
const strokeWidth = 1.75;

interface IconFrameProps {
  readonly className?: string | undefined;
  readonly children: ReactNode;
}

function IconFrame({ className, children }: IconFrameProps) {
  return (
    <svg
      viewBox={viewBox}
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      className={className ?? "size-5 shrink-0"}
    >
      {children}
    </svg>
  );
}

/** Props of every icon. */
export interface IconProps {
  /** Overrides the default size classes. */
  readonly className?: string;
}

/** House: the home page. */
export function HomeIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M3 10.5 12 3l9 7.5" />
      <path d="M5 9v11a1 1 0 0 0 1 1h4v-6h4v6h4a1 1 0 0 0 1-1V9" />
    </IconFrame>
  );
}

/** Stacked panels: media libraries. */
export function LibraryIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <rect x="3" y="4" width="18" height="6" rx="1.5" />
      <rect x="3" y="14" width="18" height="6" rx="1.5" />
    </IconFrame>
  );
}

/** Window with a left panel: shows or hides the sidebar labels. */
export function SidebarIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M9 4v16" />
    </IconFrame>
  );
}

/** Magnifying glass: search. */
export function SearchIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.5-3.5" />
    </IconFrame>
  );
}
