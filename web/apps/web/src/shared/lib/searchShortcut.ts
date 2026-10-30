const searchShortcutKey = "k";
const searchShortcutCode = "KeyK";

// A key that produces a single Latin letter. Layouts such as Dvorak put a different letter on
// the physical K key; for them `event.key` decides, so Ctrl+(physical K) stays theirs.
const latinLetter = /^[a-z]$/i;

/**
 * Reports whether a key press is the global search shortcut: Ctrl+K, or Cmd+K on macOS.
 * Combinations with Shift or Alt stay free for the browser and other shortcuts.
 *
 * Keys typed while an input method composes text (Japanese, Chinese, ...) belong to the
 * composition. Layouts without Latin letters (e.g. Cyrillic) report another `event.key`, so
 * the physical K key (`event.code`) counts there.
 */
export function isSearchShortcut(event: KeyboardEvent): boolean {
  const hasCommandModifier = event.ctrlKey || event.metaKey;
  if (event.isComposing || !hasCommandModifier || event.shiftKey || event.altKey) {
    return false;
  }
  if (latinLetter.test(event.key)) {
    return event.key.toLowerCase() === searchShortcutKey;
  }
  return event.code === searchShortcutCode;
}

/** The search shortcut in `aria-keyshortcuts` syntax, for the element it focuses. */
export const searchShortcutAriaKeys = "Control+K Meta+K";
