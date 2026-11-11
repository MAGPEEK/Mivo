/** Digits of a profile PIN (api/openapi.yaml, `ProfileCreate.pin`). */
export const pinLength = 4;

/**
 * What the editor does with the PIN: `keep` leaves a stored PIN alone, `set` stores `value`
 * (an empty value on a profile without PIN means "none"), `remove` deletes the stored PIN.
 */
export type PinDraft =
  | { readonly action: "keep" }
  | { readonly action: "set"; readonly value: string }
  | { readonly action: "remove" };

/** Why a PIN draft cannot be saved. */
export type PinProblem = "required" | "invalid";

/** The PIN draft an editor starts with: untouched with a stored PIN, an empty field without. */
export function initialPin(hasPin: boolean): PinDraft {
  return hasPin ? { action: "keep" } : { action: "set", value: "" };
}

/** Keeps the ASCII digits of typed or pasted text, at most {@link pinLength}. */
export function pinDigits(input: string): string {
  return input.replaceAll(/[^0-9]/g, "").slice(0, pinLength);
}

/** Whether `value` is a PIN the server accepts: exactly {@link pinLength} ASCII digits. */
export function isValidPin(value: string): boolean {
  return value.length === pinLength && pinDigits(value) === value;
}

/**
 * Checks a PIN draft. `hasPin` tells whether the profile has a stored PIN: then "Change PIN"
 * with an empty field is a mistake, while on a profile without PIN it means "no PIN".
 */
export function pinProblem(pin: PinDraft, hasPin: boolean): PinProblem | undefined {
  if (pin.action !== "set") {
    return undefined;
  }
  if (pin.value === "") {
    return hasPin ? "required" : undefined;
  }
  return isValidPin(pin.value) ? undefined : "invalid";
}

/**
 * The `pin` property of an update: the new PIN, `null` to remove it, `undefined` to leave it
 * out of the body so the stored PIN stays.
 */
export function pinChange(pin: PinDraft): string | null | undefined {
  switch (pin.action) {
    case "keep":
      return undefined;
    case "remove":
      return null;
    case "set":
      return pin.value === "" ? undefined : pin.value;
  }
}
