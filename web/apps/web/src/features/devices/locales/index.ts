import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the devices feature. Its strings live in the JSON files next to this one. */
export const devicesNamespace = "devices";

/** Strings of the devices feature in every supported language. */
export const devicesLocales = defineLocales({ en, de });
