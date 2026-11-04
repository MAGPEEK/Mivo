import { emptyQueue } from "./queue";
import type { QueueEntry, QueueState, RepeatMode } from "./queueTypes";

const repeatModes: readonly RepeatMode[] = ["off", "all", "one"];

export function serializeQueue<Meta>(state: QueueState<Meta>): string {
  return JSON.stringify(state);
}

type RawField = "id" | "key" | "items" | "currentIndex" | "nextKey" | "repeat" | "originalKeys";

function isRecord(value: unknown): value is Partial<Record<RawField, unknown>> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isEntry(value: unknown): value is QueueEntry<unknown> {
  return isRecord(value) && typeof value.id === "string" && Number.isInteger(value.key);
}

function hasValidIndex(currentIndex: unknown, length: number): boolean {
  if (!Number.isInteger(currentIndex)) return false;
  return length === 0
    ? currentIndex === -1
    : (currentIndex as number) >= 0 && (currentIndex as number) < length;
}

function isValidState(value: unknown): boolean {
  if (!isRecord(value) || !Array.isArray(value.items) || !value.items.every(isEntry)) return false;
  const keys = value.items.map((entry) => entry.key);
  const { currentIndex, nextKey, repeat, originalKeys } = value;
  return (
    new Set(keys).size === keys.length &&
    hasValidIndex(currentIndex, keys.length) &&
    Number.isInteger(nextKey) &&
    keys.every((key) => key < (nextKey as number)) &&
    repeatModes.includes(repeat as RepeatMode) &&
    (originalKeys === null ||
      (Array.isArray(originalKeys) && originalKeys.every((key) => Number.isInteger(key))))
  );
}

/** Restores a serialised queue; anything invalid yields an empty queue. */
export function parseQueue<Meta>(json: string): QueueState<Meta> {
  try {
    const value: unknown = JSON.parse(json);
    return isValidState(value) ? (value as QueueState<Meta>) : emptyQueue<Meta>();
  } catch {
    return emptyQueue<Meta>();
  }
}
