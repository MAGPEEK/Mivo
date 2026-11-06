import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the auth feature. Its strings live in the JSON files next to this one. */
export const authNamespace = "auth";

/** Strings of the auth feature in every supported language. */
export const authLocales = defineLocales({ en, de });
