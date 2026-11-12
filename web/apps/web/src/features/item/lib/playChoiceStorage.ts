import type { PlayChoice, StoredChoice } from "./playChoice";

// Per item, so a choice for one film does not leak into the next. The value is only ever a
// choice, never a credential, so localStorage is the right place.
const storageKeyPrefix = "mivo.playChoice.";

// Key of the history state that carries the choice to the player route, next to the queue's.
const handoffKey = "mivoPlayChoice";

function isString(value: unknown): value is string {
  return typeof value === "string";
}

/** Keeps only the fields of the right type: stored data is user-controllable. */
function toStoredChoice(value: unknown): StoredChoice {
  if (typeof value !== "object" || value === null) {
    return {};
  }
  const { fileId, audio, subtitle } = value as Record<string, unknown>;
  return {
    ...(isString(fileId) ? { fileId } : {}),
    ...(isString(audio) ? { audio } : {}),
    ...(isString(subtitle) ? { subtitle } : {}),
  };
}

/** The choice remembered for the item on this device; empty without one or without storage. */
export function readStoredChoice(storage: Storage | undefined, itemId: string): StoredChoice {
  try {
    const stored = storage?.getItem(storageKeyPrefix + itemId);
    return stored === null || stored === undefined ? {} : toStoredChoice(JSON.parse(stored));
  } catch {
    return {};
  }
}

/** Remembers the choice for the item; a blocked or full storage only forgets it on reload. */
export function writeStoredChoice(
  storage: Storage | undefined,
  itemId: string,
  choice: StoredChoice,
): void {
  try {
    storage?.setItem(storageKeyPrefix + itemId, JSON.stringify(choice));
  } catch {
    // Deliberately ignored, see above.
  }
}

/** The history state that carries `choice` to the player route; empty without a choice. */
export function playChoiceState(choice: PlayChoice): Record<string, unknown> | undefined {
  return choice.audioStreamIndex === undefined && choice.subtitleTrackId === undefined
    ? undefined
    : { [handoffKey]: choice };
}

/**
 * The choice a history state carries, validated field by field; `{}` for none. For the player,
 * which reads it once when it opens.
 */
export function readPlayChoice(state: unknown): PlayChoice {
  if (typeof state !== "object" || state === null) {
    return {};
  }
  const handoff = (state as Record<string, unknown>)[handoffKey];
  if (typeof handoff !== "object" || handoff === null) {
    return {};
  }
  const { audioStreamIndex, subtitleTrackId } = handoff as Record<string, unknown>;
  return {
    ...(Number.isInteger(audioStreamIndex) && (audioStreamIndex as number) >= 0
      ? { audioStreamIndex: audioStreamIndex as number }
      : {}),
    ...(isString(subtitleTrackId) ? { subtitleTrackId } : {}),
  };
}
