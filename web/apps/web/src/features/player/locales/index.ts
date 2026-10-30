import { defineLocales } from "../../../shared/i18n";
import de from "./de.json";
import en from "./en.json";

/** i18next namespace of the player feature. Its strings live in the JSON files next to this one. */
export const playerNamespace = "player";

/** Strings of the player feature in every supported language. */
export const playerLocales = defineLocales({ en, de });
