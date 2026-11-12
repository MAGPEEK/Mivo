// The selected profile's watch state of an item as cards and detail pages show it (ADR 0032
// section 8, LB-42). Pure: the features turn it into badges, labels and requests.

import type { ViewerState } from "@mivo/api-client";

const percentScale = 100;

// Kinds that can be marked watched in the web app: a show or season applies to its episodes.
// Audiobooks keep their position through the player; extras, music and photos have no state.
const markableKinds: ReadonlySet<string> = new Set(["movie", "episode", "show", "season"]);
const containerKinds: ReadonlySet<string> = new Set(["show", "season"]);

/** The part of an item the watch state is derived from; summaries and details both fit. */
export interface WatchSource {
  readonly kind: string;
  /** Absent while the item has no state, or when the server sends none (before M7-23). */
  readonly viewerState?: ViewerState | undefined;
  /** Length of the primary file in seconds; without it there is no progress bar. */
  readonly durationSeconds?: number | undefined;
}

/** What is drawn over the artwork of a card; all three absent means no badge at all. */
export interface WatchBadges {
  /** The check corner: the item, or every episode of a show or season, is watched. */
  readonly watched: boolean;
  /** Whole percent of the file that was played, 1 to 100; `undefined` without a position. */
  readonly progressPercent: number | undefined;
  /** Unwatched episodes of a show or season; `undefined` when none are left or for other kinds. */
  readonly unwatchedCount: number | undefined;
}

/** No badge: the state of an item that was never played, or of a kind without watch state. */
export const noWatchBadges: WatchBadges = {
  watched: false,
  progressPercent: undefined,
  unwatchedCount: undefined,
};

/** Whether "Mark as watched" applies to an item of this kind. */
export function canMarkWatched(kind: string): boolean {
  return markableKinds.has(kind);
}

/** Whether the kind is a show or season: its watched state applies to many episodes. */
export function isWatchContainer(kind: string): boolean {
  return containerKinds.has(kind);
}

/** The resume position in seconds, or `undefined` when playback starts at the beginning. */
export function resumeSeconds(viewerState: ViewerState | undefined): number | undefined {
  const position = viewerState?.positionSeconds;
  return position !== undefined && Number.isFinite(position) && position > 0 ? position : undefined;
}

function progressPercent(source: WatchSource): number | undefined {
  const position = resumeSeconds(source.viewerState);
  const duration = source.durationSeconds;
  if (position === undefined || duration === undefined || !(duration > 0)) {
    return undefined;
  }
  // At least 1 %, so a started item never shows an empty bar.
  return Math.min(percentScale, Math.max(1, Math.round((position / duration) * percentScale)));
}

/** The badges of an item: none without a viewer state. */
export function watchBadges(source: WatchSource): WatchBadges {
  const { viewerState } = source;
  if (viewerState === undefined) {
    return noWatchBadges;
  }
  const count = containerKinds.has(source.kind) ? (viewerState.unwatchedCount ?? 0) : 0;
  return {
    watched: viewerState.watched,
    progressPercent: progressPercent(source),
    unwatchedCount: count > 0 && !viewerState.watched ? count : undefined,
  };
}

/**
 * The state an item is expected to have once the server has marked it watched or unwatched;
 * shown until the server's answer is loaded. Either way the resume position is cleared. The
 * unwatched count of a show or season becomes 0 when watched and unknown (no badge) when not.
 */
export function expectedViewerState(
  current: ViewerState | undefined,
  watched: boolean,
): ViewerState {
  const expected: ViewerState = {
    watched,
    positionSeconds: 0,
    playCount: current?.playCount ?? 0,
  };
  if (current?.lastPlayedAt !== undefined) {
    expected.lastPlayedAt = current.lastPlayedAt;
  }
  if (watched && current?.unwatchedCount !== undefined) {
    expected.unwatchedCount = 0;
  }
  return expected;
}
