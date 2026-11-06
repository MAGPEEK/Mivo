import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the profiles feature. Its strings live in the JSON files next to this one. */
export const profilesNamespace = "profiles";

/** Strings of the profiles feature in every supported language. */
export const profilesLocales = defineLocales({ en, de });
