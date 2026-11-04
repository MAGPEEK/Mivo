import { ApiError } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { settingsNamespace } from "../locales";

const forbiddenStatus = 403;

/** Whether the server refused a management call because the peer is not on the local network. */
export function isManagementForbidden(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === forbiddenStatus || error.code === "forbidden")
  );
}

/** Texts with a `{{reason}}` placeholder that {@link useManagementErrorText} can fill. */
export type ManagementFailedKey =
  "libraries.remove.failed" | "libraries.dialog.saveFailed" | "libraries.browser.loadFailed";

/**
 * Text for a failed management call: the fixed local-network explanation for a 403, else
 * `failedKey` with the problem detail (title when the server sent none) as reason. Null
 * without an error.
 */
export function useManagementErrorText(
  error: Error | null,
  failedKey: ManagementFailedKey,
): string | null {
  const { t } = useTranslation(settingsNamespace);
  if (error === null) {
    return null;
  }
  if (isManagementForbidden(error)) {
    return t("libraries.errors.forbidden");
  }
  const reason = error instanceof ApiError ? (error.detail ?? error.title) : error.message;
  return t(failedKey, { reason });
}
