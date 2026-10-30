const searchShortcutKey = "k";

/**
 * Reports whether a key press is the global search shortcut: Ctrl+K, or Cmd+K on macOS.
 * Combinations with Shift or Alt stay free for the browser and other shortcuts.
 */
export function isSearchShortcut(event: KeyboardEvent): boolean {
  const hasCommandModifier = event.ctrlKey || event.metaKey;
  return (
    hasCommandModifier &&
    !event.shiftKey &&
    !event.altKey &&
    event.key.toLowerCase() === searchShortcutKey
  );
}

/** The search shortcut in `aria-keyshortcuts` syntax, for the element it focuses. */
export const searchShortcutAriaKeys = "Control+K Meta+K";
