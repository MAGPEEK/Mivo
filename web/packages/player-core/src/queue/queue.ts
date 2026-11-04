/*
 * Queue model (PL-31): pure, immutable operations on QueueState; every function returns a new
 * state. Framework-free so the player, mini player, music bar and audiobook UI share it.
 *
 * Rules:
 * - next at the last item: repeat "all" wraps to the first, otherwise the result is "ended".
 *   Repeat "one" stays on the item when playback ended by itself; a user-initiated next skips it.
 * - previous more than `restartThresholdSeconds` into the item restarts the current item
 *   (`restartCurrent: true`); at the first item it restarts too, unless repeat "all" wraps.
 * - shuffle keeps the current item first and shuffles the rest for a seed; turning it off
 *   restores the original order. Items added while shuffled go to the end of the original order.
 * - removing the current item makes the item that follows it current (or the new last item).
 */
import type {
  NextResult,
  PreviousResult,
  QueueEntry,
  QueueItem,
  QueueState,
  RepeatMode,
} from "./queueTypes";
import { shuffled } from "./shuffle";

export const restartThresholdSeconds = 3;

export function emptyQueue<Meta>(): QueueState<Meta> {
  return { items: [], currentIndex: -1, repeat: "off", originalKeys: null, nextKey: 0 };
}

function entriesFrom<Meta>(items: readonly QueueItem<Meta>[], firstKey: number) {
  return items.map((item, offset) => ({ id: item.id, meta: item.meta, key: firstKey + offset }));
}

/** Replaces the queue content; repeat is kept, shuffle is reset. */
export function setQueue<Meta>(
  state: QueueState<Meta>,
  items: readonly QueueItem<Meta>[],
  startIndex = 0,
): QueueState<Meta> {
  const entries = entriesFrom(items, 0);
  const valid = startIndex >= 0 && startIndex < entries.length;
  const currentIndex = entries.length === 0 ? -1 : valid ? startIndex : 0;
  return {
    items: entries,
    currentIndex,
    repeat: state.repeat,
    originalKeys: null,
    nextKey: entries.length,
  };
}

function insertAt<Meta>(
  state: QueueState<Meta>,
  items: readonly QueueItem<Meta>[],
  position: number,
): QueueState<Meta> {
  if (items.length === 0) return state;
  const entries = entriesFrom(items, state.nextKey);
  const list = [...state.items.slice(0, position), ...entries, ...state.items.slice(position)];
  return {
    ...state,
    items: list,
    currentIndex: state.currentIndex === -1 ? 0 : state.currentIndex,
    nextKey: state.nextKey + entries.length,
  };
}

/** Inserts items right after the current one. */
export function playNext<Meta>(
  state: QueueState<Meta>,
  items: readonly QueueItem<Meta>[],
): QueueState<Meta> {
  return insertAt(state, items, state.currentIndex + 1);
}

/** Appends items to the end. */
export function addToQueue<Meta>(
  state: QueueState<Meta>,
  items: readonly QueueItem<Meta>[],
): QueueState<Meta> {
  return insertAt(state, items, state.items.length);
}

function inRange<Meta>(state: QueueState<Meta>, index: number): boolean {
  return Number.isInteger(index) && index >= 0 && index < state.items.length;
}

export function removeAt<Meta>(state: QueueState<Meta>, index: number): QueueState<Meta> {
  if (!inRange(state, index)) return state;
  const items = state.items.filter((_, position) => position !== index);
  const shifted = index < state.currentIndex ? state.currentIndex - 1 : state.currentIndex;
  const currentIndex = items.length === 0 ? -1 : Math.min(shifted, items.length - 1);
  return { ...state, items, currentIndex };
}

/** Moves an item; the current item stays current. */
export function moveItem<Meta>(
  state: QueueState<Meta>,
  from: number,
  to: number,
): QueueState<Meta> {
  if (!inRange(state, from) || !inRange(state, to) || from === to) return state;
  const currentKey = state.items[state.currentIndex]?.key;
  const items = [...state.items];
  items.splice(to, 0, ...items.splice(from, 1));
  return { ...state, items, currentIndex: items.findIndex((entry) => entry.key === currentKey) };
}

/** Drops everything after the current item. */
export function clearUpcoming<Meta>(state: QueueState<Meta>): QueueState<Meta> {
  return { ...state, items: state.items.slice(0, state.currentIndex + 1) };
}

export function jumpTo<Meta>(state: QueueState<Meta>, index: number): QueueState<Meta> {
  return inRange(state, index) ? { ...state, currentIndex: index } : state;
}

export function setRepeat<Meta>(state: QueueState<Meta>, repeat: RepeatMode): QueueState<Meta> {
  return { ...state, repeat };
}

/** Moves forward. `userInitiated` is true for the next button: repeat "one" then still advances. */
export function next<Meta>(state: QueueState<Meta>, userInitiated = false): NextResult<Meta> {
  if (state.currentIndex === -1) return { kind: "ended", state };
  if (state.repeat === "one" && !userInitiated) return { kind: "moved", state };
  if (state.currentIndex < state.items.length - 1) {
    return { kind: "moved", state: jumpTo(state, state.currentIndex + 1) };
  }
  if (state.repeat === "off") return { kind: "ended", state };
  return { kind: "moved", state: jumpTo(state, 0) };
}

export function previous<Meta>(
  state: QueueState<Meta>,
  positionSeconds: number,
): PreviousResult<Meta> {
  const atStart = state.currentIndex <= 0;
  if (positionSeconds > restartThresholdSeconds || (atStart && state.repeat !== "all")) {
    return { state, restartCurrent: state.currentIndex !== -1 };
  }
  const target = atStart ? state.items.length - 1 : state.currentIndex - 1;
  return { state: jumpTo(state, target), restartCurrent: false };
}

/** Turns shuffle on (deterministic for `seed`) or off (restores the original order). */
export function setShuffle<Meta>(
  state: QueueState<Meta>,
  enabled: boolean,
  seed: number,
): QueueState<Meta> {
  if (enabled === (state.originalKeys !== null)) return state;
  const current = state.items[state.currentIndex];
  return enabled ? shuffleOn(state, seed, current) : shuffleOff(state, current);
}

function shuffleOn<Meta>(
  state: QueueState<Meta>,
  seed: number,
  current: QueueEntry<Meta> | undefined,
): QueueState<Meta> {
  const rest = state.items.filter((entry) => entry !== current);
  const items = current ? [current, ...shuffled(rest, seed)] : rest;
  const originalKeys = state.items.map((entry) => entry.key);
  return { ...state, items, currentIndex: current ? 0 : -1, originalKeys };
}

function shuffleOff<Meta>(
  state: QueueState<Meta>,
  current: QueueEntry<Meta> | undefined,
): QueueState<Meta> {
  const order = new Map((state.originalKeys ?? []).map((key, position) => [key, position]));
  const unknownPosition = order.size;
  const rank = (entry: QueueEntry<Meta>) => order.get(entry.key) ?? unknownPosition;
  const items = [...state.items].sort((a, b) => rank(a) - rank(b));
  const currentIndex = current ? items.indexOf(current) : -1;
  return { ...state, items, currentIndex, originalKeys: null };
}

/** The current entry, or undefined for an empty queue. */
export function currentEntry<Meta>(state: QueueState<Meta>): QueueEntry<Meta> | undefined {
  return state.items[state.currentIndex];
}
