import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the identify feature. Its strings live in the JSON files next to this one. */
export const identifyNamespace = "identify";

/** Strings of the identify feature in every supported language. */
export const identifyLocales = defineLocales({ en, de });
