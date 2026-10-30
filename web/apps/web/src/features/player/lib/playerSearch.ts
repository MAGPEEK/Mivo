/** Validated search parameters of the player route `/items/$itemId/play`. */
export interface PlayerSearch {
  /** File of the item to play; without it the player picks the item's primary file. */
  readonly fileId?: string | undefined;
  /** Start position in seconds (`t` keeps shared links short); without it playback starts at 0. */
  readonly t?: number | undefined;
}

function isFileId(value: unknown): value is string {
  return typeof value === "string" && value !== "";
}

function isStartPosition(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

/**
 * Validates the parsed search parameters of the player route. Invalid values are dropped
 * instead of thrown, so a hand-edited or outdated link still opens the player.
 *
 * Both keys are always returned: the router lays the result over the raw search, so a dropped
 * value must be overwritten with `undefined`. URLs omit `undefined` values.
 */
export function validatePlayerSearch(search: Readonly<Record<string, unknown>>): PlayerSearch {
  const fileId = search["fileId"];
  const startPosition = search["t"];
  return {
    fileId: isFileId(fileId) ? fileId : undefined,
    t: isStartPosition(startPosition) ? startPosition : undefined,
  };
}
