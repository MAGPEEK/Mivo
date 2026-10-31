// The player's keyboard map (task M2-30): Space/K play-pause, ←/→ ±10 s, F full screen,
// I stream info, M mute, Escape closes the overlay or leaves full screen. Keys a focused
// control handles itself (Space on a button, arrows on a slider) are left to the control.

/** What a key asks the player to do. */
export type PlayerCommand =
  | "togglePlay"
  | "seekBackward"
  | "seekForward"
  | "toggleFullscreen"
  | "toggleStreamInfo"
  | "toggleMute"
  | "escape";

/** Seconds the arrow keys move the position. */
export const seekStepSeconds = 10;

// Letters are compared in lower case, so Caps Lock and Shift do not matter.
const commandsByKey: ReadonlyMap<string, PlayerCommand> = new Map([
  [" ", "togglePlay"],
  ["k", "togglePlay"],
  ["ArrowLeft", "seekBackward"],
  ["ArrowRight", "seekForward"],
  ["f", "toggleFullscreen"],
  ["i", "toggleStreamInfo"],
  ["m", "toggleMute"],
  ["Escape", "escape"],
]);

/** The parts of a `KeyboardEvent` the map reads. */
export interface PlayerKeyEvent {
  readonly key: string;
  readonly altKey: boolean;
  readonly ctrlKey: boolean;
  readonly metaKey: boolean;
  /** Whether the key is held down and the browser repeats it. */
  readonly repeat: boolean;
  readonly target: EventTarget | null;
}

const seekCommands: ReadonlySet<PlayerCommand> = new Set(["seekBackward", "seekForward"]);

/**
 * The command for a key press, or `undefined` when the key is not the player's: unmapped,
 * pressed with Alt, Ctrl or Meta (browser and system shortcuts), a held toggle key (only the
 * arrows repeat, so holding F does not flicker full screen), or handled by the focused control
 * itself.
 */
export function playerKeyCommand(event: PlayerKeyEvent): PlayerCommand | undefined {
  if (event.altKey || event.ctrlKey || event.metaKey) {
    return undefined;
  }
  const key = event.key.length === 1 ? event.key.toLowerCase() : event.key;
  const command = commandsByKey.get(key);
  if (command === undefined || (event.repeat && !seekCommands.has(command))) {
    return undefined;
  }
  return isHandledByTarget(key, event.target) ? undefined : command;
}

/**
 * Space activates a focused button; sliders move with the arrow keys themselves. Letters are
 * never left to the focused element: the player has no text input.
 */
function isHandledByTarget(key: string, target: EventTarget | null): boolean {
  if (!(target instanceof Element)) {
    return false;
  }
  if (key === " ") {
    return target.closest("button, a, select, textarea") !== null;
  }
  if (key === "ArrowLeft" || key === "ArrowRight") {
    return target.closest("input, select, textarea") !== null;
  }
  return false;
}

const scrubberSteps: ReadonlyMap<string, number> = new Map([
  ["ArrowLeft", -seekStepSeconds],
  ["ArrowDown", -seekStepSeconds],
  ["ArrowRight", seekStepSeconds],
  ["ArrowUp", seekStepSeconds],
]);

/**
 * Seconds an arrow key moves the scrubber (±{@link seekStepSeconds}), or `undefined` for other
 * keys, which the native slider handles (Home, End, Page Up, Page Down).
 */
export function scrubberStepSeconds(key: string): number | undefined {
  return scrubberSteps.get(key);
}
