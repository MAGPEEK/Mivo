import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the settings feature. Its strings live in the JSON files next to this one. */
export const settingsNamespace = "settings";

/** Strings of the settings feature in every supported language. */
export const settingsLocales = defineLocales({ en, de });
