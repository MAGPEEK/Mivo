import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/**
 * i18next namespace of the audiobooks feature. Its strings live in the JSON files next to this
 * one.
 */
export const audiobooksNamespace = "audiobooks";

/** Strings of the audiobooks feature in every supported language. */
export const audiobooksLocales = defineLocales({ en, de });
