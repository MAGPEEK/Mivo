// How a page (an album, later playlists) hands a queue to the player: in the history entry's
// state, not the URL, because an album can hold hundreds of tracks. The player reads it once
// when it opens; a reload keeps it, a plain link to the same item carries none.
//
// History state is user-controllable (devtools, a crafted link target in another tab), so it is
// validated strictly before it can become a queue.

/** The queue a page hands to the player. */
export interface QueueHandoff {
  /** Item IDs in play order; the opened item is one of them. */
  readonly ids: readonly string[];
  /** Whether the player starts with shuffle on. */
  readonly shuffle: boolean;
}

const handoffKey = "mivoQueue";

/** Most items a handed-over queue may hold. */
export const maxHandoffIds = 5000;
const maxIdLength = 64;
// Item IDs are UUID-like: letters, digits and hyphens.
const idPattern = /^[A-Za-z0-9-]+$/;

/** The history state that carries `handoff` to the player route. */
export function queueHandoffState(handoff: QueueHandoff): Record<string, unknown> {
  return { [handoffKey]: handoff };
}

function isItemId(value: unknown): value is string {
  return typeof value === "string" && value.length <= maxIdLength && idPattern.test(value);
}

function areValidIds(ids: unknown): ids is string[] {
  return (
    Array.isArray(ids) &&
    ids.length > 0 &&
    ids.length <= maxHandoffIds &&
    ids.every(isItemId) &&
    new Set(ids).size === ids.length
  );
}

function isHandoff(value: unknown): value is QueueHandoff {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const { ids, shuffle } = value as Record<string, unknown>;
  return areValidIds(ids) && typeof shuffle === "boolean";
}

/**
 * The queue the history state carries, or `undefined` when there is none, it is malformed or
 * it does not hold `itemId` (state that outlived its navigation).
 */
export function readQueueHandoff(state: unknown, itemId: string): QueueHandoff | undefined {
  if (typeof state !== "object" || state === null) {
    return undefined;
  }
  const handoff = (state as Record<string, unknown>)[handoffKey];
  return isHandoff(handoff) && handoff.ids.includes(itemId) ? handoff : undefined;
}
