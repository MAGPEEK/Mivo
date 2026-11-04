/** An item to enqueue: identity is an id string plus opaque metadata owned by the caller. */
export interface QueueItem<Meta> {
  readonly id: string;
  readonly meta: Meta;
}

/**
 * A queued item. `key` is unique within one queue, so the same id may appear several times and
 * still be told apart (needed to track the current item through shuffle, move and remove).
 */
export interface QueueEntry<Meta> extends QueueItem<Meta> {
  readonly key: number;
}

export type RepeatMode = "off" | "all" | "one";

export interface QueueState<Meta> {
  /** Items in play order (the shuffled order while `originalKeys` is set). */
  readonly items: readonly QueueEntry<Meta>[];
  /** Index of the current item in `items`, -1 for an empty queue. */
  readonly currentIndex: number;
  readonly repeat: RepeatMode;
  /** Entry keys in their pre-shuffle order; null while shuffle is off. */
  readonly originalKeys: readonly number[] | null;
  /** Next free entry key. */
  readonly nextKey: number;
}

/** Outcome of moving forward: either a new state or the end of the queue. */
export type NextResult<Meta> =
  | { readonly kind: "moved"; readonly state: QueueState<Meta> }
  | { readonly kind: "ended"; readonly state: QueueState<Meta> };

/** Outcome of moving back; `restartCurrent` tells the player to seek to 0 instead. */
export interface PreviousResult<Meta> {
  readonly state: QueueState<Meta>;
  readonly restartCurrent: boolean;
}
