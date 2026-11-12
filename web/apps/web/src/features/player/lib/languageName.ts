import { normalizeLanguage } from "@mivo/player-core";

/**
 * The name of a track's language in the interface language, e.g. `ger` → "German" or
 * "Deutsch". `undefined` for an undetermined language, so the caller shows its own text. A
 * code the browser does not know is shown as it is, upper-cased.
 */
export function languageName(language: string | undefined, locale: string): string | undefined {
  const code = normalizeLanguage(language);
  if (code === "") {
    return undefined;
  }
  try {
    return new Intl.DisplayNames([locale], { type: "language", fallback: "code" }).of(code);
  } catch {
    return code.toUpperCase();
  }
}
