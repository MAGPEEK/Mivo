// Watch-state marks drawn over artwork (LB-42): progress bar, watched corner and the count of
// unwatched episodes. They are positioned inside a `relative` box around the artwork, e.g. the
// overlay of a `MediaCard`. Each one labels itself, because the artwork below is decorative.

import { IconFrame } from "./icons";

const percentMinimum = 0;
const percentMaximum = 100;

/** Props of {@link ProgressBar}. */
export interface ProgressBarProps {
  /** How much is done, 0 to 100; values outside are clamped. */
  readonly percent: number;
  /** Accessible name, e.g. "Watched 42 %". */
  readonly label: string;
  /** Extra classes for the track, e.g. its position; by default it spans the bottom edge. */
  readonly className?: string;
}

const bottomEdgeClass = "absolute inset-x-0 bottom-0";

/** Thin coral bar that shows how far an item was played; spans the bottom of the artwork. */
export function ProgressBar({ percent, label, className = bottomEdgeClass }: ProgressBarProps) {
  const clamped = Math.min(percentMaximum, Math.max(percentMinimum, percent));
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={percentMinimum}
      aria-valuemax={percentMaximum}
      aria-valuenow={clamped}
      className={`h-1 overflow-hidden bg-scrim/60 ${className}`}
    >
      <div className="h-full bg-accent" style={{ width: `${String(clamped)}%` }} />
    </div>
  );
}

/** Props of {@link WatchedBadge}. */
export interface WatchedBadgeProps {
  /** Accessible name, e.g. "Watched". */
  readonly label: string;
}

/** Check mark in the top right corner of the artwork of a watched item. */
export function WatchedBadge({ label }: WatchedBadgeProps) {
  return (
    <span
      role="img"
      aria-label={label}
      className="absolute top-0 right-0 grid size-6 place-items-center rounded-bl-md bg-scrim/75 text-direct-play"
    >
      <IconFrame className="size-4 shrink-0">
        <path d="M5 12.5l4.5 4.5L19 7.5" />
      </IconFrame>
    </span>
  );
}

/** Props of {@link CountBadge}. */
export interface CountBadgeProps {
  readonly count: number;
  /** Accessible name, e.g. "4 unwatched episodes". */
  readonly label: string;
}

/** Coral bubble with a number in the top right corner, e.g. the unwatched episodes of a show. */
export function CountBadge({ count, label }: CountBadgeProps) {
  return (
    <span
      role="img"
      aria-label={label}
      className="absolute top-1.5 right-1.5 grid h-5.5 min-w-5.5 place-items-center rounded-full bg-accent px-1.5 text-xs font-bold text-on-accent shadow-card"
    >
      {count}
    </span>
  );
}
