import type { Resource } from "i18next";

import { supportedLanguages, type SupportedLanguage } from "./languages";
import deJson from "./locales/de.json";
import enJson from "./locales/en.json";

// How translations are registered: one i18next namespace per feature, its strings in
// `src/features/<feature>/locales/{en,de}.json`. `shared` must not import features
// (dependency-cruiser), so this file only knows the shared namespace and the generic helpers.
// Each feature defines its locales with `defineLocales` and exports them from its index;
// `src/app/i18n.ts` registers all feature namespaces once and passes them to `createI18n`.

/** The strings of one namespace in one language: nested objects with string leaves. */
export type LocaleStrings = Readonly<Record<string, unknown>>;

/** The strings of one namespace in every supported language. English is the base. */
export type NamespaceLocales<Strings extends LocaleStrings = LocaleStrings> = Readonly<
  Record<SupportedLanguage, Strings>
>;

/** Namespaces by name, as passed to {@link toResources}. */
export type NamespaceRegistry = Readonly<Record<string, NamespaceLocales>>;

/**
 * Pairs the English strings of a namespace with their translations. German is typed as the
 * English shape, so a missing German key fails type checking; the locale test additionally
 * rejects extra keys and empty values.
 */
export function defineLocales<Strings extends LocaleStrings>(locales: {
  readonly en: Strings;
  readonly de: NoInfer<Strings>;
}): NamespaceLocales<Strings> {
  return { en: locales.en, de: locales.de };
}

/** Name of the namespace with the strings of the app shell and the shared UI. */
export const sharedNamespace = "translation";

/** Shape of the shared namespace's strings. */
export type Translation = typeof enJson;

/** Strings of the app shell and the shared UI in every supported language. */
export const sharedLocales = defineLocales<Translation>({ en: enJson, de: deJson });

/** Turns namespaces by name into i18next resources: language, then namespace, then strings. */
export function toResources(namespaces: NamespaceRegistry): Resource {
  const namespaceEntries = Object.entries(namespaces);
  return Object.fromEntries(
    supportedLanguages.map((language) => [
      language,
      Object.fromEntries(namespaceEntries.map(([name, locales]) => [name, locales[language]])),
    ]),
  );
}
