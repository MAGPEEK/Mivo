import { useCallback, useState } from "react";

import { readHeroEnabled, writeHeroEnabled } from "../lib/heroPreference";

/** The device preference `hero.enabled` and its setter. */
export interface HeroPreference {
  readonly enabled: boolean;
  readonly setEnabled: (enabled: boolean) => void;
}

/**
 * Whether this device shows the Home hero (HO-01; default on). Read once per mount; the
 * switch in Settings › This device (M5-80) uses `setEnabled`, which saves the choice.
 */
export function useHeroPreference(): HeroPreference {
  const [enabled, setEnabledState] = useState(readHeroEnabled);
  const setEnabled = useCallback((next: boolean) => {
    setEnabledState(next);
    writeHeroEnabled(next);
  }, []);
  return { enabled, setEnabled };
}
