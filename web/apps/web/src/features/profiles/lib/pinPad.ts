// Framework-free model of the PIN pad (US-02): the digit buffer, the keys of the pad and the
// focus movement between them. No React here, so the webOS app can reuse it.

/** Number of digits of a profile PIN (ADR 0032 §4). */
export const pinLength = 4;

/** A key of the pad: a digit, "delete" (⌫) or "submit" (OK). */
export type PadKey = number | "delete" | "submit";

/** The pad from top left to bottom right, three keys per row like a phone keypad. */
export const padKeys: readonly PadKey[] = [1, 2, 3, 4, 5, 6, 7, 8, 9, "delete", 0, "submit"];

/** Keys per row of {@link padKeys}. */
export const padColumns = 3;

/** The buffer after pressing `key`; a full buffer ignores further digits. */
export function applyPadKey(buffer: string, key: PadKey): string {
  if (key === "delete") {
    return buffer.slice(0, -1);
  }
  if (key === "submit" || buffer.length >= pinLength) {
    return buffer;
  }
  return `${buffer}${String(key)}`;
}

/** Whether `buffer` holds a whole PIN. */
export function isCompletePin(buffer: string): boolean {
  return buffer.length === pinLength;
}

/**
 * The pad key a keyboard or TV remote key stands for: digits (also from the number pad, which
 * report the same `key`), Backspace/Delete and Enter. `undefined` for any other key; Enter on a
 * focused pad button is left to the button itself.
 */
export function padKeyForKeyboard(key: string): PadKey | undefined {
  if (/^[0-9]$/.test(key)) {
    return Number(key);
  }
  if (key === "Backspace" || key === "Delete") {
    return "delete";
  }
  return undefined;
}

const rowStep = padColumns;

/**
 * The pad index focus moves to for an arrow key, or `undefined` for other keys. Movement stops
 * at the edges instead of wrapping, so a remote never jumps across the pad unexpectedly.
 */
export function movePadFocus(index: number, key: string): number | undefined {
  const column = index % padColumns;
  switch (key) {
    case "ArrowLeft":
      return column > 0 ? index - 1 : index;
    case "ArrowRight":
      return column < padColumns - 1 ? index + 1 : index;
    case "ArrowUp":
      return index - rowStep >= 0 ? index - rowStep : index;
    case "ArrowDown":
      return index + rowStep < padKeys.length ? index + rowStep : index;
    default:
      return undefined;
  }
}
