import { useTranslation } from "react-i18next";

import type { IdentifyErrorKind } from "../lib/identifyError";
import { identifyNamespace } from "../locales";
import type { MatchOutcome } from "./useMatchActions";

/** Text of a failed metadata call; undefined without an error. */
export function useIdentifyErrorText(kind: IdentifyErrorKind | undefined): string | undefined {
  const { t } = useTranslation(identifyNamespace);
  return kind === undefined ? undefined : t(`errors.${kind}`);
}

/** Text announcing the outcome of the last match action; empty before the first one. */
export function useOutcomeText(outcome: MatchOutcome | undefined): string {
  const { t } = useTranslation(identifyNamespace);
  if (outcome === undefined) {
    return "";
  }
  if (outcome.kind === "refreshFailed") {
    return t("status.refreshFailed", { reason: t(`errors.${outcome.errorKind}`) });
  }
  return t(`status.${outcome.kind}`);
}
