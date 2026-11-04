/** Search parameters of Settings › Libraries. */
export interface SettingsSearch {
  /** True when the add-library dialog should open on arrival (M3-19n). */
  readonly add?: true;
}

/**
 * Reads `?add=1` (also `true`); anything else is dropped. Empty-state links use it, so the
 * dialog can open without an extra click once it exists.
 */
export function validateSettingsSearch(raw: Record<string, unknown>): SettingsSearch {
  const add = raw["add"];
  return add === true || add === 1 || add === "1" || add === "true" ? { add: true } : {};
}
