import { productId } from "@mivo/ui-tokens";

import { browserLocalStorage, readFlag, writeFlag } from "../../../shared/lib/persistentFlag";

/**
 * `localStorage` key of the device preference `hero.enabled` (HO-01, AD-01). Settings › This
 * device (M5-80) moves it into its preference store and must keep the name, so existing
 * choices survive. Derived from the brand id (ADR 0002).
 */
export const heroEnabledStorageKey = `${productId}.hero.enabled`;

// The hero is on until the user switches it off on this device (HO-01).
const heroEnabledByDefault = true;

/** Whether this device shows the Home hero. */
export function readHeroEnabled(storage: Storage | undefined = browserLocalStorage()): boolean {
  return readFlag(storage, heroEnabledStorageKey, heroEnabledByDefault);
}

/** Saves whether this device shows the Home hero. */
export function writeHeroEnabled(
  enabled: boolean,
  storage: Storage | undefined = browserLocalStorage(),
): void {
  writeFlag(storage, heroEnabledStorageKey, enabled);
}
