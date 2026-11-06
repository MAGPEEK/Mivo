// Client-side checks of the account rules of ADR 0032 §1. The server checks again; these only
// spare a round trip and name the field. Passwords have no composition rules (prototype Q7).

/** Longest account name in characters (Unicode code points after NFC). */
export const maxNameLength = 40;

/** Shortest password in UTF-8 bytes. */
export const minPasswordBytes = 8;

/** Longest password in UTF-8 bytes. */
export const maxPasswordBytes = 128;

/** Why a name is refused; `undefined` when it is fine. */
export type NameProblem = "empty" | "tooLong";

/** Why a new password is refused; `undefined` when it is fine. */
export type PasswordProblem = "tooShort" | "tooLong" | "mismatch";

const utf8 = new TextEncoder();

/** The name as the server compares it: trimmed and NFC-normalised. */
export function normalizeName(name: string): string {
  return name.trim().normalize("NFC");
}

/** Checks an account name for setup: 1 to 40 characters after normalising. */
export function checkName(name: string): NameProblem | undefined {
  const normalized = normalizeName(name);
  if (normalized === "") {
    return "empty";
  }
  return Array.from(normalized).length > maxNameLength ? "tooLong" : undefined;
}

/** Checks a new password: 8 to 128 bytes of UTF-8. */
export function checkNewPassword(password: string): PasswordProblem | undefined {
  const bytes = utf8.encode(password).length;
  if (bytes < minPasswordBytes) {
    return "tooShort";
  }
  return bytes > maxPasswordBytes ? "tooLong" : undefined;
}

/** Checks the repetition, only once the password itself is fine. */
export function checkRepeat(password: string, repeat: string): PasswordProblem | undefined {
  return checkNewPassword(password) === undefined && password !== repeat ? "mismatch" : undefined;
}
