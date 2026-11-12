import {
  previousInQueue,
  setQueue,
  setRepeat,
  setShuffle,
  nextInQueue,
  type RepeatMode,
} from "@mivo/player-core";
import { useRouter } from "@tanstack/react-router";
import { useState } from "react";

import {
  canSkipToNext,
  currentItemId,
  hasQueue,
  isShuffled,
  nextRepeatMode,
  queueFromHandoff,
  singleItemQueue,
  type PlayerQueueItem,
  type PlayerQueueState,
} from "../lib/playerQueue";
import { readQueueHandoff } from "../lib/queueHandoff";

/** Which queue setting changed last; the controls announce it in a live region. */
export type QueueAnnouncement = "repeat" | "shuffle";

/** What the controls show of the queue. */
export interface PlayerQueueView {
  readonly canGoPrevious: boolean;
  readonly canGoNext: boolean;
  readonly repeat: RepeatMode;
  readonly shuffle: boolean;
  /** The setting the user changed last; `undefined` until one changes. */
  readonly announcement: QueueAnnouncement | undefined;
}

/** The queue's state and what the transport and queue buttons do with it. */
export interface PlayerQueue {
  readonly view: PlayerQueueView;
  readonly previous: () => void;
  readonly next: () => void;
  readonly cycleRepeat: () => void;
  readonly toggleShuffle: () => void;
  /** Replaces the queue, e.g. from the queue panel or "Play all" (M5-65); keeps repeat. */
  readonly replace: (items: readonly PlayerQueueItem[], startIndex?: number) => void;
}

/** Inputs of {@link usePlayerQueue}. */
export interface PlayerQueueOptions {
  /** The item the player plays now. */
  readonly itemId: string;
  /** Displayed position in seconds; previous restarts the item when it is far enough in. */
  readonly position: number;
  /** Moves to the start of the playing item. */
  readonly restart: () => void;
}

// Shuffle seeds are 32-bit unsigned integers.
const shuffleSeedRange = 2 ** 32;

function randomSeed(): number {
  return Math.floor(Math.random() * shuffleSeedRange);
}

/** The queue a page handed over in the history state (an album), else the item alone. */
function initialQueue(itemId: string, historyState: unknown): PlayerQueueState {
  const handoff = readQueueHandoff(historyState, itemId);
  return handoff === undefined
    ? singleItemQueue(itemId)
    : queueFromHandoff(handoff, itemId, randomSeed());
}

/**
 * Keeps the queue in sync with the route: when the route changes to another item than the
 * queue's current one (a link, the browser's history), the queue starts over with that item
 * alone. Previous and next change the route to the queue's current item, so they keep it.
 * Only a route change counts: right after previous or next, the route still names the old item.
 * State derived during rendering, as React recommends instead of an effect.
 */
function useRouteSyncedQueue(itemId: string) {
  const router = useRouter();
  const [state, setState] = useState(() => initialQueue(itemId, router.state.location.state));
  const [routeItemId, setRouteItemId] = useState(itemId);
  if (routeItemId !== itemId) {
    setRouteItemId(itemId);
    if (currentItemId(state) !== itemId) {
      const restarted = singleItemQueue(itemId, state);
      setState(restarted);
      return [restarted, setState] as const;
    }
  }
  return [state, setState] as const;
}

/** Opens the player for another queued item, replacing the player's history entry. */
function useOpenQueueItem(): (itemId: string) => void {
  const router = useRouter();
  return (itemId) => {
    void router.navigate({
      to: "/items/$itemId/play",
      params: { itemId },
      search: {},
      replace: true,
    });
  };
}

/**
 * The player's queue (PL-31 via player-core): previous and next move through it and open the
 * item they land on, replacing the player's history entry so Back still leaves the player.
 * Repeat cycles off, all, one; shuffle toggles. Without a queue, previous and next are off.
 */
export function usePlayerQueue({ itemId, position, restart }: PlayerQueueOptions): PlayerQueue {
  const [state, setState] = useRouteSyncedQueue(itemId);
  const [announcement, setAnnouncement] = useState<QueueAnnouncement>();
  const openItem = useOpenQueueItem();
  const open = (moved: PlayerQueueState) => {
    setState(moved);
    const target = currentItemId(moved);
    if (target !== undefined && target !== itemId) {
      openItem(target);
    }
  };
  return {
    view: {
      canGoPrevious: hasQueue(state),
      canGoNext: canSkipToNext(state),
      repeat: state.repeat,
      shuffle: isShuffled(state),
      announcement,
    },
    previous: () => {
      const result = previousInQueue(state, position);
      if (result.restartCurrent) {
        restart();
      } else {
        open(result.state);
      }
    },
    next: () => {
      const result = nextInQueue(state, true);
      if (result.kind === "moved") {
        open(result.state);
      }
    },
    cycleRepeat: () => {
      setState(setRepeat(state, nextRepeatMode(state.repeat)));
      setAnnouncement("repeat");
    },
    toggleShuffle: () => {
      setState(setShuffle(state, !isShuffled(state), randomSeed()));
      setAnnouncement("shuffle");
    },
    replace: (items, startIndex) => {
      open(setQueue(state, items, startIndex));
    },
  };
}
