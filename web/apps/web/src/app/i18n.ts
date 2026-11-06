import type { i18n as I18n } from "i18next";

import { audiobooksLocales, audiobooksNamespace } from "../features/audiobooks";
import { authLocales, authNamespace } from "../features/auth";
import { devicesLocales, devicesNamespace } from "../features/devices";
import { homeLocales, homeNamespace } from "../features/home";
import { identifyLocales, identifyNamespace } from "../features/identify";
import { itemLocales, itemNamespace } from "../features/item";
import { libraryLocales, libraryNamespace } from "../features/library";
import { musicLocales, musicNamespace } from "../features/music";
import { photosLocales, photosNamespace } from "../features/photos";
import { playerLocales, playerNamespace } from "../features/player";
import { settingsLocales, settingsNamespace } from "../features/settings";
import { createI18n } from "../shared/i18n";

// The single registry of feature namespaces. It lives in the app layer because `shared` must
// not import features (dependency-cruiser). A feature adds strings only to its own
// `locales/{en,de}.json`; a new feature adds one line here.
/** Every feature namespace by name, with its strings in every supported language. */
export const featureNamespaces = {
  [libraryNamespace]: libraryLocales,
  [itemNamespace]: itemLocales,
  [playerNamespace]: playerLocales,
  [settingsNamespace]: settingsLocales,
  [identifyNamespace]: identifyLocales,
  [homeNamespace]: homeLocales,
  [musicNamespace]: musicLocales,
  [audiobooksNamespace]: audiobooksLocales,
  [photosNamespace]: photosLocales,
  [authNamespace]: authLocales,
  [devicesNamespace]: devicesLocales,
} as const;

/** Type of {@link featureNamespaces}; `i18next.d.ts` types `t()` keys with it. */
export type FeatureNamespaces = typeof featureNamespaces;

/** Creates the i18next instance of the app with the shared and every feature namespace. */
export function createAppI18n(preferredLanguages: readonly string[]): I18n {
  return createI18n(preferredLanguages, featureNamespaces);
}
