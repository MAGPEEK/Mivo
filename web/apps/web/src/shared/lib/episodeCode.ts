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

/** Numbers of an episode as the API reports them; any of them may be unknown. */
export interface EpisodeNumbers {
  readonly season: number | undefined;
  readonly number: number | undefined;
  readonly numberEnd?: number | undefined;
}

/** Translation key and values of an episode code in the shared namespace. */
export type EpisodeCodeMessage =
  | {
      readonly key: "episodeCode.single";
      readonly values: { readonly season: number; readonly number: number };
    }
  | {
      readonly key: "episodeCode.range";
      readonly values: { readonly season: number; readonly first: number; readonly last: number };
    };

/**
 * The message of the `S1 · E3` (or `S1 · E1–E2`) label, or `undefined` while the season or
 * episode number is unknown. One formatter for detail pages and cards keeps the label alike.
 */
export function episodeCodeMessage(numbers: EpisodeNumbers): EpisodeCodeMessage | undefined {
  const range = episodeRange(numbers.number, numbers.numberEnd);
  if (numbers.season === undefined || range === undefined) {
    return undefined;
  }
  return isMultiEpisode(range)
    ? {
        key: "episodeCode.range",
        values: { season: numbers.season, first: range.first, last: range.last },
      }
    : { key: "episodeCode.single", values: { season: numbers.season, number: range.first } };
}
