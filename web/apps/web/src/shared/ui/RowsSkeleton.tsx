import { cardWidths } from "../lib/rowScroll";
import { Skeleton } from "./Skeleton";

/** Props of {@link RowsSkeleton}. */
export interface RowsSkeletonProps {
  /** What screen readers hear once, e.g. "Loading rows". */
  readonly label: string;
}

const skeletonRowCount = 2;
const skeletonCardCount = 6;

/** Placeholder rows of poster cards while server rows load (Home, library Overview). */
export function RowsSkeleton({ label }: RowsSkeletonProps) {
  return (
    <div role="status" className="relative flex flex-col gap-8">
      <span className="sr-only">{label}</span>
      {Array.from({ length: skeletonRowCount }, (_, row) => (
        <div key={row} aria-hidden="true" className="flex flex-col gap-3">
          <Skeleton label="" className="h-5 w-48" />
          {/* Relative, so the clip also holds the absolutely positioned screen reader labels. */}
          <div className="relative flex gap-4 overflow-hidden">
            {Array.from({ length: skeletonCardCount }, (_, card) => (
              <div key={card} className="shrink-0" style={{ width: cardWidths.poster }}>
                <Skeleton label="" className="aspect-2/3 w-full rounded-md" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
