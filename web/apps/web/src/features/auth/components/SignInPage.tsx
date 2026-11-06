import { productName } from "@mivo/ui-tokens";
import { useTranslation } from "react-i18next";

import { primaryButtonClass } from "../../../shared/ui";
import { useSignInForm, type SignInForm } from "../hooks/useSignInForm";
import type { AuthSearch } from "../lib/authSearch";
import { formatCountdown } from "../lib/countdown";
import { recoveryCommand } from "../lib/recoveryCommand";
import { AuthCard } from "./AuthCard";
import { PasswordField, TextField } from "./FormFields";
import { Notice } from "./Notice";

/** Props of {@link SignInPage}. */
export interface SignInPageProps {
  readonly search: AuthSearch;
}

function ReasonNotice({ reason }: { readonly reason: AuthSearch["reason"] }) {
  const { t } = useTranslation("auth");
  if (reason === "signed-out") {
    return (
      <Notice tone="success" role="status">
        <p>{t("signIn.signedOut")}</p>
        <p className="text-xs text-text-muted">{t("signIn.signedOutHint")}</p>
      </Notice>
    );
  }
  if (reason === "expired") {
    return (
      <Notice tone="warning" role="status">
        <p>{t("signIn.expired")}</p>
        <p className="text-xs text-text-muted">{t("signIn.expiredHint")}</p>
      </Notice>
    );
  }
  return null;
}

// The visible countdown ticks every second; screen readers hear the wait once, when it starts.
function ThrottleNotice({ form }: { readonly form: SignInForm }) {
  const { t } = useTranslation("auth");
  const { throttle } = form;
  if (throttle.throttled) {
    return (
      <Notice tone="problem">
        <p aria-hidden="true">
          {t("signIn.throttled", { time: formatCountdown(throttle.secondsLeft) })}
        </p>
        <p role="alert" className="sr-only">
          {t("signIn.throttled", { time: formatCountdown(throttle.totalSeconds) })}
        </p>
        <p className="text-xs text-text-muted">{t("signIn.throttleHint")}</p>
      </Notice>
    );
  }
  return throttle.over ? (
    <Notice tone="info" role="status">
      {t("signIn.throttleOver")}
    </Notice>
  ) : null;
}

function ForgotPassword() {
  const { t } = useTranslation("auth");
  return (
    <details className="mt-4 text-sm">
      <summary className="cursor-pointer text-accent">{t("signIn.forgot.question")}</summary>
      <p className="mt-2 text-text">{t("signIn.forgot.answer")}</p>
      <p className="mt-2 text-text-muted">
        {t("signIn.forgot.admin")}
        <br />
        <code className="rounded-sm bg-surface-raised px-1 text-xs">
          {recoveryCommand(t("setup.namePlaceholder"))}
        </code>
      </p>
    </details>
  );
}

const failureKeys = {
  credentials: "errors.credentials",
  network: "errors.network",
  generic: "errors.generic",
} as const;

/** Sign in with name and password (AU-01), also after sign-out (AU-05) and expiry (AU-06). */
export function SignInPage({ search }: SignInPageProps) {
  const { t } = useTranslation("auth");
  const form = useSignInForm();
  const disabled = form.throttle.throttled || form.pending;
  return (
    <AuthCard
      title={t("signIn.title", { productName })}
      header={<ReasonNotice reason={search.reason} />}
    >
      <form noValidate onSubmit={form.submit} className="mt-4">
        <TextField
          label={t("fields.name")}
          value={form.name}
          onChange={form.setName}
          autoComplete="username"
          error={form.emptyFields.name ? t("errors.nameEmpty") : undefined}
          inputRef={form.nameRef}
        />
        <PasswordField
          label={t("fields.password")}
          value={form.password}
          onChange={form.setPassword}
          autoComplete="current-password"
          error={form.emptyFields.password ? t("errors.passwordEmpty") : undefined}
          inputRef={form.passwordRef}
        />
        <p role="alert" className="mb-3 text-sm text-accent empty:hidden">
          {form.failure === undefined ? "" : t(failureKeys[form.failure])}
        </p>
        <ThrottleNotice form={form} />
        <button type="submit" disabled={disabled} className={`${primaryButtonClass} w-full`}>
          {form.pending ? t("signIn.submitting") : t("signIn.submit")}
        </button>
        <p className="mt-2 text-xs text-text-muted">{t("signIn.stay")}</p>
      </form>
      <ForgotPassword />
    </AuthCard>
  );
}
