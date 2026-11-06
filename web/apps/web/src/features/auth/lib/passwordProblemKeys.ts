import type { PasswordProblem } from "./credentials";

/** Translation keys of the new-password problems, shared by setup and change password. */
export const passwordProblemKeys = {
  tooShort: "errors.passwordTooShort",
  tooLong: "errors.passwordTooLong",
  mismatch: "errors.passwordMismatch",
  rejected: "errors.passwordRejected",
} as const satisfies Record<PasswordProblem | "rejected", string>;
