import type { ReactNode } from "react";

/** Props of {@link EmptyState}. */
export interface EmptyStateProps {
  /** Short statement of what is missing; rendered as a level-2 heading below the page title. */
  readonly title: string;
  /** What the user can do about it. */
  readonly description: string;
  /** Optional decorative icon above the title. */
  readonly icon?: ReactNode;
}

/** Placeholder for an area that has no content yet. */
export function EmptyState({ title, description, icon }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center rounded-lg border border-dashed border-divider bg-surface px-6 py-12 text-center">
      {icon !== undefined && <div className="mb-4 text-accent">{icon}</div>}
      <h2 className="text-lg font-semibold text-text">{title}</h2>
      <p className="mt-2 max-w-md text-sm text-text-muted">{description}</p>
    </div>
  );
}
