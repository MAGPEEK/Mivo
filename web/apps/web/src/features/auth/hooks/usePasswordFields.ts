import { useRef, useState, type RefObject } from "react";

import { checkNewPassword, checkRepeat, type PasswordProblem } from "../lib/credentials";

/** A new password with its repetition, as the setup and change-password forms ask for it. */
export interface NewPasswordFields {
  readonly password: string;
  readonly setPassword: (password: string) => void;
  readonly repeat: string;
  readonly setRepeat: (repeat: string) => void;
  readonly passwordProblem: PasswordProblem | "rejected" | undefined;
  readonly repeatProblem: PasswordProblem | undefined;
  readonly passwordRef: RefObject<HTMLInputElement | null>;
  readonly repeatRef: RefObject<HTMLInputElement | null>;
  /** Checks both fields, shows the problems and returns the first field that has one. */
  readonly validate: () => RefObject<HTMLInputElement | null> | undefined;
  /** Shows that the server refused the password. */
  readonly reject: () => void;
}

/** The new-password and repeat fields with the 8–128 byte rule (ADR 0032 §1). */
export function useNewPasswordFields(): NewPasswordFields {
  const [password, setPassword] = useState("");
  const [repeat, setRepeat] = useState("");
  const [passwordProblem, setPasswordProblem] = useState<PasswordProblem | "rejected">();
  const [repeatProblem, setRepeatProblem] = useState<PasswordProblem>();
  const passwordRef = useRef<HTMLInputElement>(null);
  const repeatRef = useRef<HTMLInputElement>(null);
  const validate = () => {
    const newProblem = checkNewPassword(password);
    const repeatedProblem = checkRepeat(password, repeat);
    setPasswordProblem(newProblem);
    setRepeatProblem(repeatedProblem);
    if (newProblem !== undefined) {
      return passwordRef;
    }
    return repeatedProblem === undefined ? undefined : repeatRef;
  };
  return {
    password,
    setPassword,
    repeat,
    setRepeat,
    passwordProblem,
    repeatProblem,
    passwordRef,
    repeatRef,
    validate,
    reject: () => {
      setPasswordProblem("rejected");
      passwordRef.current?.focus();
    },
  };
}
