import type { Profile } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { Avatar, Badge, secondaryButtonClass } from "../../../../shared/ui";
import { useUserTexts, type UserTexts } from "../../hooks/useUserTexts";
import { usersNamespace } from "../../locales";

/** Props of {@link OwnProfileList}. */
export interface OwnProfileListProps {
  /** Name of the account, for the accessible name of the list. */
  readonly accountName: string;
  /** The primary profile first, then the managed ones by name. */
  readonly profiles: readonly Profile[];
  readonly onEdit: (profile: Profile) => void;
  readonly onDelete: (profile: Profile) => void;
}

const smallButtonClass = `${secondaryButtonClass} px-2 py-1 text-xs`;

interface ProfileItemProps extends Pick<OwnProfileListProps, "onEdit" | "onDelete"> {
  readonly profile: Profile;
  readonly texts: UserTexts;
}

// A managed profile shows its own limit and libraries; the primary one has the account's.
function ProfileSettings({ profile, texts }: Pick<ProfileItemProps, "profile" | "texts">) {
  const { t } = useTranslation(usersNamespace);
  return (
    <div className="min-w-0 flex-1">
      <p className="flex flex-wrap items-center gap-2 font-semibold text-text">
        {profile.name}
        {profile.hasPin && <Badge>{t("users.profile.pin")}</Badge>}
      </p>
      <p className="text-sm text-text-muted">
        {profile.isPrimary ? t("profiles.page.primary") : t("profiles.page.managed")}
        {!profile.isPrimary && (
          <>
            {" · "}
            {texts.contentLimit(profile.contentLimit)}
            {" · "}
            {t("profiles.page.libraries", { access: texts.libraryAccess(profile.libraryAccess) })}
          </>
        )}
      </p>
    </div>
  );
}

function ProfileItem({ profile, texts, onEdit, onDelete }: ProfileItemProps) {
  const { t } = useTranslation(usersNamespace);
  return (
    <li className="flex flex-wrap items-center gap-3 border-t border-divider px-4 py-3 first:border-t-0">
      <Avatar name={profile.name} color={profile.avatarColor} size="medium" />
      <ProfileSettings profile={profile} texts={texts} />
      <div className="flex gap-2">
        <button
          type="button"
          aria-label={t("profiles.actions.editLabel", { name: profile.name })}
          onClick={() => {
            onEdit(profile);
          }}
          className={smallButtonClass}
        >
          {t("users.actions.edit")}
        </button>
        {!profile.isPrimary && (
          <button
            type="button"
            aria-label={t("profiles.actions.deleteLabel", { name: profile.name })}
            onClick={() => {
              onDelete(profile);
            }}
            className={smallButtonClass}
          >
            {t("users.actions.delete")}
          </button>
        )}
      </div>
    </li>
  );
}

/** The profiles of the own account, each with Edit and, for managed ones, Delete. */
export function OwnProfileList({ accountName, profiles, onEdit, onDelete }: OwnProfileListProps) {
  const { t } = useTranslation(usersNamespace);
  const texts = useUserTexts();
  return (
    <ul
      aria-label={t("profiles.page.listLabel", { name: accountName })}
      className="rounded-lg border border-divider bg-surface"
    >
      {profiles.map((profile) => (
        <ProfileItem
          key={profile.id}
          profile={profile}
          texts={texts}
          onEdit={onEdit}
          onDelete={onDelete}
        />
      ))}
    </ul>
  );
}
