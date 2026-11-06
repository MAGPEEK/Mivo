import { productName } from "@mivo/ui-tokens";
import { useTranslation } from "react-i18next";

import { primaryButtonClass } from "../../../shared/ui";
import { useSetupForm } from "../hooks/useSetupForm";
import { dockerRecoveryCommand, recoveryCommand } from "../lib/recoveryCommand";
import { AuthCard } from "./AuthCard";
import { TextField } from "./FormFields";
import { NewPasswordInputs } from "./NewPasswordInputs";

const nameProblemKeys = {
  empty: "errors.nameEmpty",
  tooLong: "errors.nameTooLong",
  rejected: "errors.nameRejected",
} as const;

const failureKeys = {
  checkFields: "errors.checkFields",
  network: "errors.network",
  generic: "errors.generic",
} as const;

const codeClass = "rounded-sm bg-surface-raised px-1 text-xs break-words";

/** The two setup steps; step 2 is the "No libraries yet" state on Home (HO-09). */
export function SetupSteps({ current }: { readonly current: 1 | 2 }) {
  const { t } = useTranslation("auth");
  const steps = [t("setup.stepAccount"), t("setup.stepLibrary")];
  return (
    <ol aria-label={t("setup.steps", { step: current })} className="mb-4 flex gap-2 text-xs">
      {steps.map((label, index) => {
        const step = index + 1;
        const isCurrent = step === current;
        return (
          <li
            key={label}
            aria-current={isCurrent ? "step" : undefined}
            className={`flex-1 border-t-4 pt-1 ${isCurrent ? "border-accent font-semibold text-text" : "border-divider text-text-muted"}`}
          >
            {`${String(step)} · ${label}`}
          </li>
        );
      })}
    </ol>
  );
}

function LocalOnlyNote() {
  const { t } = useTranslation("auth");
  return (
    <div className="my-4 rounded-md border border-divider bg-background p-3 text-sm">
      <p className="mb-1 font-semibold text-text">{t("setup.localOnlyTitle")}</p>
      <p className="mb-2 text-text">{t("setup.localOnly")}</p>
      <p className="mb-2 text-text">
        {t("setup.recover")}
        <br />
        <code className={codeClass}>{recoveryCommand(t("setup.namePlaceholder"))}</code>
      </p>
      <p className="text-text-muted">
        {t("setup.recoverWhere", { dockerCommand: dockerRecoveryCommand() })}
      </p>
    </div>
  );
}

/** First-run setup step 1 "Create admin account" (AU-02). */
export function SetupPage() {
  const { t } = useTranslation("auth");
  const form = useSetupForm();
  const { nameProblem } = form;
  return (
    <AuthCard title={t("setup.title")} size="wide" header={<SetupSteps current={1} />}>
      <p className="mt-1 mb-4 text-sm text-text-muted">{t("setup.intro", { productName })}</p>
      <form noValidate onSubmit={form.submit}>
        <TextField
          label={t("fields.name")}
          value={form.name}
          onChange={form.setName}
          autoComplete="username"
          hint={t("fields.nameHint")}
          error={nameProblem === undefined ? undefined : t(nameProblemKeys[nameProblem])}
          inputRef={form.nameRef}
        />
        <NewPasswordInputs
          fields={form.passwords}
          passwordLabel={t("fields.password")}
          repeatLabel={t("fields.repeatPassword")}
        />
        <p role="alert" className="text-sm text-accent empty:hidden">
          {form.failure === undefined ? "" : t(failureKeys[form.failure])}
        </p>
        <LocalOnlyNote />
        <button type="submit" disabled={form.pending} className={`${primaryButtonClass} w-full`}>
          {form.pending ? t("setup.submitting") : t("setup.submit")}
        </button>
      </form>
    </AuthCard>
  );
}
