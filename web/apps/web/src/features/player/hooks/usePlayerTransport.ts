import { dispatchPlayerMinimise } from "../lib/playerEvents";
import { usePlayerExit, type PlayerExit } from "./usePlayerExit";
import { usePlayerQueue, type PlayerQueueView } from "./usePlayerQueue";

/** Seconds the "back" button jumps (PL-03). */
export const skipBackwardSeconds = 10;

/** Seconds the "forward" button jumps (PL-03). */
export const skipForwardSeconds = 30;

/** What the transport buttons, repeat, shuffle and Minimise do. */
export interface TransportActions extends PlayerExit {
  readonly previous: () => void;
  readonly skipBackward: () => void;
  readonly skipForward: () => void;
  readonly next: () => void;
  /** Ends playback and returns to the item's page. */
  readonly stop: () => void;
  /** Asks the mini player (M5-67) to take over; does nothing until it exists. */
  readonly minimise: () => void;
  readonly cycleRepeat: () => void;
  readonly toggleShuffle: () => void;
}

/** Inputs of {@link usePlayerTransport}. */
export interface PlayerTransportOptions {
  readonly itemId: string;
  /** Displayed position in seconds. */
  readonly position: number;
  /** Probed duration in seconds; `undefined` until known. */
  readonly duration: number | undefined;
  readonly seek: (position: number) => void;
}

/** The transport's actions and the queue state the controls show. */
export interface PlayerTransport {
  readonly queue: PlayerQueueView;
  readonly actions: TransportActions;
}

/**
 * The centre and right controls of the player (PL-03, PL-04): jumps, previous and next through
 * the queue, repeat and shuffle, stop, Minimise and the ways out of the player. Stop opens the
 * item's page; leaving the player closes its session.
 */
export function usePlayerTransport(options: PlayerTransportOptions): PlayerTransport {
  const { itemId, position, duration, seek } = options;
  const { exit, openItem } = usePlayerExit(itemId);
  const queue = usePlayerQueue({
    itemId,
    position,
    restart: () => {
      seek(0);
    },
  });
  return {
    queue: queue.view,
    actions: {
      exit,
      openItem,
      previous: queue.previous,
      skipBackward: () => {
        seek(Math.max(0, position - skipBackwardSeconds));
      },
      skipForward: () => {
        const target = position + skipForwardSeconds;
        seek(duration === undefined ? target : Math.min(duration, target));
      },
      next: queue.next,
      stop: openItem,
      minimise: () => {
        dispatchPlayerMinimise(window, { itemId, position });
      },
      cycleRepeat: queue.cycleRepeat,
      toggleShuffle: queue.toggleShuffle,
    },
  };
}
