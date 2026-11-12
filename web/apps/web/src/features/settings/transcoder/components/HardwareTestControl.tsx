import { useTranslation } from "react-i18next";

import { secondaryButtonClass } from "../../../../shared/ui";
import { settingsNamespace } from "../../locales";
import type { HardwareTest } from "../hooks/useTranscoderStatus";
import { ManagementGuardHint } from "./ManagementGuardHint";

function Outcome({ test }: { readonly test: HardwareTest }) {
  const { t } = useTranslation(settingsNamespace);
  if (test.failure === "forbidden") {
    return <ManagementGuardHint />;
  }
  if (test.failure === "failed") {
    return (
      <p role="alert" className="text-sm">
        {t("transcoder.test.failed")}
      </p>
    );
  }
  // The live region stays mounted so a screen reader hears the texts when they change.
  let text = "";
  if (test.running) {
    text = test.alreadyRunning
      ? t("transcoder.test.alreadyRunning")
      : t("transcoder.test.runningStatus");
  } else if (test.stalled) {
    text = t("transcoder.test.noResult");
  } else if (test.finished) {
    text = t("transcoder.test.finished");
  }
  return (
    <p role="status" className="text-sm text-text-muted">
      {text}
    </p>
  );
}

/** The "Test hardware acceleration" button: disabled while a run is active, with its outcome. */
export function HardwareTestControl({ test }: { readonly test: HardwareTest }) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <div className="mt-4 flex flex-col items-start gap-2">
      <button
        type="button"
        disabled={test.running}
        onClick={test.start}
        className={secondaryButtonClass}
      >
        {test.running ? t("transcoder.test.running") : t("transcoder.test.button")}
      </button>
      <Outcome test={test} />
    </div>
  );
}
