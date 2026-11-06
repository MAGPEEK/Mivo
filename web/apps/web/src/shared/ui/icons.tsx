import type { ReactNode } from "react";

// Line icons on a 24-unit grid, drawn in the current text colour. They are decorative: the
// control that shows one always carries a text label, so every icon is hidden from assistive
// technology.
const viewBox = "0 0 24 24";
const strokeWidth = 1.75;

/** Props of {@link IconFrame}. */
export interface IconFrameProps {
  readonly className?: string | undefined;
  readonly children: ReactNode;
}

/** The 24-unit SVG frame every line icon draws in; decorative, hidden from assistive technology. */
export function IconFrame({ className, children }: IconFrameProps) {
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

/** Film strip: a movie. */
export function FilmIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M7 4v16M17 4v16M3 9h4M3 15h4M17 9h4M17 15h4" />
    </IconFrame>
  );
}

/** Screen on a stand: a show. */
export function ScreenIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <rect x="3" y="5" width="18" height="12" rx="2" />
      <path d="M8 21h8M12 17v4" />
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

/** Push pin: keeps a library in the sidebar. */
export function PinIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M9 4h6l-1 6 4 3H6l4-3z" />
      <path d="M12 13v8" />
    </IconFrame>
  );
}

/** Padlock: a profile that needs its PIN. */
export function LockIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <rect x="5" y="11" width="14" height="10" rx="2" />
      <path d="M8 11V7a4 4 0 0 1 8 0v4" />
    </IconFrame>
  );
}

/** Chevron pointing up: moves an entry up. */
export function ChevronUpIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="m6 15 6-6 6 6" />
    </IconFrame>
  );
}

/** Chevron pointing down: moves an entry down. */
export function ChevronDownIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="m6 9 6 6 6-6" />
    </IconFrame>
  );
}

/** Circular arrow: restores defaults. */
export function ResetIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M4 12a8 8 0 1 0 2.3-5.6" />
      <path d="M4 4v4h4" />
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

/** Circle with an exclamation mark: something needs attention, e.g. the server is unreachable. */
export function AlertIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7.5v5.5" />
      <path d="M12 16.5h.01" />
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

/** Triangle pointing right: start playback. */
export function PlayIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M7 4.5v15a1 1 0 0 0 1.5.86l12-7.5a1 1 0 0 0 0-1.72l-12-7.5A1 1 0 0 0 7 4.5Z" />
    </IconFrame>
  );
}

/** Two bars: pause playback. */
export function PauseIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <rect x="6" y="4" width="4" height="16" rx="1" />
      <rect x="14" y="4" width="4" height="16" rx="1" />
    </IconFrame>
  );
}

/** Arrow pointing left: back to the previous page. */
export function BackIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M19 12H5" />
      <path d="m12 19-7-7 7-7" />
    </IconFrame>
  );
}

/** Chevron pointing left: scroll a row back. */
export function ChevronLeftIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="m15 18-6-6 6-6" />
    </IconFrame>
  );
}

/** Chevron pointing right: scroll a row forward. */
export function ChevronRightIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="m9 18 6-6-6-6" />
    </IconFrame>
  );
}

/** Speaker with sound waves: the sound is on. */
export function VolumeIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M11 5 6 9H3v6h3l5 4V5Z" />
      <path d="M15.5 8.5a5 5 0 0 1 0 7" />
      <path d="M18.5 5.5a9 9 0 0 1 0 13" />
    </IconFrame>
  );
}

/** Speaker with a cross: the sound is muted. */
export function MutedIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M11 5 6 9H3v6h3l5 4V5Z" />
      <path d="m16 9 6 6" />
      <path d="m22 9-6 6" />
    </IconFrame>
  );
}

/** Four corners pointing outwards: enter full screen. */
export function FullscreenIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M4 9V4h5" />
      <path d="M20 9V4h-5" />
      <path d="M4 15v5h5" />
      <path d="M20 15v5h-5" />
    </IconFrame>
  );
}

/** Four corners pointing inwards: leave full screen. */
export function ExitFullscreenIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M9 4v5H4" />
      <path d="M15 4v5h5" />
      <path d="M9 20v-5H4" />
      <path d="M15 20v-5h5" />
    </IconFrame>
  );
}

/** Circle with an "i": information, e.g. the stream-info overlay. */
export function InfoIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 11v5.5" />
      <path d="M12 7.5h.01" />
    </IconFrame>
  );
}

/** Cross: close a panel. */
export function CloseIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="m6 6 12 12" />
      <path d="M18 6 6 18" />
    </IconFrame>
  );
}

/** Gear: settings. */
export function SettingsIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1 7 17M17 7l2.1-2.1" />
    </IconFrame>
  );
}
