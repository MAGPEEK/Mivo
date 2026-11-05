import { IconFrame, type IconProps } from "./icons";

// Transport and queue icons of the player; the mini player and the music bar reuse them.
// The skip icons carry their step as a small number, drawn filled instead of stroked.
const stepTextProps = {
  fill: "currentColor",
  stroke: "none",
  fontSize: 7,
  fontWeight: 700,
  textAnchor: "middle",
} as const;

/** Bar and a triangle pointing left: the previous item. */
export function SkipPreviousIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M6 5v14" />
      <path d="M18 5 9 12l9 7V5Z" />
    </IconFrame>
  );
}

/** Triangle pointing right and a bar: the next item. */
export function SkipNextIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M18 5v14" />
      <path d="m6 5 9 7-9 7V5Z" />
    </IconFrame>
  );
}

/** Arrow turning back with "10": jumps 10 seconds back. */
export function SkipBackwardIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M4 12a8 8 0 1 0 2.3-5.6" />
      <path d="M4 4v4h4" />
      <text x="12.5" y="15" {...stepTextProps}>
        10
      </text>
    </IconFrame>
  );
}

/** Arrow turning forward with "30": jumps 30 seconds forward. */
export function SkipForwardIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M20 12a8 8 0 1 1-2.3-5.6" />
      <path d="M20 4v4h-4" />
      <text x="11.5" y="15" {...stepTextProps}>
        30
      </text>
    </IconFrame>
  );
}

/** Square: stops playback. */
export function StopIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <rect x="6" y="6" width="12" height="12" rx="1.5" />
    </IconFrame>
  );
}

/** Two arrows in a loop: repeats the queue. */
export function RepeatIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="m17 2 4 4-4 4" />
      <path d="M3 11V9a3 3 0 0 1 3-3h15" />
      <path d="m7 22-4-4 4-4" />
      <path d="M21 13v2a3 3 0 0 1-3 3H3" />
    </IconFrame>
  );
}

/** Loop with a "1": repeats the current item. */
export function RepeatOneIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="m17 2 4 4-4 4" />
      <path d="M3 11V9a3 3 0 0 1 3-3h15" />
      <path d="m7 22-4-4 4-4" />
      <path d="M21 13v2a3 3 0 0 1-3 3H3" />
      <path d="M11 10.5 12.5 9.5v5" />
    </IconFrame>
  );
}

/** Crossing arrows: shuffles the queue. */
export function ShuffleIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="M16 3h5v5" />
      <path d="M4 20 21 3" />
      <path d="M21 16v5h-5" />
      <path d="m15 15 6 6" />
      <path d="M4 4l5 5" />
    </IconFrame>
  );
}

/** Chevron pointing down: shrinks the player to the mini player. */
export function MinimiseIcon({ className }: IconProps) {
  return (
    <IconFrame className={className}>
      <path d="m6 9 6 6 6-6" />
    </IconFrame>
  );
}
