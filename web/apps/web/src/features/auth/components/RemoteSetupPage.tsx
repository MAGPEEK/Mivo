import { productName } from "@mivo/ui-tokens";
import { useTranslation } from "react-i18next";

import { secondaryButtonClass } from "../../../shared/ui";
import { useRemoteSetupCheck } from "../hooks/useRemoteSetupCheck";
import { AuthCard } from "./AuthCard";

const resultKeys = {
  stillRemote: "remote.still",
  network: "errors.network",
  generic: "errors.generic",
} as const;

/** "Setup only from the local network" for a remote peer of a fresh server (AU-03). */
export function RemoteSetupPage() {
  const { t } = useTranslation("auth");
  const check = useRemoteSetupCheck();
  return (
    <AuthCard title={t("remote.title")} size="wide">
      <p className="mt-2 text-sm text-text">{t("remote.body", { productName })}</p>
      <p className="mt-3 text-sm font-semibold text-text">{t("remote.how")}</p>
      <ol className="mt-1 list-decimal pl-5 text-sm text-text">
        <li>{t("remote.step1")}</li>
        <li>{t("remote.step2")}</li>
        <li>{t("remote.step3")}</li>
      </ol>
      <button
        type="button"
        onClick={check.check}
        aria-disabled={check.pending}
        className={`${secondaryButtonClass} mt-4`}
      >
        {check.pending ? t("remote.checking") : t("remote.retry")}
      </button>
      <p role="status" className="mt-2 text-sm text-accent empty:hidden">
        {check.result === undefined ? "" : t(resultKeys[check.result])}
      </p>
    </AuthCard>
  );
}
