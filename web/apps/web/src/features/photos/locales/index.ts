import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the photos feature. Its strings live in the JSON files next to this one. */
export const photosNamespace = "photos";

/** Strings of the photos feature in every supported language. */
export const photosLocales = defineLocales({ en, de });
