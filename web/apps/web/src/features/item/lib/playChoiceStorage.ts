import type { StoredChoice } from "./playChoice";

// One key for all items, newest first and capped, so browsing a large library does not leave
// a key per item behind. The value is only ever a choice, never a credential.
const storageKey = "mivo.playChoices";

/** Most items whose choice is remembered; the least recently picked is forgotten first. */
export const maxRememberedChoices = 200;

interface RememberedChoice {
  readonly itemId: string;
  readonly choice: StoredChoice;
}

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

function toRemembered(value: unknown): RememberedChoice | undefined {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }
  const { itemId, choice } = value as Record<string, unknown>;
  return isString(itemId) ? { itemId, choice: toStoredChoice(choice) } : undefined;
}

function readAll(storage: Storage | undefined): RememberedChoice[] {
  try {
    const stored = storage?.getItem(storageKey);
    const parsed: unknown = stored === null || stored === undefined ? [] : JSON.parse(stored);
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed
      .map(toRemembered)
      .filter((entry): entry is RememberedChoice => entry !== undefined)
      .slice(0, maxRememberedChoices);
  } catch {
    return [];
  }
}

/** The choice remembered for the item on this device; empty without one or without storage. */
export function readStoredChoice(storage: Storage | undefined, itemId: string): StoredChoice {
  return readAll(storage).find((entry) => entry.itemId === itemId)?.choice ?? {};
}

/** Remembers the choice for the item; a blocked or full storage only forgets it on reload. */
export function writeStoredChoice(
  storage: Storage | undefined,
  itemId: string,
  choice: StoredChoice,
): void {
  const others = readAll(storage).filter((entry) => entry.itemId !== itemId);
  const entries = [{ itemId, choice }, ...others].slice(0, maxRememberedChoices);
  try {
    storage?.setItem(storageKey, JSON.stringify(entries));
  } catch {
    // Deliberately ignored, see above.
  }
}
