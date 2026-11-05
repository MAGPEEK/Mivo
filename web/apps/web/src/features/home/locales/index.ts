import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the home feature. Its strings live in the JSON files next to this one. */
export const homeNamespace = "home";

/** Strings of the home feature in every supported language. */
export const homeLocales = defineLocales({ en, de });
