import type { ReactNode } from "react";

/** Props of {@link Badge}. */
export interface BadgeProps {
  readonly children: ReactNode;
  /** Optional decorative icon before the text. */
  readonly icon?: ReactNode;
}

/**
 * Small inline label for technical facts (`MKV · H.264 · AAC`) or a state such as
 * "Unavailable". Text, not a control: it never takes focus.
 */
export function Badge({ children, icon }: BadgeProps) {
  return (
    <span className="inline-flex max-w-full items-center gap-1 rounded-sm bg-surface-raised px-1.5 py-0.5 text-xs font-medium text-text-muted">
      {icon}
      <span className="truncate">{children}</span>
    </span>
  );
}
