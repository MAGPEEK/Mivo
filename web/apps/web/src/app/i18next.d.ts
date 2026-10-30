import type { sharedNamespace, Translation } from "../shared/i18n";
import type { FeatureNamespaces } from "./i18n";

// Types `t()` keys against the English resources of every namespace, so a misspelled key or
// namespace fails type checking. Declared in the app layer, the only layer that knows every
// feature (`shared` must not import features).
declare module "i18next" {
  interface CustomTypeOptions {
    defaultNS: typeof sharedNamespace;
    resources: Record<typeof sharedNamespace, Translation> & {
      [Namespace in keyof FeatureNamespaces]: FeatureNamespaces[Namespace]["en"];
    };
  }
}
