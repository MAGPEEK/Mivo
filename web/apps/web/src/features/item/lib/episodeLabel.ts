/** The episode numbers one file covers; `first === last` for a single episode. */
export interface EpisodeRange {
  readonly first: number;
  readonly last: number;
}

/**
 * The episodes of a file from its number and the optional last number (S01E01-E02). A missing
 * number gives `undefined` (an unnumbered episode); an end that does not exceed the start is
 * ignored instead of shown as a backwards range.
 */
export function episodeRange(
  number: number | undefined,
  numberEnd: number | undefined,
): EpisodeRange | undefined {
  if (number === undefined) {
    return undefined;
  }
  const last = numberEnd !== undefined && numberEnd > number ? numberEnd : number;
  return { first: number, last };
}

/** Whether the range covers more than one episode. */
export function isMultiEpisode(range: EpisodeRange): boolean {
  return range.last > range.first;
}
