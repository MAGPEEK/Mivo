/** Props of {@link Skeleton}. */
export interface SkeletonProps {
  /** What is loading, read by screen readers in place of the placeholder shape. */
  readonly label: string;
  /** Size and shape of the placeholder, e.g. `h-3 w-24`. */
  readonly className?: string;
}

/**
 * Placeholder shape for content that is still loading. The shape is decorative; the label is
 * what assistive technology reads. Put it inside a live region (e.g. `role="status"`) when
 * the change to real content should be announced.
 */
export function Skeleton({ label, className = "h-4 w-full" }: SkeletonProps) {
  return (
    <span className="block">
      <span
        aria-hidden="true"
        className={`block animate-pulse rounded-sm bg-surface-raised motion-reduce:animate-none ${className}`}
      />
      <span className="sr-only">{label}</span>
    </span>
  );
}
