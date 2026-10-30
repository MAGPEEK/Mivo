import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the library feature. Its strings live in the JSON files next to this one. */
export const libraryNamespace = "library";

/** Strings of the library feature in every supported language. */
export const libraryLocales = defineLocales({ en, de });
