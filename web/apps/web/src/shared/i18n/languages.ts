import { matchLanguage } from "../lib/matchLanguage";

/** UI languages with complete translations (ADR 0012). English is the base language. */
export const supportedLanguages = ["en", "de"] as const;

/** A UI language with complete translations. */
export type SupportedLanguage = (typeof supportedLanguages)[number];

/** Language used when the browser prefers none of the supported languages. */
export const fallbackLanguage: SupportedLanguage = "en";

/** Picks the UI language from the browser's preference list, e.g. `navigator.languages`. */
export function detectLanguage(preferredLanguages: readonly string[]): SupportedLanguage {
  return matchLanguage(preferredLanguages, supportedLanguages, fallbackLanguage);
}
