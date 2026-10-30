// Time ranges as plain data, so player-core can reason about what the media element has
// without reading DOM objects.

/** A closed range of seconds. */
export interface TimeRange {
  readonly start: number;
  readonly end: number;
}

/** Answers like the DOM's `TimeRanges` (`HTMLMediaElement.seekable` and `.buffered`). */
export interface TimeRangesLike {
  readonly length: number;
  start(index: number): number;
  end(index: number): number;
}

/** Copies a `TimeRanges`-like object into plain ranges, in its order. */
export function readTimeRanges(ranges: TimeRangesLike): TimeRange[] {
  return Array.from({ length: ranges.length }, (_, index) => ({
    start: ranges.start(index),
    end: ranges.end(index),
  }));
}

/** Whether `time` lies inside `range`, both ends included. */
export function rangeContains(range: TimeRange, time: number): boolean {
  return range.start <= time && time <= range.end;
}
