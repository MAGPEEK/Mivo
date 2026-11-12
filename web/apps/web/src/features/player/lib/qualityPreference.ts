// The playback quality, kept per device (PL-10): one browser profile remembers whether the
// viewer wants the original, Automatic or a fixed step of the ladder. The home setting that
// would fill it from the profile arrives later (task M4-16b notes), so on this device the
// last choice in the playback options is all there is.

import { originalQualityBitRate, type QualityOption } from "@mivo/player-core";
import { productId } from "@mivo/ui-tokens";

/** What the viewer wants: no limit, the client's pick from measured speed, or a fixed step. */
export type QualityChoice =
  | { readonly kind: "original" }
  | { readonly kind: "automatic" }
  | { readonly kind: "limited"; readonly maxBitRate: number };

/** Key of the preference in `localStorage`. */
export const qualityStorageKey = `${productId}.player.quality`;

const originalValue = "original";
const automaticValue = "automatic";

/** The choice nothing was saved for: play the file as it is. */
export const defaultQualityChoice: QualityChoice = { kind: "original" };

function parseChoice(stored: string): QualityChoice | undefined {
  if (stored === originalValue) {
    return { kind: "original" };
  }
  if (stored === automaticValue) {
    return { kind: "automatic" };
  }
  const maxBitRate = Number(stored);
  return Number.isInteger(maxBitRate) && maxBitRate > originalQualityBitRate
    ? { kind: "limited", maxBitRate }
    : undefined;
}

/**
 * Reads the saved choice; the original without storage, without a saved value or for a value
 * this version cannot read.
 */
export function readQualityChoice(storage: Storage | undefined): QualityChoice {
  try {
    const stored = storage?.getItem(qualityStorageKey);
    return (
      (stored === null || stored === undefined ? undefined : parseChoice(stored)) ??
      defaultQualityChoice
    );
  } catch {
    return defaultQualityChoice;
  }
}

/** Saves the choice; a missing storage or a failed write only costs the memory. */
export function writeQualityChoice(storage: Storage | undefined, choice: QualityChoice): void {
  const value = choice.kind === "limited" ? String(choice.maxBitRate) : choice.kind;
  try {
    storage?.setItem(qualityStorageKey, value);
  } catch {
    // Deliberately ignored: the choice then lasts for this page load only.
  }
}

/** The choice that picks `option`. */
export function choiceOf(option: QualityOption): QualityChoice {
  return option.kind === "limited"
    ? { kind: "limited", maxBitRate: option.maxBitRate }
    : defaultQualityChoice;
}

/**
 * The choice as this file allows it: a step the file does not offer (it lies above the
 * source's bit rate) plays the original instead of forcing a pointless transcode.
 */
export function availableQualityChoice(
  choice: QualityChoice,
  options: readonly QualityOption[],
): QualityChoice {
  if (choice.kind !== "limited") {
    return choice;
  }
  return options.some((option) => option.maxBitRate === choice.maxBitRate)
    ? choice
    : defaultQualityChoice;
}

/** The `maxBitRate` a playback starts with: Automatic starts with the original. */
export function startingMaxBitRate(choice: QualityChoice): number {
  return choice.kind === "limited" ? choice.maxBitRate : originalQualityBitRate;
}

/** Whether `option` is what `choice` asks for. */
export function isChosen(choice: QualityChoice, option: QualityOption): boolean {
  if (choice.kind === "automatic") {
    return false;
  }
  return startingMaxBitRate(choice) === option.maxBitRate;
}
