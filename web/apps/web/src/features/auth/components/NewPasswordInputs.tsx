import { useTranslation } from "react-i18next";

import type { NewPasswordFields } from "../hooks/usePasswordFields";
import { PasswordField } from "./FormFields";
import { passwordProblemKeys } from "../lib/passwordProblemKeys";

/** Props of {@link NewPasswordInputs}. */
export interface NewPasswordInputsProps {
  readonly fields: NewPasswordFields;
  readonly passwordLabel: string;
  readonly repeatLabel: string;
}

/** The new password and its repetition with the length hint (ADR 0032 §1, prototype Q7). */
export function NewPasswordInputs({ fields, passwordLabel, repeatLabel }: NewPasswordInputsProps) {
  const { t } = useTranslation("auth");
  const { passwordProblem, repeatProblem } = fields;
  return (
    <>
      <PasswordField
        label={passwordLabel}
        value={fields.password}
        onChange={fields.setPassword}
        autoComplete="new-password"
        hint={t("fields.passwordHint")}
        error={passwordProblem === undefined ? undefined : t(passwordProblemKeys[passwordProblem])}
        inputRef={fields.passwordRef}
      />
      <PasswordField
        label={repeatLabel}
        value={fields.repeat}
        onChange={fields.setRepeat}
        autoComplete="new-password"
        error={repeatProblem === undefined ? undefined : t(passwordProblemKeys[repeatProblem])}
        inputRef={fields.repeatRef}
      />
    </>
  );
}
