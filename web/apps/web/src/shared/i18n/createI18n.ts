import { createInstance, type i18n as I18n } from "i18next";

import { detectLanguage, fallbackLanguage, supportedLanguages } from "./languages";
import { sharedLocales, sharedNamespace, toResources, type NamespaceRegistry } from "./resources";

/**
 * Creates an i18next instance in the language the browser prefers, falling back to English.
 * It always holds the shared namespace (the default for `t()`) plus `featureNamespaces`.
 * Every call returns a new instance, so the app and each test own their state. Components
 * receive it through `I18nextProvider`.
 */
export function createI18n(
  preferredLanguages: readonly string[],
  featureNamespaces: NamespaceRegistry = {},
): I18n {
  const namespaces: NamespaceRegistry = {
    ...featureNamespaces,
    [sharedNamespace]: sharedLocales,
  };
  const instance = createInstance();
  // The resources are bundled, so initialisation completes synchronously (initAsync: false)
  // and the returned promise carries no information.
  void instance.init({
    resources: toResources(namespaces),
    ns: Object.keys(namespaces),
    defaultNS: sharedNamespace,
    lng: detectLanguage(preferredLanguages),
    fallbackLng: fallbackLanguage,
    supportedLngs: [...supportedLanguages],
    initAsync: false,
    // React escapes rendered text already.
    interpolation: { escapeValue: false },
  });
  return instance;
}
