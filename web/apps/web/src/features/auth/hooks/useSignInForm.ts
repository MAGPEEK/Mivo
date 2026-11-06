import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouterState } from "@tanstack/react-router";
import { useRef, useState, type RefObject, type SubmitEvent } from "react";

import { useApiClient } from "../../../shared/api";
import { signIn } from "../api/authApi";
import {
  answers,
  generalFailure,
  matches,
  throttleSeconds,
  type GeneralFailure,
} from "../lib/authErrors";
import { normalizeName } from "../lib/credentials";
import { resetSession, storeSession } from "./sessionCache";
import { useThrottleCountdown, type ThrottleCountdown } from "./useThrottleCountdown";

/** Why the last sign-in failed; one generic text never tells whether the name exists. */
export type SignInFailure = "credentials" | GeneralFailure;

/** Which sign-in fields were left empty. */
export interface SignInFieldErrors {
  readonly name: boolean;
  readonly password: boolean;
}

/** State and actions of the sign-in form (AU-01). */
export interface SignInForm {
  readonly name: string;
  readonly setName: (name: string) => void;
  readonly password: string;
  readonly setPassword: (password: string) => void;
  readonly emptyFields: SignInFieldErrors;
  readonly failure: SignInFailure | undefined;
  readonly pending: boolean;
  readonly throttle: ThrottleCountdown;
  readonly nameRef: RefObject<HTMLInputElement | null>;
  /** The password field; it is emptied and focused after a failure, as drawn. */
  readonly passwordRef: RefObject<HTMLInputElement | null>;
  readonly submit: (event: SubmitEvent<HTMLFormElement>) => void;
}

const noEmptyFields: SignInFieldErrors = { name: false, password: false };

// The history state the 401 handling leaves behind (AU-06): the name to prefill.
function prefilledName(state: object): string {
  return "signInName" in state && typeof state.signInName === "string" ? state.signInName : "";
}

function useSignInMutation(onFailure: (error: Error) => void) {
  const client = useApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (credentials: { readonly name: string; readonly password: string }) =>
      signIn(client, credentials),
    onSuccess: (session) => {
      storeSession(queryClient, session);
    },
    onError: (error) => {
      if (matches(error, answers.setupRequired)) {
        // The accounts are gone (a reset database): the guard opens the setup.
        resetSession(queryClient);
        return;
      }
      onFailure(error);
    },
  });
}

/**
 * Signs in with name and password. Success stores the session, so the guard moves on to
 * `next`, Home or the profile picker; a `429` starts the countdown.
 */
export function useSignInForm(): SignInForm {
  const historyState = useRouterState({ select: (state) => state.location.state });
  const [name, setName] = useState(() => prefilledName(historyState));
  const [password, setPassword] = useState("");
  const [emptyFields, setEmptyFields] = useState(noEmptyFields);
  const [failure, setFailure] = useState<SignInFailure>();
  const nameRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const throttle = useThrottleCountdown();
  const mutation = useSignInMutation((error) => {
    setPassword("");
    passwordRef.current?.focus();
    if (matches(error, answers.tooManyAttempts)) {
      throttle.start(throttleSeconds(error));
      return;
    }
    setFailure(matches(error, answers.unauthorized) ? "credentials" : generalFailure(error));
  });
  const submit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (throttle.throttled || mutation.isPending) {
      return;
    }
    const normalizedName = normalizeName(name);
    const empty = { name: normalizedName === "", password: password === "" };
    setEmptyFields(empty);
    setFailure(undefined);
    throttle.reset();
    if (empty.name || empty.password) {
      (empty.name ? nameRef : passwordRef).current?.focus();
      return;
    }
    mutation.mutate({ name: normalizedName, password });
  };
  return {
    name,
    setName,
    password,
    setPassword,
    emptyFields,
    failure,
    pending: mutation.isPending,
    throttle,
    nameRef,
    passwordRef,
    submit,
  };
}
