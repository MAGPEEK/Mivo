// Seek previews (ADR 0030 §8): sprite sheets of columns × rows tiles, one tile per interval.
// The maths lives here; the app only draws the rectangle it is given. Sheet URLs are opaque
// (ADR 0030 Notes M4-02b): the URL of a sheet is only ever an entry of `sheetUrls`.

/**
 * The fields of the API's `TrickplayManifest` the maths needs. `sheetCount` is not among them:
 * the sheets that can be shown are the entries of `sheetUrls`, whatever the count claims.
 */
export interface TrickplayLayout {
  readonly tileWidth: number;
  readonly tileHeight: number;
  readonly columns: number;
  readonly rows: number;
  /** Seconds of the file between two tiles; tile 0 shows the start. */
  readonly intervalSeconds: number;
  /** One opaque URI reference per sheet, sheet 0 first. */
  readonly sheetUrls: readonly string[];
}

/** Where the preview of one file position lies. */
export interface TrickplayTile {
  /** Index of the sheet in `sheetUrls`. */
  readonly sheet: number;
  /** `sheetUrls[sheet]`, unchanged; resolve it against the URL of the manifest request. */
  readonly sheetUrl: string;
  readonly column: number;
  readonly row: number;
  /** Left edge of the tile on the sheet, in pixels. */
  readonly x: number;
  /** Top edge of the tile on the sheet, in pixels. */
  readonly y: number;
  readonly width: number;
  readonly height: number;
}

function isCount(value: number): boolean {
  return Number.isInteger(value) && value >= 1;
}

function isUsable(layout: TrickplayLayout): boolean {
  return (
    isCount(layout.columns) &&
    isCount(layout.rows) &&
    isCount(layout.tileWidth) &&
    isCount(layout.tileHeight) &&
    Number.isFinite(layout.intervalSeconds) &&
    layout.intervalSeconds > 0
  );
}

/**
 * The tile that previews file position `seconds`. Positions before the start show tile 0 and
 * positions past the last sheet its last tile; `NaN` counts as the start. The sheet index is
 * clamped to the length of `sheetUrls`, so a manifest whose numbers promise more sheets than
 * it lists never produces a URL. `undefined` means no preview: `sheetUrls` is empty or the
 * layout is unusable (the contract sets no minimum length, BACKLOG "m4-02b review minors").
 */
export function trickplayTile(layout: TrickplayLayout, seconds: number): TrickplayTile | undefined {
  const sheetCount = layout.sheetUrls.length;
  if (sheetCount === 0 || !isUsable(layout)) {
    return undefined;
  }
  const tilesPerSheet = layout.columns * layout.rows;
  const lastTile = sheetCount * tilesPerSheet - 1;
  const position = Number.isNaN(seconds) || seconds < 0 ? 0 : seconds;
  const tile = Math.min(Math.floor(position / layout.intervalSeconds), lastTile);
  const sheet = Math.floor(tile / tilesPerSheet);
  const sheetUrl = layout.sheetUrls[sheet];
  if (sheetUrl === undefined || sheetUrl === "") {
    return undefined;
  }
  const tileOnSheet = tile % tilesPerSheet;
  const column = tileOnSheet % layout.columns;
  const row = Math.floor(tileOnSheet / layout.columns);
  return {
    sheet,
    sheetUrl,
    column,
    row,
    x: column * layout.tileWidth,
    y: row * layout.tileHeight,
    width: layout.tileWidth,
    height: layout.tileHeight,
  };
}

/**
 * How often the manifest is asked for again after a 404. The answer does not tell "not
 * generated yet" from "will never exist" (BACKLOG "m4-02b review minors"), so polling must end.
 */
export const trickplayPollMaxAttempts = 6;

/** Wait before the first repeat, in milliseconds; every further wait doubles. */
export const trickplayPollIntervalMilliseconds = 5000;

/** Longest wait between two requests, in milliseconds. */
export const trickplayPollMaxIntervalMilliseconds = 30_000;

/**
 * How long to wait before asking for the manifest again, after `failedAttempts` requests that
 * answered 404 (1 after the first). `undefined` ends polling: the cap of
 * {@link trickplayPollMaxAttempts} is reached, and the player goes on without previews.
 */
export function trickplayPollDelay(failedAttempts: number): number | undefined {
  if (!Number.isFinite(failedAttempts) || failedAttempts >= trickplayPollMaxAttempts) {
    return undefined;
  }
  const repeats = Math.max(0, Math.floor(failedAttempts) - 1);
  return Math.min(
    trickplayPollIntervalMilliseconds * 2 ** repeats,
    trickplayPollMaxIntervalMilliseconds,
  );
}
