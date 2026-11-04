/**
 * Search parameters of Settings › Libraries. They carry the open state of the library dialog,
 * so a reload keeps it open (UX principle 6).
 */
export interface SettingsSearch {
  /** True while the add-library dialog is open. */
  readonly add?: true;
  /** ID of the library whose edit dialog is open. */
  readonly edit?: string;
}

/**
 * Reads `?add=1` (also `true`) and `?edit=<libraryId>`; anything else is dropped. Empty-state
 * links use `add`, so the dialog opens without an extra click. `add` wins over `edit`.
 */
export function validateSettingsSearch(raw: Record<string, unknown>): SettingsSearch {
  const add = raw["add"];
  if (add === true || add === 1 || add === "1" || add === "true") {
    return { add: true };
  }
  const edit = raw["edit"];
  return typeof edit === "string" && edit !== "" ? { edit } : {};
}
