import { useId } from "react";
import { useTranslation } from "react-i18next";

import { ModalDialog, primaryButtonClass, secondaryButtonClass } from "../../../shared/ui";
import { useChangePasswordForm, type ChangePasswordForm } from "../hooks/useChangePasswordForm";
import { formatCountdown } from "../lib/countdown";
import { PasswordField } from "./FormFields";
import { NewPasswordInputs } from "./NewPasswordInputs";
import { Notice } from "./Notice";

/** Props of {@link ChangePasswordDialog}. */
export interface ChangePasswordDialogProps {
  /** Called by Cancel, Close and Escape; the caller then stops rendering the dialog. */
  readonly onClose: () => void;
}

const currentProblemKeys = {
  empty: "errors.passwordEmpty",
  wrong: "errors.currentPasswordWrong",
} as const;

const generalKeys = { network: "errors.network", generic: "errors.generic" } as const;

function FailureText({ failure }: { readonly failure: ChangePasswordForm["failure"] }) {
  const { t } = useTranslation("auth");
  let text = "";
  if (failure?.kind === "throttled") {
    text = t("errors.tooManyAttempts", { time: formatCountdown(failure.seconds) });
  } else if (failure !== undefined) {
    text = t(generalKeys[failure.kind]);
  }
  return (
    <p role="alert" className="mt-2 text-sm text-accent empty:hidden">
      {text}
    </p>
  );
}

function PasswordForm({
  form,
  formId,
}: {
  readonly form: ChangePasswordForm;
  readonly formId: string;
}) {
  const { t } = useTranslation("auth");
  const { currentProblem } = form;
  return (
    <form id={formId} noValidate onSubmit={form.submit}>
      <PasswordField
        label={t("fields.currentPassword")}
        value={form.current}
        onChange={form.setCurrent}
        autoComplete="current-password"
        error={currentProblem === undefined ? undefined : t(currentProblemKeys[currentProblem])}
        inputRef={form.currentRef}
      />
      <NewPasswordInputs
        fields={form.passwords}
        passwordLabel={t("fields.newPassword")}
        repeatLabel={t("fields.repeatNewPassword")}
      />
      <Notice tone="info">{t("changePassword.note")}</Notice>
      <FailureText failure={form.failure} />
    </form>
  );
}

/**
 * Change the account's own password (AU-04), opened from the profile menu (M7-31) or the
 * account page. Only for the primary profile of an account; the caller decides when to offer it.
 */
export function ChangePasswordDialog({ onClose }: ChangePasswordDialogProps) {
  const { t } = useTranslation("auth");
  const form = useChangePasswordForm();
  const formId = useId();
  const footer = form.done ? (
    <button type="button" onClick={onClose} className={`${primaryButtonClass} ml-auto`}>
      {t("changePassword.close")}
    </button>
  ) : (
    <>
      <button type="button" onClick={onClose} className={secondaryButtonClass}>
        {t("changePassword.cancel")}
      </button>
      <button type="submit" form={formId} disabled={form.pending} className={primaryButtonClass}>
        {form.pending ? t("changePassword.submitting") : t("changePassword.submit")}
      </button>
    </>
  );
  return (
    <ModalDialog
      title={t("changePassword.title")}
      closeLabel={t("changePassword.close")}
      onClose={onClose}
      size="narrow"
      footer={footer}
    >
      {form.done ? (
        <Notice tone="success" role="status">
          {t("changePassword.done")}
        </Notice>
      ) : (
        <PasswordForm form={form} formId={formId} />
      )}
    </ModalDialog>
  );
}
