import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the item feature. Its strings live in the JSON files next to this one. */
export const itemNamespace = "item";

/** Strings of the item feature in every supported language. */
export const itemLocales = defineLocales({ en, de });
