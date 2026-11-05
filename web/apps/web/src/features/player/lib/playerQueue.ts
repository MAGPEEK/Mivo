// The player's view of the queue model from player-core (PL-31): which transport buttons work
// and which repeat mode follows. Pure functions, so the hook only holds state.

import {
  currentEntry,
  emptyQueue,
  nextInQueue,
  setQueue,
  type QueueItem,
  type QueueState,
  type RepeatMode,
} from "@mivo/player-core";

/** What the player keeps per queued item; the item page and the route need only the id. */
export type PlayerQueueMeta = Readonly<Record<string, never>>;

/** The queue the player holds. */
export type PlayerQueueState = QueueState<PlayerQueueMeta>;

/** An item to put into the player's queue. */
export type PlayerQueueItem = QueueItem<PlayerQueueMeta>;

// The order the repeat button walks through.
const repeatCycle: readonly RepeatMode[] = ["off", "all", "one"];

/** The repeat mode after `repeat` when the user presses the repeat button. */
export function nextRepeatMode(repeat: RepeatMode): RepeatMode {
  const index = repeatCycle.indexOf(repeat);
  return repeatCycle[(index + 1) % repeatCycle.length] ?? "off";
}

/** A queue that holds only the item the player opened: no previous, no next. */
export function singleItemQueue(itemId: string, from: PlayerQueueState = emptyQueue()) {
  return setQueue(from, [{ id: itemId, meta: {} }]);
}

/** Whether the queue holds more than the playing item, so previous and next mean something. */
export function hasQueue(state: PlayerQueueState): boolean {
  return state.items.length > 1;
}

/** Whether the next button moves to another item (not at the end without repeat). */
export function canSkipToNext(state: PlayerQueueState): boolean {
  return hasQueue(state) && nextInQueue(state, true).kind === "moved";
}

/** The id of the item the queue plays now, if any. */
export function currentItemId(state: PlayerQueueState): string | undefined {
  return currentEntry(state)?.id;
}

/** Whether shuffle is on. */
export function isShuffled(state: PlayerQueueState): boolean {
  return state.originalKeys !== null;
}
