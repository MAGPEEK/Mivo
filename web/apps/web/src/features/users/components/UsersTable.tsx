import type { Profile, User } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { Badge, secondaryButtonClass } from "../../../shared/ui";
import type { UserEntry } from "../hooks/useUserList";
import { useUserTexts, type UserTexts } from "../hooks/useUserTexts";
import { usersNamespace } from "../locales";

/** Props of {@link UsersTable}. */
export interface UsersTableProps {
  readonly entries: readonly UserEntry[];
  /** The time "Last activity" is measured against. */
  readonly now: Date;
  readonly onEdit: (user: User) => void;
  readonly onDelete: (user: User) => void;
}

const cellClass = "px-3 py-2 align-top";
const headerClass =
  "px-3 py-2 text-left text-xs font-medium tracking-wide text-text-muted uppercase";
const smallButtonClass = `${secondaryButtonClass} px-2 py-1 text-xs`;

function StatusPill({ user, texts }: { readonly user: User; readonly texts: UserTexts }) {
  const active = user.status === "active";
  return (
    <span className="inline-flex items-center gap-1.5 text-sm">
      <span
        aria-hidden="true"
        className={`size-2 rounded-full ${active ? "bg-direct-play" : "bg-text-muted"}`}
      />
      {texts.status(user.status)}
    </span>
  );
}

interface AccountRowProps extends Omit<UsersTableProps, "entries"> {
  readonly user: User;
  readonly texts: UserTexts;
}

function AccountRow({ user, texts, now, onEdit, onDelete }: AccountRowProps) {
  const { t } = useTranslation(usersNamespace);
  return (
    <tr className="border-t border-divider">
      <th scope="row" className={`${cellClass} text-left font-semibold`}>
        {user.name}
      </th>
      <td className={cellClass}>{texts.role(user.role)}</td>
      <td className={cellClass}>{texts.contentLimit(user.contentLimit)}</td>
      <td className={cellClass}>{texts.libraryAccess(user.libraryAccess)}</td>
      <td className={cellClass}>{texts.lastActivity(user.lastActiveAt, now)}</td>
      <td className={cellClass}>
        <StatusPill user={user} texts={texts} />
      </td>
      <td className={`${cellClass} tabular-nums`}>{user.profileCount}</td>
      <td className={`${cellClass} whitespace-nowrap`}>
        <div className="flex gap-2">
          <button
            type="button"
            aria-label={t("users.actions.editLabel", { name: user.name })}
            onClick={() => {
              onEdit(user);
            }}
            className={smallButtonClass}
          >
            {t("users.actions.edit")}
          </button>
          <button
            type="button"
            aria-label={t("users.actions.deleteLabel", { name: user.name })}
            onClick={() => {
              onDelete(user);
            }}
            className={smallButtonClass}
          >
            {t("users.actions.delete")}
          </button>
        </div>
      </td>
    </tr>
  );
}

interface ProfileRowProps {
  readonly profile: Profile;
  readonly owner: User;
  readonly texts: UserTexts;
}

// A managed profile under its account (US-20); M7-33 adds its editor.
function ProfileRow({ profile, owner, texts }: ProfileRowProps) {
  const { t } = useTranslation(usersNamespace);
  return (
    <tr className="text-text-muted">
      <th scope="row" className={`${cellClass} pl-8 text-left font-normal`}>
        <span aria-hidden="true">↳ </span>
        <span className="text-text">{profile.name}</span>
        <span className="sr-only"> – {t("users.profile.managedOf", { name: owner.name })}</span>
        {profile.hasPin && (
          <span className="ml-2">
            <Badge>{t("users.profile.pin")}</Badge>
          </span>
        )}
      </th>
      <td className={cellClass}>{t("users.role.profile")}</td>
      <td className={cellClass}>{texts.contentLimit(profile.contentLimit)}</td>
      <td className={cellClass}>{texts.libraryAccess(profile.libraryAccess)}</td>
      <td className={cellClass} />
      <td className={cellClass} />
      <td className={cellClass} />
      <td className={cellClass} />
    </tr>
  );
}

/** The accounts with their managed profiles nested below each (US-20). */
export function UsersTable({ entries, ...rowProps }: UsersTableProps) {
  const { t } = useTranslation(usersNamespace);
  const texts = useUserTexts();
  const columns = [
    "name",
    "role",
    "contentLimit",
    "libraries",
    "lastActivity",
    "status",
    "profiles",
    "actions",
  ] as const;
  return (
    <div className="overflow-x-auto rounded-lg border border-divider bg-surface">
      <table className="w-full min-w-[56rem] border-collapse text-sm">
        <caption className="sr-only">{t("users.table.label")}</caption>
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column} scope="col" className={headerClass}>
                {t(`users.table.${column}`)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {entries.map(({ user, managedProfiles }) => [
            <AccountRow key={user.id} user={user} texts={texts} {...rowProps} />,
            ...managedProfiles.map((profile) => (
              <ProfileRow key={profile.id} profile={profile} owner={user} texts={texts} />
            )),
          ])}
        </tbody>
      </table>
    </div>
  );
}
