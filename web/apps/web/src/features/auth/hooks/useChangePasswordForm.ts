import { useMutation } from "@tanstack/react-query";
import { useRef, useState, type RefObject, type SubmitEvent } from "react";

import { useApiClient } from "../../../shared/api";
import { changePassword, type PasswordChangeRequest } from "../api/authApi";
import {
  answers,
  generalFailure,
  matches,
  throttleSeconds,
  type GeneralFailure,
} from "../lib/authErrors";
import { useNewPasswordFields, type NewPasswordFields } from "./usePasswordFields";

/** Why the current password is marked. */
export type CurrentPasswordProblem = "empty" | "wrong";

/** What the dialog says below the fields. */
export type ChangePasswordFailure =
  { readonly kind: GeneralFailure } | { readonly kind: "throttled"; readonly seconds: number };

/** State and actions of the change-password dialog (AU-04). */
export interface ChangePasswordForm {
  readonly current: string;
  readonly setCurrent: (current: string) => void;
  readonly currentProblem: CurrentPasswordProblem | undefined;
  readonly currentRef: RefObject<HTMLInputElement | null>;
  readonly passwords: NewPasswordFields;
  readonly failure: ChangePasswordFailure | undefined;
  readonly pending: boolean;
  /** True once the server changed the password. */
  readonly done: boolean;
  readonly submit: (event: SubmitEvent<HTMLFormElement>) => void;
}

/**
 * Changes the account's own password. A wrong current password answers `403
 * invalid_credentials`: it marks the field and never signs out (the session stays, ADR 0032).
 */
export function useChangePasswordForm(): ChangePasswordForm {
  const client = useApiClient();
  const [current, setCurrent] = useState("");
  const [currentProblem, setCurrentProblem] = useState<CurrentPasswordProblem>();
  const [failure, setFailure] = useState<ChangePasswordFailure>();
  const currentRef = useRef<HTMLInputElement>(null);
  const passwords = useNewPasswordFields();
  const mutation = useMutation({
    mutationFn: (request: PasswordChangeRequest) => changePassword(client, request),
    onError: (error) => {
      if (matches(error, answers.wrongCurrentPassword)) {
        setCurrentProblem("wrong");
        currentRef.current?.focus();
      } else if (matches(error, answers.invalid) && error.field === "newPassword") {
        passwords.reject();
      } else if (matches(error, answers.tooManyAttempts)) {
        setFailure({ kind: "throttled", seconds: throttleSeconds(error) });
      } else {
        setFailure({ kind: generalFailure(error) });
      }
    },
  });
  const submit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (mutation.isPending) {
      return;
    }
    const emptyCurrent = current === "";
    setCurrentProblem(emptyCurrent ? "empty" : undefined);
    setFailure(undefined);
    const invalidPassword = passwords.validate();
    const firstInvalid = emptyCurrent ? currentRef : invalidPassword;
    if (firstInvalid !== undefined) {
      firstInvalid.current?.focus();
      return;
    }
    mutation.mutate({ currentPassword: current, newPassword: passwords.password });
  };
  return {
    current,
    setCurrent,
    currentProblem,
    currentRef,
    passwords,
    failure,
    pending: mutation.isPending,
    done: mutation.isSuccess,
    submit,
  };
}
