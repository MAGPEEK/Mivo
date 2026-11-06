import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the users feature. Its strings live in the JSON files next to this one. */
export const usersNamespace = "users";

/** Strings of the users feature in every supported language. */
export const usersLocales = defineLocales({ en, de });
