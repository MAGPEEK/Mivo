import type { SupportedLanguage } from "./languages";
import deJson from "./locales/de.json";
import enJson from "./locales/en.json";

/** Shape of one language's strings. English is the base, so it defines the keys. */
export type Translation = typeof enJson;

// Typing German as the English shape turns a missing German key into a compile error. The
// locale test additionally rejects extra keys and empty values.
const de: Translation = deJson;

/** Bundled translations of every supported language, in the `translation` namespace. */
export const resources = {
  en: { translation: enJson },
  de: { translation: de },
} as const satisfies Record<SupportedLanguage, { translation: Translation }>;
