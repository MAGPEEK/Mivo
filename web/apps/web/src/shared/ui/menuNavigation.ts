/** Index of the next enabled entry from `from` in direction `step`, wrapping; -1 if none. */
export function nextEnabledIndex(disabled: readonly boolean[], from: number, step: 1 | -1): number {
  const count = disabled.length;
  for (let offset = 1; offset <= count; offset += 1) {
    const index = (((from + step * offset) % count) + count) % count;
    if (disabled[index] !== true) {
      return index;
    }
  }
  return -1;
}

// "aaa" means "the next entry with a", as in native list boxes, not a label starting with "aaa".
function isRepeatedCharacter(text: string): boolean {
  return text === text.charAt(0).repeat(text.length);
}

/**
 * Type-ahead match: the next enabled label starting with `typed` (case-insensitive). A single
 * or repeated character searches after `current` so pressing a letter again cycles; longer input
 * may stay on the current entry.
 */
export function findTypeAhead(
  labels: readonly string[],
  disabled: readonly boolean[],
  typed: string,
  current: number,
): number {
  const cycling = isRepeatedCharacter(typed);
  const needle = (cycling ? typed.slice(0, 1) : typed).toLowerCase();
  // Without a current entry (-1) longer input searches from the top, not from the last entry.
  const start = cycling ? current + 1 : Math.max(current, 0);
  for (let offset = 0; offset < labels.length; offset += 1) {
    const index = (((start + offset) % labels.length) + labels.length) % labels.length;
    if (disabled[index] !== true && labels[index]?.toLowerCase().startsWith(needle) === true) {
      return index;
    }
  }
  return -1;
}
