import type { Translation } from "./resources";

// Types `t()` keys against the English resources, so a misspelled key fails type checking.
declare module "i18next" {
  interface CustomTypeOptions {
    defaultNS: "translation";
    resources: { translation: Translation };
  }
}
