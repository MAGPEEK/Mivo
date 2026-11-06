import type { ContentLimit, LibraryAccess, User } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { contentLimitDraft, draftMaxAge } from "../lib/contentLimit";
import { formatRelativeTime } from "../lib/relativeTime";
import { usersNamespace } from "../locales";

/** Formatters for the cells of the users list. */
export interface UserTexts {
  readonly role: (role: User["role"]) => string;
  readonly status: (status: User["status"]) => string;
  readonly contentLimit: (limit: ContentLimit) => string;
  readonly libraryAccess: (access: LibraryAccess) => string;
  /** Relative to `now`; "Never" without a time. */
  readonly lastActivity: (lastActiveAt: string | undefined, now: Date) => string;
}

// The generated unions list what this client knows; a newer server may send more (ADR 0017).
const knownRoles = ["admin", "user"] as const;
const knownStatuses = ["active", "disabled"] as const;

function isOneOf<Known extends string>(known: readonly Known[], value: string): value is Known {
  return (known as readonly string[]).includes(value);
}

/** Translated, tolerant texts: values of a newer server fall back to a generic word. */
export function useUserTexts(): UserTexts {
  const { t, i18n } = useTranslation(usersNamespace);
  return {
    role: (role) => (isOneOf(knownRoles, role) ? t(`users.role.${role}`) : t("users.role.unknown")),
    status: (status) =>
      isOneOf(knownStatuses, status) ? t(`users.status.${status}`) : t("users.status.unknown"),
    contentLimit: (limit) => {
      const draft = contentLimitDraft(limit);
      const age = draftMaxAge(draft);
      const label =
        draft.preset === "custom"
          ? t("users.limit.customAge", { age: draft.customAge })
          : t(`users.limit.${draft.preset}`);
      return limit.hideUnrated && age !== null
        ? `${label}, ${t("users.limit.hidesUnrated")}`
        : label;
    },
    libraryAccess: (access) => {
      if (access.all) {
        return t("users.access.all");
      }
      const count = access.libraryIds?.length ?? 0;
      return count === 0 ? t("users.access.none") : t("users.access.count", { count });
    },
    lastActivity: (lastActiveAt, now) =>
      lastActiveAt === undefined
        ? t("users.lastActivity.never")
        : formatRelativeTime(new Date(lastActiveAt), now, i18n.language),
  };
}
