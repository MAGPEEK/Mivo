import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useRef, useState, type RefObject, type SubmitEvent } from "react";

import { useApiClient } from "../../../shared/api";
import { completeSetup, type Credentials } from "../api/authApi";
import { remoteSetupPath } from "../lib/authPaths";
import { answers, generalFailure, matches, type GeneralFailure } from "../lib/authErrors";
import { checkName, normalizeName, type NameProblem } from "../lib/credentials";
import { resetSession, storeSession } from "./sessionCache";
import { useNewPasswordFields, type NewPasswordFields } from "./usePasswordFields";

/** What the form says below the fields. */
export type SetupFailure = "checkFields" | GeneralFailure;

/** State and actions of the setup form (AU-02). */
export interface SetupForm {
  readonly name: string;
  readonly setName: (name: string) => void;
  readonly nameProblem: NameProblem | "rejected" | undefined;
  readonly nameRef: RefObject<HTMLInputElement | null>;
  readonly passwords: NewPasswordFields;
  readonly failure: SetupFailure | undefined;
  readonly pending: boolean;
  readonly submit: (event: SubmitEvent<HTMLFormElement>) => void;
}

/** How the form shows a refusal the server sent for one of its fields or the whole form. */
interface SetupRefusalHandlers {
  readonly rejectName: () => void;
  readonly rejectPassword: () => void;
  readonly fail: (failure: SetupFailure) => void;
}

function useSetupMutation(handlers: SetupRefusalHandlers) {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  return useMutation({
    mutationFn: (credentials: Credentials) => completeSetup(client, credentials),
    onSuccess: (session) => {
      storeSession(queryClient, session);
    },
    onError: (error) => {
      if (matches(error, answers.forbidden)) {
        void navigate({ href: remoteSetupPath });
      } else if (matches(error, answers.conflict)) {
        // Someone else finished the setup meanwhile: the guard opens the sign-in page.
        resetSession(queryClient);
      } else if (matches(error, answers.invalid) && error.field === "name") {
        handlers.rejectName();
      } else if (matches(error, answers.invalid) && error.field === "password") {
        handlers.rejectPassword();
      } else {
        handlers.fail(generalFailure(error));
      }
    },
  });
}

/**
 * Creates the first admin account and signs in. Success stores the session, so the guard
 * leaves the setup for Home with "No libraries yet" (HO-09, prototype Q4); a remote peer is
 * sent to "Only from the local network" (AU-03).
 */
export function useSetupForm(): SetupForm {
  const [name, setName] = useState("");
  const [nameProblem, setNameProblem] = useState<NameProblem | "rejected">();
  const [failure, setFailure] = useState<SetupFailure>();
  const nameRef = useRef<HTMLInputElement>(null);
  const passwords = useNewPasswordFields();
  const mutation = useSetupMutation({
    rejectName: () => {
      setNameProblem("rejected");
      nameRef.current?.focus();
    },
    rejectPassword: passwords.reject,
    fail: setFailure,
  });
  const submit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (mutation.isPending) {
      return;
    }
    const problem = checkName(name);
    setNameProblem(problem);
    const invalidPassword = passwords.validate();
    const firstInvalid = problem === undefined ? invalidPassword : nameRef;
    if (firstInvalid !== undefined) {
      setFailure("checkFields");
      firstInvalid.current?.focus();
      return;
    }
    setFailure(undefined);
    mutation.mutate({ name: normalizeName(name), password: passwords.password });
  };
  return {
    name,
    setName,
    nameProblem,
    nameRef,
    passwords,
    failure,
    pending: mutation.isPending,
    submit,
  };
}
