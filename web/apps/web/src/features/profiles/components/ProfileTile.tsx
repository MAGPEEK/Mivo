import type { ProfileSummary } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { LockIcon } from "../../../shared/ui";
import { Avatar } from "./Avatar";

const tileClass =
  "flex w-32 flex-col items-center gap-2 rounded-lg p-2 text-text hover:bg-surface aria-disabled:cursor-wait";

/** Props of {@link ProfileTile}. */
export interface ProfileTileProps {
  readonly profile: ProfileSummary;
  readonly onChoose: () => void;
  readonly busy: boolean;
  /** Set on the tile whose PIN pad is open, so focus can return to it. */
  readonly tileRef?: (element: HTMLButtonElement | null) => void;
}

// Visual only: the button names the PIN after the profile name, so the name comes first.
const lockMarker = (
  <span className="absolute -right-0.5 -bottom-0.5 grid size-7 place-items-center rounded-full border border-divider bg-surface text-text">
    <LockIcon className="size-4" />
  </span>
);

/** One profile of the picker: avatar, name, the lock for a PIN and the admin marker. */
export function ProfileTile({ profile, onChoose, busy, tileRef }: ProfileTileProps) {
  const { t } = useTranslation("profiles");
  return (
    <button
      ref={tileRef}
      type="button"
      onClick={onChoose}
      aria-disabled={busy}
      className={tileClass}
    >
      <Avatar
        name={profile.name}
        color={profile.avatarColor}
        size="large"
        marker={profile.hasPin ? lockMarker : undefined}
      />
      <span className="max-w-full truncate font-semibold">{profile.name}</span>
      {profile.hasPin && <span className="sr-only">, {t("picker.pinRequired")}</span>}
      {profile.isAdmin && (
        <span className="rounded-sm border border-accent px-1.5 text-xs font-medium text-accent">
          {t("picker.admin")}
        </span>
      )}
    </button>
  );
}

/** Props of {@link AddProfileTile}. */
export interface AddProfileTileProps {
  readonly onAdd: () => void;
  readonly busy: boolean;
}

/** The dashed "Add profile" tile; it opens the managed-profile editor (M7-33). */
export function AddProfileTile({ onAdd, busy }: AddProfileTileProps) {
  const { t } = useTranslation("profiles");
  return (
    <button type="button" onClick={onAdd} aria-disabled={busy} className={tileClass}>
      <span
        aria-hidden="true"
        className="grid size-24 place-items-center rounded-full border-2 border-dashed border-text-muted text-4xl text-text-muted"
      >
        +
      </span>
      <span className="font-semibold text-text-muted">{t("picker.add")}</span>
    </button>
  );
}
