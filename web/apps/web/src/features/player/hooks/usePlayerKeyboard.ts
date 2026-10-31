import { useEffect, useEffectEvent } from "react";

import { playerKeyCommand, type PlayerCommand } from "../lib/playerKeys";

/** What each player command does. */
export type PlayerCommandHandlers = Readonly<Record<PlayerCommand, () => void>>;

/**
 * Runs the player's keyboard shortcuts (`playerKeys.ts`) while the page is shown. The player
 * fills the page, so the shortcuts listen on the window: they work wherever focus is, except
 * for keys the focused control handles itself.
 */
export function usePlayerKeyboard(handlers: PlayerCommandHandlers): void {
  const onKeyDown = useEffectEvent((event: KeyboardEvent) => {
    const command = playerKeyCommand(event);
    if (command !== undefined) {
      // Space would scroll the page, the arrow keys too.
      event.preventDefault();
      handlers[command]();
    }
  });
  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      onKeyDown(event);
    };
    window.addEventListener("keydown", listener);
    return () => {
      window.removeEventListener("keydown", listener);
    };
  }, []);
}
