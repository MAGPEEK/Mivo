import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the music feature. Its strings live in the JSON files next to this one. */
export const musicNamespace = "music";

/** Strings of the music feature in every supported language. */
export const musicLocales = defineLocales({ en, de });
