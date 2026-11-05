// Events the player sends to the rest of the app. The mini player (M5-67) listens for the
// minimise event on `window`; until it exists, nobody listens and Minimise does nothing.

/** Name of the event the Minimise button dispatches on `window`. */
export const playerMinimiseEvent = "player:minimise";

/** What the minimise event carries: enough for the mini player to continue the item. */
export interface PlayerMinimiseDetail {
  readonly itemId: string;
  /** Displayed position in seconds when the user minimised. */
  readonly position: number;
}

/** Dispatches the minimise event on `target`. */
export function dispatchPlayerMinimise(target: EventTarget, detail: PlayerMinimiseDetail): void {
  target.dispatchEvent(new CustomEvent<PlayerMinimiseDetail>(playerMinimiseEvent, { detail }));
}
